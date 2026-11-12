package catalog

import (
	"context"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go/table"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Table is one Iceberg table and the definition it was opened with. It
// knows nothing of the Database or Catalog that opened it. Commits through
// it (Append, Batch.Commit) move it to the new snapshot. It is not safe for
// concurrent use.
type Table struct {
	def datamodels.TableDef
	tbl *table.Table
}

// Def is the table's definition.
func (t *Table) Def() datamodels.TableDef { return t.def }

// Iceberg is the underlying iceberg-go table, at the snapshot the Table
// last read or committed; for readers built on iceberg-go (gnsstools).
func (t *Table) Iceberg() *table.Table { return t.tbl }

// Refresh moves the table to its latest snapshot in the catalog, picking up
// other writers' commits. A Table otherwise sees only the snapshot it was
// opened at and its own commits, so a commit after someone else's fails as
// a conflict until it is refreshed.
func (t *Table) Refresh(ctx context.Context) error {
	if err := t.tbl.Refresh(ctx); err != nil {
		return fmt.Errorf("refreshing %s: %w", t.def.Name, err)
	}
	return nil
}

// inTableOrder returns rows relabelled with tbl's own schema: its column
// order and field IDs, matched by name. Rows built from a TableDef carry the
// def's field IDs (declaration order) in their Arrow field metadata, and
// iceberg-go maps written columns by those IDs; an evolved table (see
// evolve) numbers its added columns after the ones it had, so def-labelled
// rows would land values in the wrong columns. A table column missing from
// rows is written as nulls if optional; a rows column the table lacks, or a
// type mismatch, is an error. The caller releases the result.
func inTableOrder(tbl *table.Table, rows arrow.Table) (arrow.Table, error) {
	n := rows.NumRows()
	tsc, err := table.SchemaToArrowSchema(tbl.Schema(), nil, true, false)
	if err != nil {
		return nil, err
	}
	outFields := tsc.Fields()
	cols := make([]arrow.Column, len(outFields))
	release := func() {
		for i := range cols {
			if cols[i].Data() != nil {
				cols[i].Release()
			}
		}
	}
	used := 0
	for i, f := range outFields {
		idx := rows.Schema().FieldIndices(f.Name)
		switch {
		case len(idx) == 1:
			data := rows.Column(idx[0]).Data()
			if !arrow.TypeEqual(data.DataType(), f.Type) {
				release()
				return nil, fmt.Errorf("column %q is %s, table has %s", f.Name, data.DataType(), f.Type)
			}
			cols[i] = *arrow.NewColumn(f, data)
			used++
		case !f.Nullable:
			release()
			return nil, fmt.Errorf("rows lack required column %q", f.Name)
		default:
			nulls := array.MakeArrayOfNull(memory.DefaultAllocator, f.Type, int(n))
			chunked := arrow.NewChunked(f.Type, []arrow.Array{nulls})
			nulls.Release()
			cols[i] = *arrow.NewColumn(f, chunked)
			chunked.Release()
		}
	}
	if used != int(rows.NumCols()) {
		release()
		return nil, fmt.Errorf("rows have columns the table lacks")
	}
	out := array.NewTable(tsc, cols, n)
	release()
	return out, nil
}

// appendBatchRows is the row count per write batch when appending.
const appendBatchRows = 1 << 20

// Append writes a full-row Arrow table (a file parser's output for the
// table's definition) as one snapshot, replacing rather than adding to what is already there: for
// each stream_edid in rows, existing rows of that stream whose
// TimeColumn falls within the stream's time range in rows are deleted in
// the same commit (see ReplaceFilter). Re-ingesting a file therefore leaves
// the table as one copy of its rows, and re-ingesting it with a fixed parser
// replaces the old output.
//
// This assumes files of one stream do not overlap in time: a file whose
// range overlaps another file of the same stream replaces that file's rows
// in the overlap. Only the new rows' range is replaced, so if a re-parse
// covers a narrower range than before, the old rows outside it remain.
func (t *Table) Append(ctx context.Context, rows arrow.Table, opts ...AppendOption) (int64, error) {
	b := t.NewBatch()
	if _, err := b.Append(ctx, rows, opts...); err != nil {
		return 0, err
	}
	return b.Commit(ctx)
}

// A Batch stages appends to one table and commits them together: each
// Append is its own snapshot, with its own summary properties, as with
// Table.Append, but the catalog is updated once, at Commit. Each staged
// Append sees the ones before it, so a batch leaves the table as the same
// appends committed one by one would. Staged rows are written to storage at
// Append; only the commit is deferred.
type Batch struct {
	t      *Table
	def    datamodels.TableDef
	tx     *table.Transaction
	staged int64 // rows staged since the last Commit
	n      int   // appends staged since the last Commit
}

// NewBatch starts a batch of appends to the table. Only one batch of a
// table may be open at a time: each commits from the snapshot it started at.
func (t *Table) NewBatch() *Batch {
	return &Batch{t: t, def: t.def, tx: t.tbl.NewTransaction()}
}

// Append stages rows as one snapshot, as Table.Append would commit them,
// and returns the rows staged. An error before anything is staged leaves
// the batch as it was; Failed reports whether an error left it unusable.
func (b *Batch) Append(ctx context.Context, rows arrow.Table, opts ...AppendOption) (int64, error) {
	return b.AppendParts(ctx, []arrow.Table{rows}, opts...)
}

// AppendParts stages parts (e.g. several files' rows) as one snapshot: each
// part replaces its own streams' windows (see StreamWindows), as if appended
// alone, but they are written together. Parts must not overlap: two parts
// with rows of one stream at a shared time would both be kept, where
// appending them in turn keeps the second.
func (b *Batch) AppendParts(ctx context.Context, parts []arrow.Table, opts ...AppendOption) (int64, error) {
	var windows []map[string]Window
	var recs []arrow.RecordBatch
	defer func() {
		for _, r := range recs {
			r.Release()
		}
	}()
	var schema *arrow.Schema
	for _, rows := range parts {
		if rows.NumRows() == 0 {
			continue
		}
		if err := b.def.Validate(rows); err != nil {
			return 0, err
		}
		w, err := StreamWindows(b.def, rows)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", b.def.Name, err)
		}
		windows = append(windows, w)
		if schema == nil {
			schema = rows.Schema()
		} else if !schema.Equal(rows.Schema()) {
			return 0, fmt.Errorf("%s: parts have different schemas", b.def.Name)
		}
		rdr := array.NewTableReader(rows, -1)
		for rdr.Next() {
			rec := rdr.RecordBatch()
			rec.Retain()
			recs = append(recs, rec)
		}
		rdr.Release()
	}
	if len(windows) == 0 {
		return 0, nil
	}
	if b.tx == nil {
		return 0, fmt.Errorf("%s: batch failed earlier", b.def.Name)
	}
	var o appendOptions
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return 0, err
		}
	}
	filter, err := windowFilter(b.def, windows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", b.def.Name, err)
	}
	rows := array.NewTableFromRecords(schema, recs)
	defer rows.Release()
	ordered, err := inTableOrder(b.t.tbl, rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", b.def.Name, err)
	}
	defer ordered.Release()
	if err := b.tx.OverwriteTable(ctx, ordered, appendBatchRows, o.props, table.WithOverwriteFilter(filter)); err != nil {
		b.tx = nil // the transaction may hold part of this write
		return 0, fmt.Errorf("writing %s: %w", b.def.Name, err)
	}
	b.staged += rows.NumRows()
	b.n++
	return rows.NumRows(), nil
}

// Failed reports whether an Append error left the batch unable to stage
// or commit; what it had staged is lost.
func (b *Batch) Failed() bool { return b.tx == nil }

// Staged is the number of appends staged since the last Commit.
func (b *Batch) Staged() int { return b.n }

// Commit commits the staged appends in one catalog update and returns
// their rows; the Table then reads the new snapshot. The batch can then
// stage more.
func (b *Batch) Commit(ctx context.Context) (int64, error) {
	if b.tx == nil {
		return 0, fmt.Errorf("%s: batch failed earlier", b.def.Name)
	}
	if b.n == 0 {
		return 0, nil
	}
	updated, err := b.tx.Commit(ctx)
	b.n, b.tx = 0, nil
	if err != nil {
		return 0, fmt.Errorf("committing %s: %w", b.def.Name, err)
	}
	n := b.staged
	b.staged = 0
	b.t.tbl, b.tx = updated, updated.NewTransaction()
	return n, nil
}
