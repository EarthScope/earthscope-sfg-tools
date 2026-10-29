package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// Snapshot summary properties Append records with WithSource.
const (
	PropSourceFile   = "sfg.source-file"
	PropSourceSHA256 = "sfg.source-sha256"
)

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
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("hashing %s: %w", path, err)
		}
		if o.props == nil {
			o.props = iceberg.Properties{}
		}
		o.props[PropSourceFile] = path
		o.props[PropSourceSHA256] = hex.EncodeToString(h.Sum(nil))
		return nil
	}
}

// ReplaceFilter is the rows Append replaces when writing rows to def: for
// each stream_edid in rows, that stream's rows with def.TimeColumn between
// the stream's earliest and latest time in rows, inclusive.
func ReplaceFilter(def datamodels.TableDef, rows arrow.Table) (iceberg.BooleanExpression, error) {
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

	type window struct{ min, max arrow.Timestamp }
	windows := map[string]window{}
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
				w = window{t, t}
			}
			w.min, w.max = min(w.min, t), max(w.max, t)
			windows[id] = w
		}
	}
	var preds []iceberg.BooleanExpression
	for _, id := range slices.Sorted(maps.Keys(windows)) {
		w := windows[id]
		preds = append(preds, iceberg.NewAnd(
			iceberg.EqualTo(iceberg.Reference(datamodels.StreamEDIDColumn.Name), id),
			iceberg.GreaterThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.min)),
			iceberg.LessThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.max)),
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
