package ahrs00

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
)

func TestMagneticHeading(t *testing.T) {
	for _, c := range []struct{ x, y, want float64 }{
		{1, 0, 0}, {0, -1, 90}, {-1, 0, 180}, {0, 1, 270},
	} {
		if got := magneticHeading(c.x, c.y); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("magneticHeading(%v, %v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	if got := wrap360(-170); got != 190 {
		t.Errorf("wrap360(-170) = %v", got)
	}
}

// TestParseFile runs on a real AHRS00.bin when SFG_AHRS00 names one.
func TestParseFile(t *testing.T) {
	path := os.Getenv("SFG_AHRS00")
	if path == "" {
		t.Skip("set SFG_AHRS00 to an AHRS00.bin file")
	}
	rows, stats, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d rows, %+v", filepath.Base(path), len(rows), stats)
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	first := time.UnixMicro(rows[0].Time).UTC()
	if first.Year() < 2015 || first.After(time.Now()) {
		t.Errorf("implausible first time %v", first)
	}
	for i, r := range rows {
		if !math.IsNaN(r.H) && (r.H < 0 || r.H >= 360 || math.Abs(r.P) > 90 || math.Abs(r.R) > 180) {
			t.Fatalf("row %d out of range: %+v", i, r)
		}
	}
}

func TestTable(t *testing.T) {
	nan := math.NaN()
	rows := []Row{
		{Time: 1_700_000_000_000_000, TimeInstrument: 1.5, Acx: 0.1, Acy: 0.2, Acz: 9.8, H: 10, HMag: 12, P: 1, R: 2},
		{Time: 1_700_000_000_100_000, TimeInstrument: 1.6, Acx: nan, Acy: nan, Acz: nan, H: nan, HMag: 13, P: nan, R: nan},
	}
	scope := schemas.Scope{Network: "net", Station: "STA", Campaign: "2026_A_0001"}
	tbl, err := attitudeTable(rows, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()

	want, _ := schemas.AttitudeAHRS.ArrowSchema()
	if !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want %v", tbl.Schema(), want)
	}
	if tbl.NumRows() != 2 {
		t.Fatalf("%d rows, want 2", tbl.NumRows())
	}
	str := func(name string, row int) string {
		c := tbl.Column(tbl.Schema().FieldIndices(name)[0]).Data().Chunk(0).(*array.String)
		return c.Value(row)
	}
	for name, v := range map[string]string{
		"Network": "net", "Station": "STA", "Campaign": "2026_A_0001", "Log_Source": LogSource,
	} {
		if got := str(name, 1); got != v {
			t.Errorf("%s = %q, want %q", name, got, v)
		}
	}
	nulls := func(name string) int {
		return tbl.Column(tbl.Schema().FieldIndices(name)[0]).NullN()
	}
	for name, n := range map[string]int{"time_start_count": 2, "time_status": 2, "h": 1, "h_mag": 0, "acx": 1} {
		if got := nulls(name); got != n {
			t.Errorf("%s has %d nulls, want %d", name, got, n)
		}
	}
}

func TestParseFileUnsupported(t *testing.T) {
	if _, err := ParseFile("unused", schemas.TWTT, schemas.Scope{}); err == nil {
		t.Fatal("want an error for a table AHRS00 cannot produce")
	}
}
