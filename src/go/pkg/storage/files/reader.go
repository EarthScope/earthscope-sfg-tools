package files

import (
	"context"
	encsv "encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/compute"
	"github.com/apache/arrow-go/v18/arrow/csv"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
)

// Reader reads the layout the Writer produces, <dir>/<database>/<table>/
// <name>.<ext>, one Batch per file. A directory <database>/<table> that
// names no table of datamodels.SFG is logged and ignored.
//
// A CSV is typed by its table's Arrow schema (TableDef.ArrowSchema), so
// timestamps, floats and nulls come back as they were written; its header
// names the columns, which may be any subset in any order. A Parquet file
// keeps the Arrow schema it was written with. A Parquet file's Batch has
// the Source the Writer recorded in its metadata: the raw file its rows
// were parsed from (path and SHA-256), so provenance flows on to a sink that
// records it. A Parquet file without that record (written by another tool,
// or from rows of several raw files) and every CSV, which cannot carry it,
// have no Source. A file that cannot be read is a Batch with Err (naming the
// file) and the read goes on.
//
// Read applies sel.Tables, sel.Columns, and sel.Scope and sel.LogSources
// as a filter on stream_edid (the same rows storage.Selection.Filter picks
// from Iceberg); sel.Window is not applied. A file with no row left
// yields no Batch.
type Reader struct {
	dir    string
	format Format
	tables []catalog.TableRef
}

// NewReader makes a Reader of the format files under dir. Only CSV and
// Parquet can be read. It fails if dir is not a directory.
func NewReader(dir string, format Format) (*Reader, error) {
	switch format {
	case CSV, Parquet:
	default:
		return nil, fmt.Errorf("cannot read table format %q (want %s or %s)", format, Parquet, CSV)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	r := &Reader{dir: dir, format: format}
	dbs, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, db := range dbs {
		if !db.IsDir() {
			continue
		}
		tbls, err := os.ReadDir(filepath.Join(dir, db.Name()))
		if err != nil {
			return nil, err
		}
		for _, tb := range tbls {
			if !tb.IsDir() {
				continue
			}
			ref, err := datamodels.SFG.Find(db.Name() + "." + tb.Name())
			if err != nil {
				slog.Warn("ignoring a directory that is not a table", "path", filepath.Join(dir, db.Name(), tb.Name()), "error", err)
				continue
			}
			if len(r.filesOf(ref)) > 0 {
				r.tables = append(r.tables, ref)
			}
		}
	}
	return r, nil
}

// Tables are the tables with at least one file of the Reader's format.
func (r *Reader) Tables() []catalog.TableRef { return slices.Clone(r.tables) }

// filesOf are the files of ref's directory in the Reader's format, sorted.
func (r *Reader) filesOf(ref catalog.TableRef) []string {
	paths, _ := filepath.Glob(filepath.Join(r.dir, ref.Database, ref.Def.Name, "*."+r.format.Ext()))
	slices.Sort(paths)
	return paths
}

// Read yields one Batch per file of each selected table.
func (r *Reader) Read(ctx context.Context, sel storage.Selection, yield func(storage.Batch) error) error {
	match, err := sel.StreamMatcher()
	if err != nil {
		return err
	}
	refs := sel.Tables
	if len(refs) == 0 {
		refs = r.tables
	}
	for _, ref := range refs {
		schema, err := ref.Def.ArrowSchema()
		if err != nil {
			return err
		}
		for _, col := range sel.Columns {
			if !schema.HasField(col) {
				return fmt.Errorf("%s has no column %q", ref, col)
			}
		}
	}
	for _, ref := range refs {
		for _, path := range r.filesOf(ref) {
			if err := ctx.Err(); err != nil {
				return err
			}
			b := r.readFile(ctx, ref, path, sel.Columns, match)
			if b.Err == nil && b.Table.NumRows() == 0 {
				b.Table.Release()
				continue
			}
			if err := yield(b); err != nil {
				return err
			}
		}
	}
	return nil
}

// readFile reads one file into a Batch, whose Err is set on failure.
func (r *Reader) readFile(ctx context.Context, ref catalog.TableRef, path string, columns []string, match func(string) bool) storage.Batch {
	b := storage.Batch{Ref: ref}
	fail := func(err error) storage.Batch {
		b.Err = fmt.Errorf("%s: %w", path, err)
		return b
	}
	var (
		tbl arrow.Table
		err error
	)
	switch r.format {
	case CSV:
		tbl, err = readCSV(path, ref.Def)
	default:
		tbl, b.Source, err = readParquet(ctx, path)
	}
	if err != nil {
		return fail(err)
	}
	if tbl, err = filterTable(ctx, tbl, match, columns); err != nil {
		return fail(err)
	}
	b.Table = tbl
	return b
}

// readCSV reads a CSV written by the Writer, typed by def's Arrow schema.
// Columns in the header are read in the header's order; any column of the
// schema may be absent.
func readCSV(path string, def catalog.TableDef) (arrow.Table, error) {
	full, err := def.ArrowSchema()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// The header names the columns; each is typed by the table's schema.
	header, err := csvHeader(f)
	if err != nil {
		return nil, err
	}
	fields := make([]arrow.Field, len(header))
	for i, name := range header {
		idx := full.FieldIndices(name)
		if len(idx) != 1 {
			return nil, fmt.Errorf("column %q is not a column of %s", name, def.Name)
		}
		fields[i] = full.Field(idx[0])
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	rdr := csv.NewReader(f, arrow.NewSchema(fields, nil), csv.WithHeader(true),
		csv.WithNullReader(true, ""), csv.WithChunk(65536), csv.WithAllocator(memory.DefaultAllocator))
	defer rdr.Release()
	var recs []arrow.RecordBatch
	for rdr.Next() {
		rec := rdr.RecordBatch()
		rec.Retain()
		recs = append(recs, rec)
	}
	release := func() {
		for _, rec := range recs {
			rec.Release()
		}
	}
	if err := rdr.Err(); err != nil {
		release()
		return nil, err
	}
	tbl := array.NewTableFromRecords(arrow.NewSchema(fields, nil), recs)
	release()
	return tbl, nil
}

// csvHeader is the first record of f.
func csvHeader(f *os.File) ([]string, error) {
	h, err := encsv.NewReader(f).Read()
	if err == io.EOF {
		return nil, fmt.Errorf("no header row")
	}
	return h, err
}

// readParquet reads a whole Parquet file as written by the Writer, and the
// one raw file it records as its source (nil if none, or several).
func readParquet(ctx context.Context, path string) (arrow.Table, *catalog.SourceFile, error) {
	pf, err := file.OpenParquetFile(path, false)
	if err != nil {
		return nil, nil, err
	}
	defer pf.Close()
	kv := pf.MetaData().KeyValueMetadata()
	srcs, err := catalog.SourcesOf(func(key string) (string, bool) {
		v := kv.FindValue(key)
		if v == nil {
			return "", false
		}
		return *v, true
	})
	if err != nil {
		return nil, nil, err
	}
	var src *catalog.SourceFile
	if len(srcs) == 1 {
		src = &srcs[0]
	}
	fr, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
	if err != nil {
		return nil, nil, err
	}
	tbl, err := fr.ReadTable(ctx)
	return tbl, src, err
}

// filterTable keeps the rows of tbl whose stream_edid match accepts (all, if
// it has no such column) and the columns, in that order (all when empty). It
// releases tbl and returns the new table.
func filterTable(ctx context.Context, tbl arrow.Table, match func(string) bool, columns []string) (arrow.Table, error) {
	defer tbl.Release()
	if idx := tbl.Schema().FieldIndices(catalog.StreamEDIDName); len(idx) == 1 && tbl.NumRows() > 0 {
		var mask []bool
		keep := 0
		for _, chunk := range tbl.Column(idx[0]).Data().Chunks() {
			col, ok := chunk.(*array.String)
			if !ok {
				return nil, fmt.Errorf("%s is not a string column", catalog.StreamEDIDName)
			}
			for i := range col.Len() {
				m := col.IsValid(i) && match(col.Value(i))
				mask = append(mask, m)
				if m {
					keep++
				}
			}
		}
		if keep < len(mask) {
			bld := array.NewBooleanBuilder(memory.DefaultAllocator)
			bld.AppendValues(mask, nil)
			maskArr := bld.NewArray()
			bld.Release()
			defer maskArr.Release()
			out, err := compute.FilterTable(ctx, tbl, compute.NewDatum(maskArr), compute.DefaultFilterOptions())
			if err != nil {
				return nil, err
			}
			// FilterTable's result is a new table; tbl is released by defer.
			return projectTable(out, columns)
		}
	}
	tbl.Retain()
	return projectTable(tbl, columns)
}

// projectTable keeps columns of tbl, in that order, releasing tbl.
func projectTable(tbl arrow.Table, columns []string) (arrow.Table, error) {
	if len(columns) == 0 {
		return tbl, nil
	}
	defer tbl.Release()
	fields := make([]arrow.Field, len(columns))
	cols := make([]arrow.Column, len(columns))
	for i, name := range columns {
		idx := tbl.Schema().FieldIndices(name)
		if len(idx) != 1 {
			return nil, fmt.Errorf("no column %q", name)
		}
		fields[i] = tbl.Schema().Field(idx[0])
		cols[i] = *tbl.Column(idx[0])
	}
	return array.NewTable(arrow.NewSchema(fields, nil), cols, tbl.NumRows()), nil
}
