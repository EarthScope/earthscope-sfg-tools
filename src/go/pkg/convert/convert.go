// Package convert moves rows from one source to one sink: the library
// behind `sfg convert`. Every part of a conversion is stated explicitly;
// nothing is inferred from paths, file names or extensions.
//
// A Source yields tables of rows; a Sink writes them and reports what it
// wrote. CheckFlags and New reject a conversion before any work starts;
// Run carries it out and returns its Result.
package convert

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Source and sink kinds.
const (
	SourceIceberg = "iceberg"
	SinkFiles     = "files"
	SinkIceberg   = "iceberg"
)

// Spec is one conversion as given on the command line.
type Spec struct {
	Source        string   // SourceIceberg or SourceRaw
	SourceURIs    []string // the warehouse: a directory (catalog.Local) or overriding the catalog's; or raw files: files, globs or directories
	SourceCatalog string   // catalog.Local or a catalog of the iceberg-go configuration
	Sink          string   // SinkFiles or SinkIceberg
	SinkURI       string   // a local directory; or the sink warehouse: a directory (catalog.Local) or overriding the catalog's
	SinkCatalog   string   // catalog.Local or a catalog of the iceberg-go configuration
	TableFormat   string   // FormatParquet, FormatCSV or FormatArrow

	Scope       datamodels.Scope
	Tables      []string // bare or namespace-qualified table names
	SourceTypes []string // raw file types; for Iceberg, one file type, or product for Level 1/2 tables
	LogSource   string   // log source, or product input for Level 1/2 tables
	Start, End  time.Time
	Columns     []string
	MaxFiles    int // raw: at most this many files of each type; 0 is all

	// TypesFromNames (raw) tells each file's type by its name instead of
	// taking the files as SourceTypes: SourceTypes then only limits the
	// types read, all by default, and named files of no type are skipped.
	TypesFromNames bool

	FilesPerCommit   int // Iceberg sink: snapshots per catalog commit; 0 is all of a table's
	FilesPerSnapshot int // Iceberg sink: groups (files) merged into one snapshot; 0 or 1 is one each
	Workers          int // raw: files parsed at once; 0 is runtime.NumCPU()
}

// A UsageError is a conversion stated wrongly: a missing, inapplicable or
// invalid flag. It is found before any work starts.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, args ...any) error {
	return &UsageError{fmt.Errorf(format, args...)}
}

// endpoint is the flags a source or sink kind takes, by flag name. A sink's
// rejects are flags its source would take but it can't honour.
type endpoint struct{ required, optional, rejects []string }

// alwaysRequired are the flags every conversion needs; alwaysAccepted
// are flags any conversion may take.
var (
	alwaysRequired = []string{"source", "source-uri", "sink", "sink-uri", "scope"}
	alwaysAccepted = []string{"result"}
)

var sources = map[string]endpoint{
	SourceIceberg: {
		required: []string{"source-catalog", "table"},
		optional: []string{"source-type", "log-source", "start", "end", "columns"},
	},
	SourceRaw: {
		required: []string{"source-type"},
		optional: []string{"table", "log-source", "max-files", "workers"},
	},
}

var sinks = map[string]endpoint{
	SinkFiles:   {required: []string{"table-format"}},
	SinkIceberg: {required: []string{"sink-catalog"}, optional: []string{"files-per-commit", "files-per-snapshot"}, rejects: []string{"columns"}},
}

func kinds(m map[string]endpoint) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return strings.Join(ks, ", ")
}

// CheckFlags checks the names of the flags given (without dashes) against
// what the source and sink kinds take: every required flag present, and
// none that the pair doesn't take.
func CheckFlags(source, sink string, given []string) error {
	src, ok := sources[source]
	if !ok {
		return usagef("--source %q: want one of %s", source, kinds(sources))
	}
	snk, ok := sinks[sink]
	if !ok {
		return usagef("--sink %q: want one of %s", sink, kinds(sinks))
	}
	var missing, extra []string
	for _, name := range slices.Concat(alwaysRequired, src.required, snk.required) {
		if !slices.Contains(given, name) {
			missing = append(missing, "--"+name)
		}
	}
	takes := slices.Concat(alwaysRequired, alwaysAccepted, src.required, src.optional, snk.required, snk.optional)
	for _, name := range given {
		if !slices.Contains(takes, name) || slices.Contains(snk.rejects, name) {
			extra = append(extra, "--"+name)
		}
	}
	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("--source %s --sink %s needs %s", source, sink, strings.Join(missing, ", ")))
	}
	if len(extra) > 0 {
		errs = append(errs, fmt.Errorf("%s doesn't apply to --source %s --sink %s", strings.Join(extra, ", "), source, sink))
	}
	if err := errors.Join(errs...); err != nil {
		return &UsageError{err}
	}
	return nil
}

// ParseScope parses NET, NET/STA or NET/STA/CAMP.
func ParseScope(s string) (datamodels.Scope, error) {
	parts := strings.Split(s, "/")
	if len(parts) > 3 || slices.Contains(parts, "") {
		return datamodels.Scope{}, usagef("--scope %q: want NET, NET/STA or NET/STA/CAMP", s)
	}
	parts = append(parts, "", "")
	sc := datamodels.Scope{Network: parts[0], Station: parts[1], Campaign: parts[2]}
	if err := datamodels.CheckScope(sc); err != nil {
		return datamodels.Scope{}, usagef("--scope: %w", err)
	}
	return sc, nil
}

// Conversion is a checked Spec, ready to run.
type Conversion struct {
	spec   Spec
	defs   []datamodels.TableDef // the tables the source reads
	source Source
	sink   Sink
}

// New checks spec's values and builds its source and sink. An Iceberg
// source or sink without a URI takes its catalog's configured warehouse
// (see catalog.Warehouse). Every error is a *UsageError.
func New(spec Spec) (*Conversion, error) {
	if _, ok := sources[spec.Source]; !ok {
		return nil, usagef("--source %q: want one of %s", spec.Source, kinds(sources))
	}
	if _, ok := sinks[spec.Sink]; !ok {
		return nil, usagef("--sink %q: want one of %s", spec.Sink, kinds(sinks))
	}
	if spec.Source == SourceIceberg && len(spec.SourceURIs) == 0 {
		if wh := catalog.Warehouse(spec.SourceCatalog); wh != "" {
			spec.SourceURIs = []string{wh}
		}
	}
	if spec.Sink == SinkIceberg && spec.SinkURI == "" {
		spec.SinkURI = catalog.Warehouse(spec.SinkCatalog)
	}
	if len(spec.SourceURIs) == 0 || slices.Contains(spec.SourceURIs, "") {
		return nil, usagef("--source-uri is required: %s", uriHint(spec.Source, spec.SourceCatalog, "source"))
	}
	if spec.SinkURI == "" {
		return nil, usagef("--sink-uri is required: %s", uriHint(spec.Sink, spec.SinkCatalog, "sink"))
	}
	c := &Conversion{spec: spec}
	if spec.Scope.Network == "" {
		return nil, usagef("--scope is required")
	}

	switch spec.Source {
	case SourceIceberg:
		src, err := newIcebergSource(spec)
		if err != nil {
			return nil, err
		}
		c.source, c.defs = src, src.defs
	case SourceRaw:
		src, err := newRawSource(spec)
		if err != nil {
			return nil, err
		}
		c.source, c.defs = src, src.defs
	}

	switch spec.Sink {
	case SinkFiles:
		// Files are named after each table's bare name.
		for i, def := range c.defs {
			for _, prev := range c.defs[:i] {
				if prev.Name == def.Name {
					return nil, usagef("--table: %s.%s and %s.%s would both write %s files",
						prev.Namespace(), prev.Name, def.Namespace(), def.Name, def.Name)
				}
			}
		}
		if !slices.Contains([]string{FormatParquet, FormatCSV, FormatArrow}, spec.TableFormat) {
			return nil, usagef("--table-format %q: want %s, %s or %s", spec.TableFormat, FormatParquet, FormatCSV, FormatArrow)
		}
		c.sink = &filesSink{dir: spec.SinkURI, format: spec.TableFormat}
	case SinkIceberg:
		if err := catalog.Check(spec.SinkCatalog); err != nil {
			return nil, usagef("--sink-catalog: %w", err)
		}
		var props map[string]string
		if src, ok := c.source.(*icebergSource); ok {
			if spec.SourceCatalog == spec.SinkCatalog && sameWarehouse(spec.SourceURIs[0], spec.SinkURI) {
				return nil, usagef("--source-uri and --sink-uri are the same %s warehouse: a table can't be copied onto itself", spec.SinkCatalog)
			}
			src.byStream = true
			props = map[string]string{
				PropCopySource:    spec.SourceCatalog + ":" + spec.SourceURIs[0],
				PropCopySelection: selectionString(spec),
			}
		}
		if spec.FilesPerCommit < 0 {
			return nil, usagef("--files-per-commit %d: want 0 (all) or more", spec.FilesPerCommit)
		}
		if spec.FilesPerSnapshot < 0 {
			return nil, usagef("--files-per-snapshot %d: want 0 or more", spec.FilesPerSnapshot)
		}
		c.sink = &icebergSink{cfg: catalog.Config{Name: spec.SinkCatalog, Warehouse: spec.SinkURI},
			props: props, perCommit: spec.FilesPerCommit, perSnapshot: max(spec.FilesPerSnapshot, 1)}
	}
	return c, nil
}

// Snapshot summary properties of rows copied from another catalog.
const (
	PropCopySource    = "sfg.copy-source"    // <catalog>:<warehouse>
	PropCopySelection = "sfg.copy-selection" // the --scope, --source-type, --log-source, --start and --end used
)

// sameWarehouse reports whether two warehouse URIs name the same place.
// uriHint says why an endpoint of kind needs its URI.
func uriHint(kind, cat, side string) string {
	switch {
	case kind == SourceRaw:
		return "the raw files, globs or directories"
	case kind == SinkFiles:
		return "the output directory"
	case cat == catalog.Local:
		return "the local warehouse directory"
	default:
		return fmt.Sprintf("--%s-catalog %q has no warehouse in the iceberg-go configuration", side, cat)
	}
}

func sameWarehouse(a, b string) bool {
	clean := func(u string) string {
		if strings.Contains(u, "://") {
			return strings.TrimSuffix(u, "/")
		}
		if abs, err := filepath.Abs(u); err == nil {
			return abs
		}
		return filepath.Clean(u)
	}
	return clean(a) == clean(b)
}

// selectionString is spec's row selection as flags, for a snapshot summary.
func selectionString(spec Spec) string {
	parts := []string{"--scope " + scopeString(spec.Scope)}
	if len(spec.SourceTypes) > 0 {
		parts = append(parts, "--source-type "+strings.Join(spec.SourceTypes, ","))
	}
	if spec.LogSource != "" {
		parts = append(parts, "--log-source "+spec.LogSource)
	}
	if !spec.Start.IsZero() {
		parts = append(parts, "--start "+timeString(spec.Start))
	}
	if !spec.End.IsZero() {
		parts = append(parts, "--end "+timeString(spec.End))
	}
	return strings.Join(parts, " ")
}

// Rows is one table's rows from a source: its schema, then its groups.
// A table the source couldn't read has Err set and no groups.
type Rows struct {
	Def    datamodels.TableDef
	Schema *arrow.Schema
	Groups iter.Seq2[Group, error]
	Err    error
}

// A Group is rows a sink writes as one unit: an Iceberg sink appends each
// group as one snapshot, replacing each of its streams' rows over the
// group's time span. A raw source makes one group per parsed file.
type Group struct {
	Rows   arrow.Table
	Source string // the file the rows came from, if any
	SHA256 string // Source's hex SHA-256, if the source hashed it
}

// A Source reads tables of rows.
type Source interface {
	// Read calls write once per table, in order. An error from Read itself
	// (the source can't be opened) is fatal to the run.
	Read(ctx context.Context, write func(Rows)) error
}

// A Sink writes tables of rows.
type Sink interface {
	// Write writes one table's rows and returns what it wrote. It releases
	// each group it takes, and may stop taking groups after an error.
	Write(ctx context.Context, rows Rows) ([]Output, error)
	// Close releases what Write opened.
	Close() error
}

// Run carries out the conversion. A table that fails is recorded in the
// result and the run moves on; the returned error is a fatal one, after
// which the result holds what was done before it.
func (c *Conversion) Run(ctx context.Context) (*Result, error) {
	res := c.result()
	defer func() {
		if err := c.sink.Close(); err != nil {
			res.addError("sink", c.spec.SinkURI, err)
			res.OK = false
		}
	}()
	err := c.source.Read(ctx, func(rows Rows) {
		name := rows.Def.Namespace() + "." + rows.Def.Name
		if rows.Err != nil {
			res.addError("read", name, rows.Err)
			return
		}
		tally := newStreamTally(rows.Def, rows.Schema)
		rows.Groups = tally.wrap(rows.Groups)
		outs, err := c.sink.Write(ctx, rows)
		res.Streams = append(res.Streams, tally.streams()...)
		res.Outputs = append(res.Outputs, outs...)
		if err != nil {
			res.addError("write", name, err)
		}
	})
	if in, ok := c.source.(interface{ Inputs() []Input }); ok {
		res.Inputs = append(res.Inputs, in.Inputs()...)
		for _, i := range in.Inputs() {
			if i.Status == "failed" {
				res.Errors = append(res.Errors, Error{Stage: "parse", Subject: i.Path, Message: i.Error})
			}
		}
	}
	if err != nil {
		res.addError("source", strings.Join(c.spec.SourceURIs, " "), err)
	}
	res.OK = len(res.Errors) == 0
	return res, err
}
