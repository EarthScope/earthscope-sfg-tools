package qcpin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// rangeA is a RANGEA with GPS PRN 30 L1 C/A and L2 P(Y) at GPS week 2326,
// 188418 s, and a hex receiver status (the scanner workaround).
const rangeA = `#RANGEA,USB2,0,69.0,FINESTEERING,2326,188418.000,024c0000,5103,15823;2,` +
	`30,0,21007753.481,0.022,-110396605.995380,0.007,1631.778,51.5,7756.971,1810bc04,` +
	`30,0,21007760.712,0.056,-86023381.342590,0.012,1271.514,51.4,7751.734,11303c0b*bec38899`

// A ping and two replies, as a QCPIN file: the ping and the first reply carry
// the same GNSS block and RANGEA log, the second reply logged sensor errors
// and detected nothing.
const sample = `{
"interrogation": {"event": "interrogation", "event_id": 1, "observations": {
  "AHRS": {"h": 313.35, "p": -0.03, "r": -0.53, "time": {"common": 1755005665.0205498, "instrument": 812.97, "start_count": 52, "status": "GPS_SYNC"}},
  "GNSS": {"hae": -28.429, "latitude": 40.807132808, "longitude": -124.166569545, "q": 1, "sdx": 8.795, "sdy": 14.422, "sdz": 27.6, "separation": -30.4, "time": {"common": 1755005665.0, "instrument": 812.95, "start_count": 52, "status": "GPS_SYNC"}},
  "NOV_RANGE": {"raw": "` + rangeA + `", "time": {"common": 1755005665.01}}},
  "sequence": 1, "time": {"common": 1755005665.0, "instrument": 812.95, "start_count": 52, "status": "GPS_SYNC"}, "type": "BR"},
"00761D": {"event": "range", "event_id": 1, "observations": {
  "GNSS": {"hae": -28.429, "latitude": 40.807132808, "longitude": -124.166569545, "q": 1, "sdx": 8.795, "sdy": 14.422, "sdz": 27.6, "separation": -30.4, "time": {"common": 1755005665.0, "instrument": 812.95, "start_count": 52, "status": "GPS_SYNC"}},
  "NOV_RANGE": {"raw": "` + rangeA + `", "time": {"common": 1755005665.01}}},
  "range": {"cn": "IR5205", "diag": {"dbv": [-32.0], "snr": [16.0], "xc": [52.0]}, "range": 0.330061, "tat": 520},
  "sequence": 1, "time": {"common": 1755005665.330061, "instrument": 813.28, "start_count": 52, "status": "GPS_SYNC"}, "uid": "00761D"},
"00761E": {"event": "range", "event_id": 1, "observations": {"AHRS": "ERR1", "GNSS": "ERR3", "NOV_RANGE": "ERR3"},
  "range": {"cn": "IR5206", "diag": {"dbv": [0.0], "snr": [0.0], "xc": [20.0]}, "range": 0.0, "tat": 520},
  "sequence": 1, "time": {"common": 1755005665.0, "instrument": 812.95, "start_count": 52, "status": "GPS_SYNC"}, "uid": "00761E"}
}`

func writeSample(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x_USGS.pin")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func parse(t *testing.T, fn func(path, logType string) (arrow.Table, error), path, logType string, def datamodels.TableDef) arrow.Table {
	t.Helper()
	tbl, err := fn(path, logType)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := datamodels.ParsedSchema(def); !tbl.Schema().Equal(want) {
		tbl.Release()
		t.Fatalf("schema = %v, want %s parsed schema %v", tbl.Schema(), def.Name, want)
	}
	return tbl
}

func col[T arrow.Array](tbl arrow.Table, name string) T {
	return tbl.Column(tbl.Schema().FieldIndices(name)[0]).Data().Chunk(0).(T)
}

func TestSample(t *testing.T) {
	path := writeSample(t)

	tt := parse(t, TWTT, path, LogEvents, datamodels.TWTT)
	defer tt.Release()
	if tt.NumRows() != 1 || col[*array.String](tt, "MT_ID").Value(0) != "IR5205" ||
		col[*array.Float64](tt, "TravelTime").Value(0) != 0.330061 {
		t.Errorf("travel_times: %d rows", tt.NumRows())
	}

	att := parse(t, AttitudeAHRS, path, LogEvents, datamodels.AttitudeAHRS)
	defer att.Release()
	if att.NumRows() != 1 || col[*array.Float64](att, "h").Value(0) != 313.35 {
		t.Errorf("attitude_ahrs: %d rows", att.NumRows())
	}

	pos := parse(t, AntennaPositions, path, LogEvents, datamodels.GNSSAntennaPositions)
	defer pos.Release()
	if pos.NumRows() != 1 || col[*array.Int32](pos, "q").Value(0) != 1 || col[*array.Float64](pos, "ant_sigx").IsNull(0) {
		t.Errorf("gnss_antenna_positions: %d rows", pos.NumRows())
	}

	obs := parse(t, GNSSObs, path, LogRANGEA, datamodels.GNSSObs)
	defer obs.Release()
	// One RANGEA (logged twice) with two signals.
	if obs.NumRows() != 2 || col[*array.String](obs, "signal").Value(0) != "1C" ||
		col[*array.Float64](obs, "code").Value(0) != 21007753.481 {
		t.Errorf("gnss_observations: %d rows", obs.NumRows())
	}
	if got := col[*array.String](obs, datamodels.LogSourceColumn).Value(0); got != LogRANGEA {
		t.Errorf("Log_Source = %q", got)
	}
}

func TestWrongLogType(t *testing.T) {
	path := writeSample(t)
	if tbl, err := TWTT(path, LogRANGEA); err == nil {
		tbl.Release()
		t.Error("TWTT from RANGEA: want an error")
	}
	if tbl, err := GNSSObs(path, LogEvents); err == nil {
		tbl.Release()
		t.Error("GNSSObs from QCPIN events: want an error")
	}
}

// TestParseFile runs on a real QC PIN file when SFG_QCPIN names one.
func TestParseFile(t *testing.T) {
	path := os.Getenv("SFG_QCPIN")
	if path == "" {
		t.Skip("set SFG_QCPIN to a .pin file")
	}
	for _, c := range []struct {
		fn      func(path, logType string) (arrow.Table, error)
		logType string
		def     datamodels.TableDef
	}{
		{TWTT, LogEvents, datamodels.TWTT},
		{AttitudeAHRS, LogEvents, datamodels.AttitudeAHRS},
		{AntennaPositions, LogEvents, datamodels.GNSSAntennaPositions},
		{GNSSObs, LogRANGEA, datamodels.GNSSObs},
	} {
		tbl := parse(t, c.fn, path, c.logType, c.def)
		t.Logf("%s: %d rows", c.def.Name, tbl.NumRows())
		if tbl.NumRows() == 0 {
			t.Errorf("%s: no rows", c.def.Name)
		}
		tbl.Release()
	}
}
