package catalog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

func TestScan(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(ctx, Config{Name: Local, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	write := func(def datamodels.TableDef, rows arrow.Table) {
		t.Helper()
		defer rows.Release()
		if _, err := cat.Append(ctx, def, rows); err != nil {
			t.Fatal(err)
		}
	}
	// The stream EDIDs set the scope and file types.
	const ahrs, dfop = "net.STA.2026_A_1126.AHRS00.AHRS00", "net.STA.2026_A_1126.DFOP00.DFOP00"
	write(datamodels.AttitudeAHRS, attitudeRows(t, ahrs, t0, 10))
	write(datamodels.AttitudeAHRS, attitudeRows(t, dfop, t0, 4))
	scope := datamodels.Scope{Network: "net", Station: "STA", Campaign: "2026_A_1126"}
	gnssa := datamodels.DerivedEDID(scope, datamodels.ProductGNSSA, "DFOP00:DFOP00")
	write(datamodels.GarposL1, derivedRows(t, datamodels.GarposL1, gnssa, t0, 3))

	scan := func(def datamodels.TableDef, sel Selection) (*arrow.Schema, int64) {
		t.Helper()
		schema, batches, err := cat.Scan(ctx, def, sel)
		if err != nil {
			t.Fatal(err)
		}
		var n int64
		for rec, err := range batches {
			if err != nil {
				t.Fatal(err)
			}
			if !rec.Schema().Equal(schema) {
				t.Errorf("batch schema %s, want %s", rec.Schema(), schema)
			}
			n += rec.NumRows()
			rec.Release()
		}
		return schema, n
	}
	for _, tc := range []struct {
		name string
		def  datamodels.TableDef
		sel  Selection
		want int64
	}{
		{"network", datamodels.AttitudeAHRS, Selection{Scope: datamodels.Scope{Network: "net"}}, 14},
		{"other network", datamodels.AttitudeAHRS, Selection{Scope: datamodels.Scope{Network: "x"}}, 0},
		{"file type", datamodels.AttitudeAHRS, Selection{Scope: scope, FileType: "DFOP00"}, 4},
		{"stream", datamodels.AttitudeAHRS, Selection{Scope: scope, FileType: "AHRS00", LogSource: "AHRS00"}, 10},
		{"station", datamodels.AttitudeAHRS, Selection{Scope: datamodels.Scope{Network: "net", Station: "STA"}}, 14},
		{"other campaign", datamodels.AttitudeAHRS, Selection{Scope: datamodels.Scope{Network: "net", Station: "STA", Campaign: "2026_A_0001"}}, 0},
		{"derived scope", datamodels.GarposL1, Selection{Scope: scope}, 3},
		{"window", datamodels.AttitudeAHRS, Selection{Scope: scope, FileType: "AHRS00",
			Start: t0.Add(2 * time.Second), End: t0.Add(5 * time.Second)}, 3},
		{"derived stream", datamodels.GarposL1, Selection{Scope: scope, FileType: datamodels.ProductGNSSA, LogSource: "DFOP00:DFOP00"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, n := scan(tc.def, tc.sel); n != tc.want {
				t.Errorf("%d rows, want %d", n, tc.want)
			}
		})
	}

	sel := Selection{Scope: scope, FileType: "AHRS00", Columns: []string{"time_instrument", "time", "h"}}
	schema, n := scan(datamodels.AttitudeAHRS, sel)
	var names []string
	for _, f := range schema.Fields() {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "time_instrument,time,h" || n != 10 {
		t.Errorf("columns %s, %d rows; want time_instrument,time,h and 10 rows", got, n)
	}

	// Scan reads; it doesn't create a missing table.
	if _, _, err := cat.Scan(ctx, datamodels.INSPVA, Selection{Scope: scope}); err == nil {
		t.Error("scanning a missing table: no error")
	}
}

func TestSelectionErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  datamodels.TableDef
		sel  Selection
	}{
		{"no network", datamodels.INSPVA, Selection{}},
		{"campaign without station", datamodels.INSPVA, Selection{Scope: datamodels.Scope{Network: "n", Campaign: "c"}}},
		{"type without full scope", datamodels.INSPVA, Selection{Scope: datamodels.Scope{Network: "n", Station: "s"}, FileType: "NOV000"}},
		{"log source without file type", datamodels.INSPVA,
			Selection{Scope: datamodels.Scope{Network: "n", Station: "s", Campaign: "c"}, LogSource: "RANGEA"}},
	} {
		if _, err := tc.sel.Filter(tc.def); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}
