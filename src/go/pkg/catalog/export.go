package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/csv"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// Selection picks the rows of a table to export. Network is required;
// Station and Campaign narrow the scope, and Campaign needs Station.
//
// FileType and LogSource pick streams (see datamodels.StreamEDID) and need
// the full scope. In a derived table they are the product and its input
// (see datamodels.DerivedEDID), e.g. PRIDE and NOV000:RANGEA. LogSource
// without FileType matches the Log_Source column, so it needs a table that
// has one.
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
	}
	sc, err := def.Schema()
	if err != nil {
		return nil, err
	}
	has := func(name string) bool { _, ok := sc.FindFieldByName(name); return ok }
	edid := iceberg.Reference(datamodels.StreamEDIDColumn.Name)

	var preds []iceberg.BooleanExpression
	if has("Network") {
		for _, kv := range [][2]string{{"Network", s.Network}, {"Station", s.Station}, {"Campaign", s.Campaign}} {
			if kv[1] != "" {
				preds = append(preds, iceberg.EqualTo(iceberg.Reference(kv[0]), kv[1]))
			}
		}
	} else {
		// gnss_observations has no scope columns: its stream EDIDs carry it.
		prefix := s.Network + ":"
		if s.Station != "" {
			prefix += s.Station + ":"
		}
		if s.Campaign != "" {
			prefix += s.Campaign + ":"
		}
		preds = append(preds, hasPrefix(edid, prefix))
	}

	streamEDID := datamodels.StreamEDID
	if def.Level > 1 {
		streamEDID = datamodels.DerivedEDID
	}
	switch {
	case sel.FileType != "" && sel.LogSource != "":
		preds = append(preds, iceberg.EqualTo(edid, streamEDID(s, sel.FileType, sel.LogSource)))
	case sel.FileType != "":
		preds = append(preds, hasPrefix(edid, streamEDID(s, sel.FileType, "")))
	case sel.LogSource != "":
		if !has(datamodels.LogSourceColumn) {
			return nil, fmt.Errorf("%s has no %s column: select a log source with its file type", def.Name, datamodels.LogSourceColumn)
		}
		preds = append(preds, iceberg.EqualTo(iceberg.Reference(datamodels.LogSourceColumn), sel.LogSource))
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

// Export formats.
const (
	FormatParquet = "parquet" // zstd, Arrow schema stored, as sink.Parquet
	FormatCSV     = "csv"     // header row; timestamps as RFC 3339
	FormatArrow   = "arrow"   // Arrow IPC file
)

// Export writes the rows sel selects from def's table to w in format, batch
// by batch, and returns the number of rows written. A selection with no rows
// still writes a valid, empty file.
func (c *Catalog) Export(ctx context.Context, w io.Writer, def datamodels.TableDef, sel Selection, format string) (int64, error) {
	filter, err := sel.Filter(def)
	if err != nil {
		return 0, err
	}
	tbl, err := c.Table(ctx, def)
	if err != nil {
		return 0, err
	}
	opts := []table.ScanOption{table.WithRowFilter(filter)}
	if len(sel.Columns) > 0 {
		opts = append(opts, table.WithSelectedFields(sel.Columns...))
	}
	schema, batches, err := tbl.Scan(opts...).ToArrowRecords(ctx)
	if err != nil {
		return 0, fmt.Errorf("scanning %s: %w", def.Name, err)
	}
	// The scan returns columns in table order; write them in sel's.
	order := make([]int, len(sel.Columns))
	for i, name := range sel.Columns {
		idx := schema.FieldIndices(name)
		if len(idx) != 1 {
			return 0, fmt.Errorf("%s has no column %q", def.Name, name)
		}
		order[i] = idx[0]
	}
	if len(order) > 0 {
		fields := make([]arrow.Field, len(order))
		for i, j := range order {
			fields[i] = schema.Field(j)
		}
		schema = arrow.NewSchema(fields, nil)
	}
	out, err := newBatchWriter(w, schema, format)
	if err != nil {
		return 0, err
	}
	var n int64
	for rec, err := range batches {
		if err != nil {
			out.Close()
			return n, fmt.Errorf("scanning %s: %w", def.Name, err)
		}
		if len(order) > 0 {
			cols := make([]arrow.Array, len(order))
			for i, j := range order {
				cols[i] = rec.Column(j)
			}
			reordered := array.NewRecordBatch(schema, cols, rec.NumRows())
			rec.Release()
			rec = reordered
		}
		err = out.Write(rec)
		n += rec.NumRows()
		rec.Release()
		if err != nil {
			out.Close()
			return n, err
		}
	}
	return n, out.Close()
}

// batchWriter is the part of the Arrow csv, IPC and Parquet writers Export
// uses.
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
		return nil, fmt.Errorf("unknown format %q (want %s, %s or %s)", format, FormatParquet, FormatCSV, FormatArrow)
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
