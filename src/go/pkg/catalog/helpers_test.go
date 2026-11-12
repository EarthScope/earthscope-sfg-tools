package catalog

import (
	"context"
	"iter"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// openTable is def's table in its SFG database.
func openTable(_ context.Context, cat *Catalog, def datamodels.TableDef) (*Table, error) {
	return cat.TableOf(def)
}

// appendRows appends rows to def's table in its SFG database.
func appendRows(ctx context.Context, cat *Catalog, def datamodels.TableDef, rows arrow.Table, opts ...AppendOption) (int64, error) {
	tbl, err := cat.TableOf(def)
	if err != nil {
		return 0, err
	}
	return tbl.Append(ctx, rows, opts...)
}

// newBatch starts a batch on def's table in its SFG database.
func newBatch(_ context.Context, cat *Catalog, def datamodels.TableDef) (*Batch, error) {
	tbl, err := cat.TableOf(def)
	if err != nil {
		return nil, err
	}
	return tbl.NewBatch(), nil
}

// scanTable reads sel from def's table in its SFG database.
func scanTable(ctx context.Context, cat *Catalog, def datamodels.TableDef, sel Selection) (*arrow.Schema, iter.Seq2[arrow.RecordBatch, error], error) {
	tbl, err := cat.TableOf(def)
	if err != nil {
		return nil, nil, err
	}
	return tbl.Scan(ctx, sel)
}
