package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// The properties that record the raw files rows were parsed from: the file
// and its SHA-256 for rows of one file, or a JSON array of SourceFile for
// rows of several. WithSources puts them in a snapshot summary; the files
// Writer puts them in a Parquet file's key-value metadata (SourceProps).
const (
	PropSourceFile   = "sfg.source-file"
	PropSourceSHA256 = "sfg.source-sha256"
	PropSourceFiles  = "sfg.source-files"
)

// A SourceFile is a raw file a snapshot's rows were parsed from.
type SourceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// HashFile is the file at path with its hex SHA-256.
func HashFile(path string) (SourceFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return SourceFile{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return SourceFile{}, fmt.Errorf("hashing %s: %w", path, err)
	}
	return SourceFile{Path: path, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// SourceProps are the properties recording files, already hashed: one as
// PropSourceFile and PropSourceSHA256, several as a JSON array in
// PropSourceFiles; none is no properties.
func SourceProps(files ...SourceFile) map[string]string {
	switch len(files) {
	case 0:
		return nil
	case 1:
		return map[string]string{PropSourceFile: files[0].Path, PropSourceSHA256: files[0].SHA256}
	}
	b, _ := json.Marshal(files) // strings only: cannot fail
	return map[string]string{PropSourceFiles: string(b)}
}

// SourcesOf reads back the files SourceProps recorded in props, looked up
// with get (which reports whether a key is present); nil when none are.
func SourcesOf(get func(key string) (string, bool)) ([]SourceFile, error) {
	if v, ok := get(PropSourceFiles); ok {
		var files []SourceFile
		if err := json.Unmarshal([]byte(v), &files); err != nil {
			return nil, fmt.Errorf("%s: %w", PropSourceFiles, err)
		}
		return files, nil
	}
	path, ok1 := get(PropSourceFile)
	sha, ok2 := get(PropSourceSHA256)
	if !ok1 && !ok2 {
		return nil, nil
	}
	return []SourceFile{{Path: path, SHA256: sha}}, nil
}

// WithSources records the raw files a snapshot's rows were parsed from,
// already hashed, in its summary (see SourceProps).
func WithSources(files ...SourceFile) AppendOption {
	return func(o *appendOptions) {
		props := SourceProps(files...)
		if len(props) == 0 {
			return
		}
		if o.props == nil {
			o.props = iceberg.Properties{}
		}
		maps.Copy(o.props, props)
	}
}

// A Window is the time span, inclusive, of one stream's rows in a table.
type Window struct{ Min, Max arrow.Timestamp }

// Replace writes a full-row Arrow table (a file parser's output for the
// table's definition) as one snapshot, replacing rather than adding to what
// is already there: for each stream_edid in rows, existing rows of that
// stream whose TimeColumn falls within the stream's time range in rows are
// deleted in the same commit. Re-writing a file therefore leaves the table
// as one copy of its rows, and re-writing it with a fixed parser replaces
// the old output. It returns the number of rows written.
//
// This assumes files of one stream do not overlap in time: a file whose
// range overlaps another file of the same stream replaces that file's rows
// in the overlap. Only the new rows' range is replaced, so if a re-parse
// covers a narrower range than before, the old rows outside it remain.
func (t *Table) Replace(ctx context.Context, rows arrow.Table, opts ...AppendOption) (int64, error) {
	n := rows.NumRows()
	if n == 0 {
		return 0, nil
	}
	var o appendOptions
	for _, opt := range opts {
		opt(&o)
	}
	if err := t.def.Validate(rows); err != nil {
		return 0, err
	}
	windows, err := StreamWindows(t.def, rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", t.def.Name, err)
	}
	filter, err := windowFilter(t.def, windows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", t.def.Name, err)
	}
	ordered, err := inTableOrder(t.tbl, rows)
	if err != nil {
		return 0, fmt.Errorf("replacing in %s: %w", t.def.Name, err)
	}
	defer ordered.Release()
	tx := t.tbl.NewTransaction()
	if err := tx.OverwriteTable(ctx, ordered, appendBatchRows, o.props, table.WithOverwriteFilter(filter)); err != nil {
		return 0, fmt.Errorf("replacing in %s: %w", t.def.Name, err)
	}
	tbl, err := tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("committing to %s: %w", t.def.Name, err)
	}
	t.tbl = tbl
	return n, nil
}

// EarliestTime is the earliest time a row may have: the start of GPS time,
// 1980-01-06T00:00:00Z. A row before it (e.g. a packet with no time, read
// as the Unix epoch), or more than FutureSlack after now, is not a real
// observation time.
var EarliestTime = time.Date(1980, 1, 6, 0, 0, 0, 0, time.UTC)

// FutureSlack is how far after now a row's time may be.
const FutureSlack = 24 * time.Hour

// StreamWindows is the time span of each stream_edid's rows in rows. A time
// before EarliestTime or after now+FutureSlack is an error naming the
// stream, the column and the value: Replace deletes each stream's existing
// rows over its window, so one bad time would widen the window back to 1970
// (or into the future) and delete the rows of every other file of the
// stream. The batch fails instead; its rows are not dropped silently.
func StreamWindows(def TableDef, rows arrow.Table) (map[string]Window, error) {
	if def.TimeColumn == "" {
		return nil, fmt.Errorf("table has no TimeColumn")
	}
	sc := rows.Schema()
	edidIdx, timeIdx := sc.FieldIndices(StreamEDIDName), sc.FieldIndices(def.TimeColumn)
	if len(edidIdx) != 1 || len(timeIdx) != 1 {
		return nil, fmt.Errorf("rows need one %s and one %s column", StreamEDIDName, def.TimeColumn)
	}
	ts, ok := sc.Field(timeIdx[0]).Type.(*arrow.TimestampType)
	if !ok || ts.Unit != arrow.Microsecond {
		return nil, fmt.Errorf("%s is %s, want a microsecond timestamp", def.TimeColumn, sc.Field(timeIdx[0]).Type)
	}
	edidType := sc.Field(edidIdx[0]).Type
	if edidType.ID() != arrow.STRING {
		return nil, fmt.Errorf("%s is %s, want a string", StreamEDIDName, edidType)
	}

	toTime, err := ts.GetToTimeFunc()
	if err != nil {
		return nil, err
	}
	earliest := arrow.Timestamp(EarliestTime.UnixMicro())
	latest := arrow.Timestamp(time.Now().Add(FutureSlack).UnixMicro())

	windows := map[string]Window{}
	rdr := array.NewTableReader(rows, -1) // batches with aligned columns
	defer rdr.Release()
	for rdr.Next() {
		rec := rdr.RecordBatch()
		e := rec.Column(edidIdx[0]).(*array.String)
		tm := rec.Column(timeIdx[0]).(*array.Timestamp)
		for i := range e.Len() {
			if e.IsNull(i) || tm.IsNull(i) {
				return nil, fmt.Errorf("row has a null %s or %s; cannot replace it idempotently",
					StreamEDIDName, def.TimeColumn)
			}
			id, t := e.Value(i), tm.Value(i)
			if t < earliest || t > latest {
				return nil, fmt.Errorf("stream %s: %s %s is outside %s to now+%s; refusing to replace the stream over that span",
					id, def.TimeColumn, toTime(t).UTC().Format(time.RFC3339Nano), EarliestTime.Format(time.RFC3339), FutureSlack)
			}
			w, ok := windows[id]
			if !ok {
				w = Window{t, t}
			}
			w.Min, w.Max = min(w.Min, t), max(w.Max, t)
			windows[id] = w
		}
	}
	return windows, nil
}

// windowFilter matches each stream's rows within the stream's window.
func windowFilter(def TableDef, windows map[string]Window) (iceberg.BooleanExpression, error) {
	var preds []iceberg.BooleanExpression
	for _, id := range slices.Sorted(maps.Keys(windows)) {
		w := windows[id]
		preds = append(preds, iceberg.NewAnd(
			iceberg.EqualTo(iceberg.Reference(StreamEDIDName), id),
			iceberg.GreaterThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.Min)),
			iceberg.LessThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.Max)),
		))
	}
	switch len(preds) {
	case 0:
		return nil, fmt.Errorf("no rows")
	case 1:
		return preds[0], nil
	default:
		return iceberg.NewOr(preds[0], preds[1], preds[2:]...), nil
	}
}
