package catalog

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

func TestExport(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(ctx, Config{Type: TypeSQL, Warehouse: t.TempDir()})
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
	// attitudeRows stamps every row with Log_Source AHRS00 and scope
	// net/STA/2026_A_1126; the stream EDIDs set the file types.
	const ahrs, dfop = "net:STA:2026_A_1126:AHRS00:AHRS00", "net:STA:2026_A_1126:DFOP00:DFOP00"
	write(datamodels.AttitudeAHRS, attitudeRows(t, ahrs, t0, 10))
	write(datamodels.AttitudeAHRS, attitudeRows(t, dfop, t0, 4))
	scope := datamodels.Scope{Network: "net", Station: "STA", Campaign: "2026_A_1126"}
	gnssa := datamodels.DerivedEDID(scope, datamodels.ProductGNSSA, "DFOP00:DFOP00")
	write(datamodels.GarposL2, derivedRows(t, datamodels.GarposL2, gnssa, t0, 3))

	export := func(def datamodels.TableDef, sel Selection, format string) (int64, []byte) {
		t.Helper()
		var buf bytes.Buffer
		n, err := cat.Export(ctx, &buf, def, sel, format)
		if err != nil {
			t.Fatal(err)
		}
		return n, buf.Bytes()
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
		{"log source column", datamodels.AttitudeAHRS, Selection{Scope: scope, LogSource: "AHRS00"}, 14},
		{"window", datamodels.AttitudeAHRS, Selection{Scope: scope, FileType: "AHRS00",
			Start: t0.Add(2 * time.Second), End: t0.Add(5 * time.Second)}, 3},
		{"derived stream", datamodels.GarposL2, Selection{Scope: scope, FileType: datamodels.ProductGNSSA, LogSource: "DFOP00:DFOP00"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if n, _ := export(tc.def, tc.sel, FormatArrow); n != tc.want {
				t.Errorf("%d rows, want %d", n, tc.want)
			}
		})
	}

	sel := Selection{Scope: scope, FileType: "AHRS00", Columns: []string{"time_instrument", "time", "h"}}
	_, b := export(datamodels.AttitudeAHRS, sel, FormatCSV)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 11 || lines[0] != "time_instrument,time,h" {
		t.Fatalf("csv: %d lines, header %q", len(lines), lines[0])
	}
	if want := "0,2026-06-02T19:54:31Z,"; lines[1] != want {
		t.Errorf("csv row %q, want %q (RFC 3339 time, empty null)", lines[1], want)
	}

	_, b = export(datamodels.AttitudeAHRS, sel, FormatArrow)
	r, err := ipc.NewFileReader(bytes.NewReader(b), ipc.WithAllocator(memory.DefaultAllocator))
	if err != nil {
		t.Fatal(err)
	}
	if r.Schema().NumFields() != 3 {
		t.Errorf("arrow: %d fields, want 3", r.Schema().NumFields())
	}
	r.Close()

	_, b = export(datamodels.AttitudeAHRS, sel, FormatParquet)
	pf, err := file.NewParquetReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if pf.NumRows() != 10 {
		t.Errorf("parquet: %d rows, want 10", pf.NumRows())
	}

	// An empty selection is still a valid file.
	if _, b = export(datamodels.AttitudeAHRS, Selection{Scope: datamodels.Scope{Network: "x"}}, FormatParquet); len(b) == 0 {
		t.Error("empty selection wrote nothing")
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
		{"log source alone, no Log_Source column", datamodels.GNSSObs,
			Selection{Scope: datamodels.Scope{Network: "n", Station: "s", Campaign: "c"}, LogSource: "RANGEA"}},
	} {
		if _, err := tc.sel.Filter(tc.def); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}
