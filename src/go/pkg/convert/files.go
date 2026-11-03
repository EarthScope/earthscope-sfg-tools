package convert

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/csv"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

// Table formats of a files sink.
const (
	FormatParquet = "parquet" // zstd, Arrow schema stored, as sink.Parquet
	FormatCSV     = "csv"     // header row; RFC 3339 UTC times; empty nulls
	FormatArrow   = "arrow"   // Arrow IPC file
)

// filesSink writes each table to <dir>/<table>.<format>. A table that fails
// leaves no file behind.
type filesSink struct{ dir, format string }

func (s *filesSink) Write(ctx context.Context, rows Rows) ([]Output, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir, rows.Def.Name+"."+s.format)
	tmp, err := os.CreateTemp(s.dir, "."+rows.Def.Name+".*.tmp")
	if err != nil {
		return nil, err
	}
	n, err := s.write(tmp, rows)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644) // CreateTemp makes it private
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return []Output{{Kind: "file", Path: path, Table: rows.Def.Namespace() + "." + rows.Def.Name, Rows: n, Bytes: info.Size()}}, nil
}

// Close does nothing: Write closes each file.
func (s *filesSink) Close() error { return nil }

func (s *filesSink) write(f io.Writer, rows Rows) (int64, error) {
	w := bufio.NewWriterSize(f, 1<<20)
	out, err := newBatchWriter(w, rows.Schema, s.format)
	if err != nil {
		return 0, err
	}
	var n int64
	for g, err := range rows.Groups {
		if err != nil {
			out.Close()
			return n, err
		}
		err = writeTable(out, g.Rows)
		n += g.Rows.NumRows()
		g.Rows.Release()
		if err != nil {
			out.Close()
			return n, err
		}
	}
	if err := out.Close(); err != nil {
		return n, err
	}
	return n, w.Flush()
}

// writeTable writes tbl's record batches to out.
func writeTable(out batchWriter, tbl arrow.Table) error {
	rdr := array.NewTableReader(tbl, -1)
	defer rdr.Release()
	for rdr.Next() {
		if err := out.Write(rdr.RecordBatch()); err != nil {
			return err
		}
	}
	return rdr.Err()
}

// batchWriter is the part of the Arrow csv, IPC and Parquet writers a files
// sink uses.
type batchWriter interface {
	Write(arrow.RecordBatch) error
	Close() error
}

func newBatchWriter(w io.Writer, schema *arrow.Schema, format string) (batchWriter, error) {
	switch format {
	case FormatParquet:
		return pqarrow.NewFileWriter(schema, w,
			parquet.NewWriterProperties(parquet.WithCompression(compress.Codecs.Zstd)),
			pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema()))
	case FormatCSV:
		return csvWriter{csv.NewWriter(w, schema, csv.WithHeader(true), csv.WithNullWriter(""),
			csv.WithCustomTypeConverter(rfc3339))}, nil
	case FormatArrow:
		return ipc.NewFileWriter(w, ipc.WithSchema(schema))
	default:
		return nil, fmt.Errorf("unknown table format %q (want %s, %s or %s)", format, FormatParquet, FormatCSV, FormatArrow)
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
