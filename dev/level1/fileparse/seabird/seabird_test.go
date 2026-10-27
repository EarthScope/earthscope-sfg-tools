package seabird

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// A trimmed SBE .cnv header: named columns with sound speed in column 3, not
// the Python parser's 5, both cast time headers (NMEA wins), and a bad flag.
const cnv = `* Sea-Bird SBE 9 Data File:
* FileName = C:\data\cast_001.hex
* NMEA UTC (Time) = Sep 12 2025 10:15:00
* System UTC = Sep 12 2025 10:14:58
# nquan = 4
# name 0 = depSM: Depth [salt water, m]
# name 1 = t090C: Temperature [ITS-90, deg C]
# name 2 = sal00: Salinity, Practical [PSU]
# name 3 = svCM: Sound Velocity [Chen-Millero, m/s]
# start_time = Sep 12 2025 10:14:30 [Instrument's time stamp, header]
# bad_flag = -9.990e-29
*END*
      0.000    12.5011    32.1150    1495.10
      1.000    12.4990    32.1160    1495.12
      2.000 -9.990e-29    32.1170 -9.990e-29
      3.000    12.3800
     -1.000    12.4000    32.1000    1495.00
   1900.000     2.1000    34.6000    1488.74
`

func write(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func col[T arrow.Array](tbl arrow.Table, name string) T {
	return tbl.Column(tbl.Schema().FieldIndices(name)[0]).Data().Chunk(0).(T)
}

func TestSoundSpeed(t *testing.T) {
	tbl, err := SoundSpeed(write(t, "cast_001.cnv", cnv), LogType)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := datamodels.ParsedSchema(datamodels.SoundSpeed); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want sound_speed parsed schema %v", tbl.Schema(), want)
	}
	want := []struct{ depth, speed float64 }{{0, 1495.10}, {1, 1495.12}, {1900, 1488.74}}
	if int(tbl.NumRows()) != len(want) {
		t.Fatalf("%d rows, want %d (bad-flag, short and above-surface rows skipped)", tbl.NumRows(), len(want))
	}
	depth := col[*array.Float64](tbl, "depth")
	speed := col[*array.Float64](tbl, "speed")
	tm := col[*array.Timestamp](tbl, "time")
	at := time.Date(2025, 9, 12, 10, 15, 0, 0, time.UTC).UnixMicro()
	for i, w := range want {
		if depth.Value(i) != w.depth || speed.Value(i) != w.speed || int64(tm.Value(i)) != at {
			t.Errorf("row %d = %v m %v m/s at %v", i, depth.Value(i), speed.Value(i), tm.Value(i))
		}
	}
}

// Without column names the Python parser's columns (0 and 5) apply, and
// without an NMEA time the start time does.
func TestDefaultsAndStartTime(t *testing.T) {
	data := "# start_time = Sep  2 2025 08:00:01 [Instrument's time stamp, first data scan]\n" +
		"*END*\n" +
		"   10.0  a  b  c  d  1500.5\n"
	tbl, err := SoundSpeed(write(t, "svp.cnv", data), LogType)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if tbl.NumRows() != 1 || col[*array.Float64](tbl, "depth").Value(0) != 10 ||
		col[*array.Float64](tbl, "speed").Value(0) != 1500.5 {
		t.Fatalf("rows = %d", tbl.NumRows())
	}
	if got, want := int64(col[*array.Timestamp](tbl, "time").Value(0)), time.Date(2025, 9, 2, 8, 0, 1, 0, time.UTC).UnixMicro(); got != want {
		t.Errorf("time = %v, want %v", time.UnixMicro(got).UTC(), time.UnixMicro(want).UTC())
	}
}

func TestErrors(t *testing.T) {
	for name, data := range map[string]string{
		"no cast time": "# name 0 = depSM: Depth\n*END*\n1 1500\n",
		"no *END*":     "* NMEA UTC (Time) = Sep 12 2025 10:15:00\n1 2 3 4 5 1500\n",
		"bad time":     "* NMEA UTC (Time) = 2025-09-12T10:15:00Z\n*END*\n",
	} {
		if tbl, err := SoundSpeed(write(t, "x.cnv", data), LogType); err == nil {
			tbl.Release()
			t.Errorf("%s: want an error", name)
		}
	}
	if tbl, err := SoundSpeed(write(t, "x.cnv", cnv), "CTD"); err == nil {
		tbl.Release()
		t.Error("wrong log type: want an error")
	}
}
