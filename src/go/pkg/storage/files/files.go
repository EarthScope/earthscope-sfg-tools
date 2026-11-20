// Package files is the CSV, Parquet and Arrow IPC file Writer: the Batches of
// a table become one file per source under <dir>/<database>/<table>/.
package files

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/csv"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
)

// Format is a file format.
type Format string

// The formats. Parquet is zstd-compressed with the Arrow schema stored; CSV
// has a header row, RFC 3339 UTC times and empty nulls; Arrow is an Arrow
// IPC file.
const (
	CSV     Format = "csv"
	Parquet Format = "parquet"
	Arrow   Format = "arrow"
)

// Ext is the file extension of f, without the dot.
func (f Format) Ext() string { return string(f) }

// Writer appends the Batches of each table into one file per output name,
// <dir>/<database>/<table>/<name>.<ext>: the name is the Batch's source file
// name without its extension, or part-00000 when it has no source. Batches of
// one source (and table) share a file; two sources with the same name get
// -2, -3, ... after it. Each file is written to a temporary file and renamed
// into place by Close, so a failure leaves nothing partial: a Batch whose
// write fails drops its file, and Close finalises the rest. A Batch with no
// rows writes nothing. It accepts every ref.
//
// A Parquet file records the raw files its rows were parsed from (the
// Batches' Source, path and SHA-256) in its key-value metadata, under the
// keys of catalog.SourceProps, so the Reader can carry them on to a sink
// that records provenance. A file's Batches share one source, so it records
// one; Batches without a Source record none. CSV and Arrow files carry no
// such metadata.
//
// Write returns no outputs; the files are reported by Outputs, after Close.
type Writer struct {
	dir    string
	format Format

	mu      sync.Mutex
	open    map[string]*openFile // table dir + source path -> file
	dropped map[string]bool      // keys whose file failed
	used    map[string]bool      // final paths allocated in this run
	outs    []storage.Output
	closed  bool
}

// openFile is a file being written.
type openFile struct {
	ref  catalog.TableRef
	path string // final path
	tmp  *os.File
	bw   *bufio.Writer
	enc  encoder
	rows int64
	srcs []catalog.SourceFile // the distinct Sources of its Batches
}

// NewWriter makes a Writer of format into dir, which is created when the
// first file is written.
func NewWriter(dir string, format Format) (*Writer, error) {
	switch format {
	case CSV, Parquet, Arrow:
	default:
		return nil, fmt.Errorf("unknown table format %q (want %s, %s or %s)", format, Parquet, CSV, Arrow)
	}
	if dir == "" {
		return nil, fmt.Errorf("files writer needs a directory")
	}
	return &Writer{dir: dir, format: format, open: map[string]*openFile{},
		dropped: map[string]bool{}, used: map[string]bool{}}, nil
}

// Accepts is true for every table.
func (w *Writer) Accepts(catalog.TableRef) bool { return true }

// Write appends b's rows to its file, opening it on the first Batch.
func (w *Writer) Write(ctx context.Context, b storage.Batch) ([]storage.Output, error) {
	if b.Table == nil {
		return nil, fmt.Errorf("%s: batch has no table", b.Ref)
	}
	if b.Table.NumRows() == 0 {
		return nil, nil // nothing to write: not an empty file
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, fmt.Errorf("%s: write after close", b.Ref)
	}
	dir := filepath.Join(w.dir, b.Ref.Database, b.Ref.Def.Name)
	srcPath := ""
	if b.Source != nil {
		srcPath = b.Source.Path
	}
	key := dir + "\x00" + srcPath
	if w.dropped[key] {
		return nil, fmt.Errorf("%s: an earlier batch of this file failed", b.Ref)
	}
	f, ok := w.open[key]
	if !ok {
		var err error
		if f, err = w.create(dir, srcPath, b); err != nil {
			w.dropped[key] = true
			return nil, err
		}
		w.open[key] = f
	}
	if err := writeTable(f.enc, b.Table); err != nil {
		w.drop(key, f)
		return nil, err
	}
	f.rows += b.Table.NumRows()
	if b.Source != nil && !slices.Contains(f.srcs, *b.Source) {
		f.srcs = append(f.srcs, *b.Source)
	}
	return nil, nil
}

// create opens the temporary file for b's output.
func (w *Writer) create(dir, srcPath string, b storage.Batch) (*openFile, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	stem := "part-00000"
	if srcPath != "" {
		base := filepath.Base(srcPath)
		stem = strings.TrimSuffix(base, filepath.Ext(base))
	}
	path := filepath.Join(dir, stem+"."+w.format.Ext())
	for i := 2; w.used[path]; i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.%s", stem, i, w.format.Ext()))
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, err
	}
	bw := bufio.NewWriterSize(tmp, 1<<20)
	enc, err := newBatchWriter(bw, b.Table.Schema(), w.format)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	w.used[path] = true
	return &openFile{ref: b.Ref, path: path, tmp: tmp, bw: bw, enc: enc}, nil
}

// drop abandons f, leaving nothing behind, and refuses its later Batches.
func (w *Writer) drop(key string, f *openFile) {
	f.enc.Close()
	f.tmp.Close()
	os.Remove(f.tmp.Name())
	delete(w.open, key)
	w.dropped[key] = true
}

// Close finalises every open file: flush, close and rename into place. A file
// that cannot be finalised is removed and its error returned.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	var errs []error
	for _, key := range slices.Sorted(maps.Keys(w.open)) {
		f := w.open[key]
		err := recordSources(f.enc, f.srcs)
		if cerr := f.enc.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = f.bw.Flush()
		}
		if cerr := f.tmp.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Chmod(f.tmp.Name(), 0o644) // CreateTemp makes it private
		}
		if err == nil {
			err = os.Rename(f.tmp.Name(), f.path)
		}
		var info os.FileInfo
		if err == nil {
			info, err = os.Stat(f.path)
		}
		if err != nil {
			os.Remove(f.tmp.Name())
			errs = append(errs, fmt.Errorf("%s: %w", f.path, err))
			continue
		}
		w.outs = append(w.outs, storage.Output{Ref: f.ref, Path: f.path, Rows: f.rows, Bytes: info.Size()})
	}
	w.open = map[string]*openFile{}
	return errors.Join(errs...)
}

// Outputs are the files written, once Close has finalised them.
func (w *Writer) Outputs() []storage.Output {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.outs)
}

// recordSources puts srcs in a Parquet file's key-value metadata; other
// formats have none.
func recordSources(enc encoder, srcs []catalog.SourceFile) error {
	pw, ok := enc.(*pqarrow.FileWriter)
	if !ok {
		return nil
	}
	props := catalog.SourceProps(srcs...)
	for _, k := range slices.Sorted(maps.Keys(props)) {
		if err := pw.AppendKeyValueMetadata(k, props[k]); err != nil {
			return err
		}
	}
	return nil
}

// encoder is the part of the Arrow csv, IPC and Parquet writers used.
type encoder interface {
	Write(arrow.RecordBatch) error
	Close() error
}

func writeTable(out encoder, tbl arrow.Table) error {
	rdr := array.NewTableReader(tbl, -1)
	defer rdr.Release()
	for rdr.Next() {
		if err := out.Write(rdr.RecordBatch()); err != nil {
			return err
		}
	}
	return rdr.Err()
}

func newBatchWriter(w io.Writer, schema *arrow.Schema, format Format) (encoder, error) {
	switch format {
	case Parquet:
		return pqarrow.NewFileWriter(schema, w,
			parquet.NewWriterProperties(parquet.WithCompression(compress.Codecs.Zstd)),
			pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema()))
	case CSV:
		return csvWriter{csv.NewWriter(w, schema, csv.WithHeader(true), csv.WithNullWriter(""),
			csv.WithCustomTypeConverter(rfc3339))}, nil
	case Arrow:
		return ipc.NewFileWriter(w, ipc.WithSchema(schema))
	default:
		return nil, fmt.Errorf("unknown table format %q (want %s, %s or %s)", format, Parquet, CSV, Arrow)
	}
}

// rfc3339 writes timestamp columns as RFC 3339 UTC times, which pandas,
// DuckDB and Excel all read; the Arrow default separates date and time
// with a space.
func rfc3339(typ arrow.DataType, col arrow.Array) ([]string, bool) {
	ts, ok := typ.(*arrow.TimestampType)
	if !ok {
		return nil, false
	}
	toTime, err := ts.GetToTimeFunc()
	if err != nil {
		return nil, false
	}
	vals := col.(*array.Timestamp)
	out := make([]string, vals.Len())
	for i := range out {
		if vals.IsValid(i) {
			out[i] = toTime(vals.Value(i)).UTC().Format(time.RFC3339Nano)
		}
	}
	return out, true
}

// csvWriter flushes on Close: the Arrow csv writer has no Close of its own.
type csvWriter struct{ *csv.Writer }

func (c csvWriter) Close() error { c.Flush(); return c.Error() }
