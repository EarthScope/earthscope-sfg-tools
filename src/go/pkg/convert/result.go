package convert

import (
	"errors"
	"iter"
	"slices"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// ResultSchema is the version of the Result document. Fields are only ever
// added; a reader of version 1 keeps working.
const ResultSchema = 1

// Exit codes of a run.
const (
	ExitOK      = 0
	ExitFatal   = 1
	ExitUsage   = 2
	ExitPartial = 3
)

// Result is the machine-readable record of one run: what was asked for,
// then what was read, written and went wrong.
type Result struct {
	Schema    int        `json:"schema"`
	Command   string     `json:"command"`
	OK        bool       `json:"ok"`
	Source    *Endpoint  `json:"source,omitempty"`
	Sink      *Endpoint  `json:"sink,omitempty"`
	Scope     string     `json:"scope,omitempty"`
	Selection *Selection `json:"selection,omitempty"`
	Inputs    []Input    `json:"inputs"`
	Streams   []Stream   `json:"streams"`
	Outputs   []Output   `json:"outputs"`
	Errors    []Error    `json:"errors"`
}

// Endpoint is a resolved source or sink.
type Endpoint struct {
	Kind        string   `json:"kind"`
	URI         string   `json:"uri"`
	URIs        []string `json:"uris,omitempty"` // every source URI, when more than one was given
	Catalog     string   `json:"catalog,omitempty"`
	Tables      []string `json:"tables,omitempty"`
	Types       []string `json:"types,omitempty"`
	TableFormat string   `json:"table_format,omitempty"`
}

// Selection is the resolved row selection.
type Selection struct {
	LogSource string   `json:"log_source,omitempty"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Columns   []string `json:"columns,omitempty"`
}

// Input is one file a source read.
type Input struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Stream is one stream's rows that passed from source to sink.
type Stream struct {
	EDID  string    `json:"edid"`
	Table string    `json:"table"`
	Rows  int64     `json:"rows"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Output is one thing a sink wrote.
type Output struct {
	Kind  string `json:"kind"`
	Path  string `json:"path,omitempty"`
	Table string `json:"table,omitempty"`
	Rows  int64  `json:"rows"`
	Bytes int64  `json:"bytes,omitempty"`
}

// Error is one failure: the stage it happened in (usage, source, read,
// write), what it happened to, and the message.
type Error struct {
	Stage   string `json:"stage"`
	Subject string `json:"subject,omitempty"`
	Message string `json:"message"`
}

// UsageResult is the result of a run rejected before it started.
func UsageResult(err error) *Result {
	res := &Result{Schema: ResultSchema, Command: "convert",
		Inputs: []Input{}, Streams: []Stream{}, Outputs: []Output{}, Errors: []Error{}}
	res.addError("usage", "", err)
	return res
}

func (r *Result) addError(stage, subject string, err error) {
	r.Errors = append(r.Errors, Error{Stage: stage, Subject: subject, Message: err.Error()})
}

// ExitCode is the exit code of a run that returned res and err.
func ExitCode(res *Result, err error) int {
	var usage *UsageError
	switch {
	case errors.As(err, &usage):
		return ExitUsage
	case res != nil && len(res.Errors) > 0 && res.Errors[0].Stage == "usage":
		return ExitUsage
	case err != nil:
		return ExitFatal
	case res == nil || res.OK:
		return ExitOK
	case len(res.Outputs) > 0:
		return ExitPartial
	default:
		return ExitFatal
	}
}

func sourceEndpoint(s Spec) *Endpoint {
	e := &Endpoint{Kind: s.Source, Catalog: s.SourceCatalog, Types: s.SourceTypes}
	if len(s.SourceURIs) > 0 {
		e.URI = s.SourceURIs[0]
	}
	if len(s.SourceURIs) > 1 {
		e.URIs = s.SourceURIs
	}
	return e
}

func (c *Conversion) result() *Result {
	s := c.spec
	res := &Result{
		Schema:  ResultSchema,
		Command: "convert",
		Source:  sourceEndpoint(s),
		Sink:    &Endpoint{Kind: s.Sink, URI: s.SinkURI, Catalog: s.SinkCatalog, TableFormat: s.TableFormat},
		Scope:   scopeString(s.Scope),
		Selection: &Selection{
			LogSource: s.LogSource,
			Start:     timeString(s.Start),
			End:       timeString(s.End),
			Columns:   s.Columns,
		},
		Inputs:  []Input{},
		Streams: []Stream{},
		Outputs: []Output{},
		Errors:  []Error{},
	}
	for _, def := range c.defs {
		res.Source.Tables = append(res.Source.Tables, def.Namespace()+"."+def.Name)
	}
	return res
}

func scopeString(s datamodels.Scope) string {
	out := s.Network
	for _, p := range []string{s.Station, s.Campaign} {
		if p != "" {
			out += "/" + p
		}
	}
	return out
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// streamTally counts the rows of each stream as batches pass by, with each
// stream's first and last time. It counts nothing when the rows lack
// stream_edid or the table's time column (a --columns selection).
type streamTally struct {
	table      string
	edid, time int
	byEDID     map[string]*Stream
}

func newStreamTally(def datamodels.TableDef, schema *arrow.Schema) *streamTally {
	t := &streamTally{table: def.Namespace() + "." + def.Name, edid: -1, time: -1, byEDID: map[string]*Stream{}}
	if schema == nil {
		return t
	}
	if idx := schema.FieldIndices(datamodels.StreamEDIDColumn.Name); len(idx) == 1 {
		t.edid = idx[0]
	}
	if idx := schema.FieldIndices(def.TimeColumn); len(idx) == 1 {
		if _, ok := schema.Field(idx[0]).Type.(*arrow.TimestampType); ok {
			t.time = idx[0]
		}
	}
	return t
}

func (t *streamTally) wrap(groups iter.Seq2[Group, error]) iter.Seq2[Group, error] {
	if t.edid < 0 || t.time < 0 {
		return groups
	}
	return func(yield func(Group, error) bool) {
		for g, err := range groups {
			if err == nil {
				rdr := array.NewTableReader(g.Rows, -1)
				for rdr.Next() {
					t.add(rdr.RecordBatch())
				}
				rdr.Release()
			}
			if !yield(g, err) {
				return
			}
		}
	}
}

func (t *streamTally) add(rec arrow.RecordBatch) {
	edids, ok := rec.Column(t.edid).(*array.String)
	if !ok {
		return
	}
	times := rec.Column(t.time).(*array.Timestamp)
	toTime, err := rec.Schema().Field(t.time).Type.(*arrow.TimestampType).GetToTimeFunc()
	if err != nil {
		return
	}
	for i := range int(rec.NumRows()) {
		if !edids.IsValid(i) || !times.IsValid(i) {
			continue
		}
		edid, ts := edids.Value(i), toTime(times.Value(i)).UTC()
		s, ok := t.byEDID[edid]
		if !ok {
			s = &Stream{EDID: edid, Table: t.table, Start: ts, End: ts}
			t.byEDID[edid] = s
		}
		s.Rows++
		if ts.Before(s.Start) {
			s.Start = ts
		}
		if ts.After(s.End) {
			s.End = ts
		}
	}
}

func (t *streamTally) streams() []Stream {
	out := make([]Stream, 0, len(t.byEDID))
	for _, s := range t.byEDID {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b Stream) int {
		if a.EDID < b.EDID {
			return -1
		}
		if a.EDID > b.EDID {
			return 1
		}
		return 0
	})
	return out
}
