package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Snapshot summary properties Append records with WithSource and
// WithSources: the file and its SHA-256 for a snapshot of one file, or a
// JSON array of SourceFile for a snapshot of several.
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

// HashFile is the hex SHA-256 of the file at path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// AppendOption configures Append.
type AppendOption func(*appendOptions) error

type appendOptions struct {
	props iceberg.Properties
}

// WithSource records the raw file rows were parsed from, and its SHA-256, in
// the snapshot's summary properties (PropSourceFile, PropSourceSHA256), so
// each snapshot can be traced to its source.
func WithSource(path string) AppendOption {
	return func(o *appendOptions) error {
		sum, err := HashFile(path)
		if err != nil {
			return err
		}
		return WithSources(SourceFile{Path: path, SHA256: sum})(o)
	}
}

// WithSources records the raw files a snapshot's rows were parsed from,
// already hashed: one as WithSource does, several as a JSON array in
// PropSourceFiles.
func WithSources(files ...SourceFile) AppendOption {
	return func(o *appendOptions) error {
		if len(files) == 0 {
			return nil
		}
		if o.props == nil {
			o.props = iceberg.Properties{}
		}
		if len(files) == 1 {
			o.props[PropSourceFile] = files[0].Path
			o.props[PropSourceSHA256] = files[0].SHA256
			return nil
		}
		b, err := json.Marshal(files)
		if err != nil {
			return err
		}
		o.props[PropSourceFiles] = string(b)
		return nil
	}
}

// WithProperties adds props to the snapshot's summary properties, e.g. to
// record where copied rows came from.
func WithProperties(props map[string]string) AppendOption {
	return func(o *appendOptions) error {
		if o.props == nil {
			o.props = iceberg.Properties{}
		}
		for k, v := range props {
			o.props[k] = v
		}
		return nil
	}
}

// A Window is the time span, inclusive, of one stream's rows in a table.
type Window struct{ Min, Max arrow.Timestamp }

// Overlaps reports whether w and v share a time.
func (w Window) Overlaps(v Window) bool { return w.Min <= v.Max && v.Min <= w.Max }

// ReplaceFilter is the rows Append replaces when writing rows to def: for
// each stream_edid in rows, that stream's rows with def.TimeColumn between
// the stream's earliest and latest time in rows, inclusive.
func ReplaceFilter(def datamodels.TableDef, rows arrow.Table) (iceberg.BooleanExpression, error) {
	windows, err := StreamWindows(def, rows)
	if err != nil {
		return nil, err
	}
	return windowFilter(def, []map[string]Window{windows})
}

// StreamWindows is the time span of each stream_edid's rows in rows.
func StreamWindows(def datamodels.TableDef, rows arrow.Table) (map[string]Window, error) {
	if def.TimeColumn == "" {
		return nil, fmt.Errorf("table has no TimeColumn")
	}
	sc := rows.Schema()
	edidIdx, timeIdx := sc.FieldIndices(datamodels.StreamEDIDColumn.Name), sc.FieldIndices(def.TimeColumn)
	if len(edidIdx) != 1 || len(timeIdx) != 1 {
		return nil, fmt.Errorf("rows need one %s and one %s column", datamodels.StreamEDIDColumn.Name, def.TimeColumn)
	}
	ts, ok := sc.Field(timeIdx[0]).Type.(*arrow.TimestampType)
	if !ok || ts.Unit != arrow.Microsecond {
		return nil, fmt.Errorf("%s is %s, want a microsecond timestamp", def.TimeColumn, sc.Field(timeIdx[0]).Type)
	}
	edidType := sc.Field(edidIdx[0]).Type
	if edidType.ID() != arrow.STRING {
		return nil, fmt.Errorf("%s is %s, want a string", datamodels.StreamEDIDColumn.Name, edidType)
	}

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
					datamodels.StreamEDIDColumn.Name, def.TimeColumn)
			}
			id, t := e.Value(i), tm.Value(i)
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

// windowFilter matches, for each part, each of its streams' rows within
// the stream's window in that part.
func windowFilter(def datamodels.TableDef, parts []map[string]Window) (iceberg.BooleanExpression, error) {
	var preds []iceberg.BooleanExpression
	for _, windows := range parts {
		for _, id := range slices.Sorted(maps.Keys(windows)) {
			w := windows[id]
			preds = append(preds, iceberg.NewAnd(
				iceberg.EqualTo(iceberg.Reference(datamodels.StreamEDIDColumn.Name), id),
				iceberg.GreaterThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.Min)),
				iceberg.LessThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.Max)),
			))
		}
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
