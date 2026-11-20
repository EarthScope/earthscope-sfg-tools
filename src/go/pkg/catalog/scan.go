package catalog

import (
	"context"
	"fmt"
	"iter"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// Scan reads the rows matching filter (nil: all) from the table, at the
// snapshot it last read or committed, batch by batch, with the columns in
// columns order (all, in table order, when empty). The caller releases each
// batch. Open with Load to read without creating anything.
func (t *Table) Scan(ctx context.Context, filter iceberg.BooleanExpression, columns []string) (*arrow.Schema, iter.Seq2[arrow.RecordBatch, error], error) {
	var opts []table.ScanOption
	if filter != nil {
		opts = append(opts, table.WithRowFilter(filter))
	}
	if len(columns) > 0 {
		opts = append(opts, table.WithSelectedFields(columns...))
	}
	schema, batches, err := t.tbl.Scan(opts...).ToArrowRecords(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("scanning %s: %w", t.def.Name, err)
	}
	if len(columns) == 0 {
		return schema, batches, nil
	}
	// The scan returns columns in table order; return them in columns'.
	order := make([]int, len(columns))
	fields := make([]arrow.Field, len(columns))
	for i, name := range columns {
		idx := schema.FieldIndices(name)
		if len(idx) != 1 {
			return nil, nil, fmt.Errorf("%s has no column %q", t.def.Name, name)
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
