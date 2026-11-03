package catalog

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Selection picks the rows of a table to read. Network is required;
// Station and Campaign narrow the scope, and Campaign needs Station.
//
// FileType and LogSource pick streams (see datamodels.StreamEDID) and need
// the full scope; LogSource needs FileType. In a derived table they are the
// product and its input (see datamodels.DerivedEDID), e.g. PRIDE and
// NOV000:RANGEA. Scope, FileType and LogSource all match a prefix of
// stream_edid, the table's only identity column, which it is partitioned on.
//
// Start and End bound the table's TimeColumn, [Start, End); zero is
// unbounded. For gnss_observations the time column is time_gpst, GPS time.
type Selection struct {
	Scope      datamodels.Scope
	FileType   string
	LogSource  string
	Start, End time.Time
	Columns    []string // in this order; all, in table order, when empty
}

// Filter is sel as a row filter on def.
func (sel Selection) Filter(def datamodels.TableDef) (iceberg.BooleanExpression, error) {
	s := sel.Scope
	switch {
	case s.Network == "":
		return nil, errors.New("selection needs a network")
	case s.Campaign != "" && s.Station == "":
		return nil, errors.New("selection by campaign needs a station")
	case sel.FileType != "" && s.Campaign == "":
		return nil, errors.New("selection by file type needs the full scope (network, station, campaign)")
	case sel.LogSource != "" && sel.FileType == "":
		return nil, errors.New("selection by log source needs its file type")
	}
	edid := iceberg.Reference(datamodels.StreamEDIDColumn.Name)

	streamEDID := datamodels.StreamEDID
	if def.Level > 1 {
		streamEDID = datamodels.DerivedEDID
	}
	var preds []iceberg.BooleanExpression
	switch {
	case sel.LogSource != "":
		preds = append(preds, iceberg.EqualTo(edid, streamEDID(s, sel.FileType, sel.LogSource)))
	case sel.FileType != "":
		preds = append(preds, hasPrefix(edid, streamEDID(s, sel.FileType, "")))
	default:
		prefix := s.Network + ":"
		for _, p := range []string{s.Station, s.Campaign} {
			if p != "" {
				prefix += p + ":"
			}
		}
		preds = append(preds, hasPrefix(edid, prefix))
	}

	if !sel.Start.IsZero() {
		preds = append(preds, iceberg.GreaterThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(sel.Start.UnixMicro())))
	}
	if !sel.End.IsZero() {
		preds = append(preds, iceberg.LessThan(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(sel.End.UnixMicro())))
	}
	if len(preds) == 1 {
		return preds[0], nil
	}
	return iceberg.NewAnd(preds[0], preds[1], preds[2:]...), nil
}

// hasPrefix matches strings starting with prefix, which ends in ':'. It is
// the range [prefix, prefix with ':' replaced by ';'), since iceberg-go
// scans do not implement starts_with; the range also prunes data files by
// their column bounds.
func hasPrefix(ref iceberg.UnboundTerm, prefix string) iceberg.BooleanExpression {
	upper := strings.TrimSuffix(prefix, ":") + ";"
	return iceberg.NewAnd(iceberg.GreaterThanEqual(ref, prefix), iceberg.LessThan(ref, upper))
}

// Scan reads the rows sel selects from def's table, batch by batch, with
// the columns in sel.Columns order (all, in table order, when empty). Unlike
// Table it neither creates nor evolves the table: a table the catalog lacks
// is an error.
func (c *Catalog) Scan(ctx context.Context, def datamodels.TableDef, sel Selection) (*arrow.Schema, iter.Seq2[arrow.RecordBatch, error], error) {
	filter, err := sel.Filter(def)
	if err != nil {
		return nil, nil, err
	}
	tbl, err := c.cat.LoadTable(ctx, def.Identifier())
	if err != nil {
		return nil, nil, fmt.Errorf("loading %s.%s: %w", def.Namespace(), def.Name, err)
	}
	opts := []table.ScanOption{table.WithRowFilter(filter)}
	if len(sel.Columns) > 0 {
		opts = append(opts, table.WithSelectedFields(sel.Columns...))
	}
	schema, batches, err := tbl.Scan(opts...).ToArrowRecords(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("scanning %s: %w", def.Name, err)
	}
	if len(sel.Columns) == 0 {
		return schema, batches, nil
	}
	// The scan returns columns in table order; return them in sel's.
	order := make([]int, len(sel.Columns))
	fields := make([]arrow.Field, len(sel.Columns))
	for i, name := range sel.Columns {
		idx := schema.FieldIndices(name)
		if len(idx) != 1 {
			return nil, nil, fmt.Errorf("%s has no column %q", def.Name, name)
		}
		order[i], fields[i] = idx[0], schema.Field(idx[0])
	}
	schema = arrow.NewSchema(fields, nil)
	reordered := func(yield func(arrow.RecordBatch, error) bool) {
		for rec, err := range batches {
			if err != nil {
				yield(nil, err)
				return
			}
			cols := make([]arrow.Array, len(order))
			for i, j := range order {
				cols[i] = rec.Column(j)
			}
			out := array.NewRecordBatch(schema, cols, rec.NumRows())
			rec.Release()
			if !yield(out, nil) {
				return
			}
		}
	}
	return schema, reordered, nil
}
