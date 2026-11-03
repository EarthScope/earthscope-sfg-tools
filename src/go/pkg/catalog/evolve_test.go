package catalog

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// inspvaRows is n full rows of def (INSPVA or an older version of it) for
// stream edid, one per second from start; status fills ins_status and
// time_status where def has them.
func inspvaRows(t *testing.T, def datamodels.TableDef, edid string, start time.Time, n int, status string) arrow.Table {
	t.Helper()
	sc, err := def.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	for i := range n {
		for j, f := range sc.Fields() {
			switch fb := b.Field(j).(type) {
			case *array.TimestampBuilder:
				fb.Append(arrow.Timestamp(start.Add(time.Duration(i) * time.Second).UnixMicro()))
			case *array.Float64Builder:
				fb.Append(1)
			case *array.StringBuilder:
				switch f.Name {
				case "stream_edid":
					fb.Append(edid)
				case "ins_status":
					fb.Append(status)
				case "time_status":
					fb.Append("FINESTEERING")
				default:
					fb.Append("x")
				}
			default:
				t.Fatalf("unhandled column %s", f.Name)
			}
		}
	}
	rec := b.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

// A table created before ins_status/time_status existed gains them on
// reopen; old rows read them as null, new rows carry them.
func TestEvolveAddsOptionalColumns(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	newCols := []string{"ins_status", "time_status"}
	old := datamodels.INSPVA
	old.Columns = slices.DeleteFunc(slices.Clone(old.Columns), func(c datamodels.Column) bool {
		return slices.Contains(newCols, c.Name)
	})
	const edidOld, edidNew = "net:STA:C:NOV000:old", "net:STA:C:NOV000:new"

	cat, err := Open(ctx, Config{Type: TypeSQL, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	rows := inspvaRows(t, old, edidOld, t0, 3, "")
	if _, err := cat.Append(ctx, old, rows); err != nil {
		t.Fatal(err)
	}
	rows.Release()
	cat.Close()

	cat, err = Open(ctx, Config{Type: TypeSQL, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tbl, err := cat.Table(ctx, datamodels.INSPVA)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range newCols {
		if f, ok := tbl.Schema().FindFieldByName(name); !ok || f.Required {
			t.Fatalf("%s: not added as an optional column", name)
		}
	}
	rows = inspvaRows(t, datamodels.INSPVA, edidNew, t0, 2, "INS_SOLUTION_GOOD")
	if _, err := cat.Append(ctx, datamodels.INSPVA, rows); err != nil {
		t.Fatalf("append after evolving: %v", err)
	}
	rows.Release()

	tbl, _ = cat.Table(ctx, datamodels.INSPVA)
	at, err := tbl.Scan().ToArrowTable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer at.Release()
	if at.NumRows() != 5 {
		t.Fatalf("%d rows, want 5", at.NumRows())
	}
	rdr := array.NewTableReader(at, -1)
	defer rdr.Release()
	// Every row is checked by name: the evolved table has the new columns
	// after stream_edid, so a positional write would shift values across.
	edidIdx := at.Schema().FieldIndices("stream_edid")[0]
	statusIdx := at.Schema().FieldIndices("ins_status")[0]
	timeIdx := at.Schema().FieldIndices("time_status")[0]
	seen := map[string]int{}
	for rdr.Next() {
		rec := rdr.RecordBatch()
		edid := rec.Column(edidIdx).(*array.String)
		status := rec.Column(statusIdx).(*array.String)
		ts := rec.Column(timeIdx).(*array.String)
		for i := range int(rec.NumRows()) {
			seen[edid.Value(i)]++
			switch edid.Value(i) {
			case edidOld:
				if !status.IsNull(i) || !ts.IsNull(i) {
					t.Errorf("old row %d: statuses %v %v, want null", i, status.Value(i), ts.Value(i))
				}
			case edidNew:
				if status.Value(i) != "INS_SOLUTION_GOOD" || ts.Value(i) != "FINESTEERING" {
					t.Errorf("new row %d: ins_status %q time_status %q", i, status.Value(i), ts.Value(i))
				}
			default:
				t.Errorf("row %d: stream_edid %q", i, edid.Value(i))
			}
		}
	}
	if seen[edidOld] != 3 || seen[edidNew] != 2 {
		t.Errorf("rows by stream = %v, want 3 old and 2 new", seen)
	}
}

func TestEvolveRefusesRequiredColumn(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat, err := Open(ctx, Config{Type: TypeSQL, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Table(ctx, datamodels.SoundSpeed); err != nil {
		t.Fatal(err)
	}
	cat.Close()

	grown := datamodels.SoundSpeed
	grown.Columns = append(slices.Clone(grown.Columns), datamodels.Column{Name: "cast_id", Type: datamodels.SoundSpeed.Columns[0].Type, Required: true})
	cat, err = Open(ctx, Config{Type: TypeSQL, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if _, err := cat.Table(ctx, grown); err == nil {
		t.Fatal("want an error adding a required column to an existing table")
	}
}
