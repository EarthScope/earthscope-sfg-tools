package catalog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// derivedRows is n rows of def in stream edid and its scope, def.TimeColumn
// one second apart from start, other required columns zero and optional
// ones null.
func derivedRows(t *testing.T, def datamodels.TableDef, edid string, start time.Time, n int) arrow.Table {
	t.Helper()
	sc, err := def.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	parts := strings.Split(edid, ":")
	str := map[string]string{"stream_edid": edid, "Network": parts[0], "Station": parts[1], "Campaign": parts[2]}
	for i := range n {
		ts := arrow.Timestamp(start.Add(time.Duration(i) * time.Second).UnixMicro())
		for j, f := range sc.Fields() {
			if f.Nullable {
				b.Field(j).AppendNull()
				continue
			}
			switch fb := b.Field(j).(type) {
			case *array.StringBuilder:
				v, ok := str[f.Name]
				if !ok {
					v = "x"
				}
				fb.Append(v)
			case *array.TimestampBuilder:
				fb.Append(ts)
			case *array.Float64Builder:
				fb.Append(0)
			case *array.Float32Builder:
				fb.Append(0)
			case *array.Int32Builder:
				fb.Append(0)
			case *array.BooleanBuilder:
				fb.Append(false)
			default:
				t.Fatalf("%s: unhandled builder %T", f.Name, fb)
			}
		}
	}
	rec := b.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

// Every derived table is created, and a rebuilt product replaces its stream
// without touching the same table at the other level.
func TestDerivedTables(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(ctx, Config{Type: TypeSQL, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, def := range append(append([]datamodels.TableDef{}, datamodels.Level2Tables...), datamodels.Level3Tables...) {
		if _, err := cat.Table(ctx, def); err != nil {
			t.Fatalf("%s.%s: %v", def.Namespace(), def.Name, err)
		}
	}
	scope := datamodels.Scope{Network: "net", Station: "STA", Campaign: "2025_A_1126"}
	sv3 := datamodels.DerivedEDID(scope, datamodels.ProductGNSSA, "DFOP00:DFOP00")
	smoothed := datamodels.DerivedEDID(scope, datamodels.ProductGNSSA, "SMOOTHED.PRIDE.NOV000.RANGEA")
	write := func(def datamodels.TableDef, edid string, n int) {
		t.Helper()
		rows := derivedRows(t, def, edid, t0, n)
		defer rows.Release()
		if _, err := cat.Append(ctx, def, rows); err != nil {
			t.Fatal(err)
		}
	}
	write(datamodels.GarposL2, sv3, 10)
	write(datamodels.GarposL3, smoothed, 10)
	write(datamodels.GarposL2, sv3, 6) // rebuilt over the first 6 s
	if all, got := count(t, cat, datamodels.GarposL2, sv3); all != 10 || got != 10 {
		t.Errorf("L2 garpos: %d rows (%d of stream), want 10", all, got)
	}
	if all, _ := count(t, cat, datamodels.GarposL3, smoothed); all != 10 {
		t.Errorf("L3 garpos: %d rows, want 10", all)
	}
}
