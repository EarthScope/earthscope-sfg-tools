package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

var t0 = time.Date(2026, 6, 2, 19, 54, 31, 0, time.UTC)

// attitudeRows is n attitude_ahrs rows of stream edid, one per second from
// start, in the table's full schema.
func attitudeRows(t *testing.T, edid string, start time.Time, n int) arrow.Table {
	t.Helper()
	sc, err := datamodels.AttitudeAHRS.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	set := func(name string, v any) {
		switch fb := b.Field(sc.FieldIndices(name)[0]).(type) {
		case *array.StringBuilder:
			fb.Append(v.(string))
		case *array.TimestampBuilder:
			fb.Append(v.(arrow.Timestamp))
		case *array.Float64Builder:
			fb.Append(v.(float64))
		}
	}
	for i := range n {
		set("Network", "net")
		set("Station", "STA")
		set("Campaign", "2026_A_1126")
		set("Log_Source", "AHRS00")
		set("stream_edid", edid)
		set("time", arrow.Timestamp(start.Add(time.Duration(i)*time.Second).UnixMicro()))
		set("time_instrument", float64(i))
	}
	for i, f := range sc.Fields() {
		if fb := b.Field(i); fb.Len() < n {
			if !f.Nullable {
				t.Fatalf("required column %s not set", f.Name)
			}
			fb.AppendNulls(n - fb.Len())
		}
	}
	rec := b.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

// count is the number of rows in def's table, and of those, the rows of edid.
func count(t *testing.T, cat *Catalog, def datamodels.TableDef, edid string) (all, stream int64) {
	t.Helper()
	tbl, err := cat.Table(context.Background(), def)
	if err != nil {
		t.Fatal(err)
	}
	at, err := tbl.Scan().ToArrowTable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer at.Release()
	rdr := array.NewTableReader(at, -1)
	defer rdr.Release()
	idx := at.Schema().FieldIndices("stream_edid")[0]
	for rdr.Next() {
		e := rdr.RecordBatch().Column(idx).(*array.String)
		for i := range e.Len() {
			all++
			if e.Value(i) == edid {
				stream++
			}
		}
	}
	return all, stream
}

func TestAppendIsIdempotent(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(ctx, Config{Type: TypeSQL, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	def := datamodels.AttitudeAHRS
	const a, b = "net:STA:2026_A_1126:AHRS00:AHRS00", "net:STA:2026_A_1126:DFOP00:DFOP00"
	write := func(rows arrow.Table, opts ...AppendOption) {
		t.Helper()
		defer rows.Release()
		if _, err := cat.Append(ctx, def, rows, opts...); err != nil {
			t.Fatal(err)
		}
	}
	check := func(step string, wantAll, wantA int64) {
		t.Helper()
		if all, gotA := count(t, cat, def, a); all != wantAll || gotA != wantA {
			t.Errorf("%s: %d rows (%d of stream a), want %d (%d)", step, all, gotA, wantAll, wantA)
		}
	}

	src := filepath.Join(t.TempDir(), "x_AHRS00.bin")
	if err := os.WriteFile(src, []byte("raw"), 0o644); err != nil {
		t.Fatal(err)
	}
	write(attitudeRows(t, a, t0, 10), WithSource(src)) // file 1 of stream a
	check("first ingest", 10, 10)
	tbl, _ := cat.Table(ctx, def)
	if p := tbl.CurrentSnapshot().Summary.Properties; p[PropSourceFile] != src || len(p[PropSourceSHA256]) != 64 {
		t.Errorf("snapshot properties = %v, want source file and sha256", p)
	}

	write(attitudeRows(t, a, t0, 10)) // the same file again
	check("re-ingest", 10, 10)

	write(attitudeRows(t, a, t0.Add(10*time.Second), 5)) // file 2 of stream a, after file 1
	check("next file", 15, 15)

	write(attitudeRows(t, b, t0, 7)) // another stream over the same time
	check("other stream", 22, 15)

	// File 1 re-parsed to 8 rows: only its new, narrower window is
	// replaced, so file 1's old rows at t0+8s and t0+9s remain (documented
	// on Append).
	write(attitudeRows(t, a, t0, 8))
	check("narrower re-parse", 22, 15)
}
