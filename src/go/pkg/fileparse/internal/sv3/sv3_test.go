package sv3

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/earthscope/gnsstools/core/coordinates"
)

func TestUnixMicros(t *testing.T) {
	for in, want := range map[string]int64{
		"1780460559.981377":  1780460559981377,
		"1780460560.0":       1780460560000000,
		"1780460560":         1780460560000000,
		"1780460560.0187323": 1780460560018732, // truncated below the microsecond
		"1.78046056e9":       1780460560000000,
	} {
		got, err := UnixMicros(json.Number(in))
		if err != nil || got != want {
			t.Errorf("UnixMicros(%s) = %d, %v; want %d", in, got, err, want)
		}
	}
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestENUSigmasToECEF(t *testing.T) {
	deg := math.Pi / 180
	for _, c := range []struct {
		name     string
		lat, lon float64
		want     [3]float64 // for sigE=1, sigN=2, sigU=3
	}{
		// At (0, 0) east is +Y, north is +Z and up is +X.
		{"equator", 0, 0, [3]float64{3, 1, 2}},
		// At the north pole (lon 0) east is +Y, north is -X and up is +Z.
		{"pole", 90, 0, [3]float64{2, 1, 3}},
	} {
		got := enuSigmasToECEF(coordinates.GeodeticCoord{Latitude: c.lat * deg, Longitude: c.lon * deg}, 1, 2, 3)
		for i := range 3 {
			if !near(got[i], c.want[i], 1e-12) {
				t.Errorf("%s: sigmas = %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
	// A rotation keeps the total variance.
	got := enuSigmasToECEF(coordinates.GeodeticCoord{Latitude: 42.13 * deg, Longitude: -125.11 * deg}, 8.2, 10.8, 23)
	total := got[0]*got[0] + got[1]*got[1] + got[2]*got[2]
	if !near(total, 8.2*8.2+10.8*10.8+23*23, 1e-9) {
		t.Errorf("total variance %v, want %v", total, 8.2*8.2+10.8*10.8+23*23)
	}
}

// A QCPIN file: the interrogation, two replies under their uids, and a
// non-event key; the replies come back after the interrogation in key order.
const qcpin = `{"00788F":{"event":"range","event_id":7,"observations":{"GNSS":"ERR3"},"range":{"cn":"IR5210","diag":{"dbv":[-30],"snr":[15],"xc":[50]},"range":0.41,"tat":520},"time":{"common":1758209784.41}},
"interrogation":{"event":"interrogation","event_id":7,"observations":{"NOV_RANGE":{"raw":"#RANGEA,USB2,0,67.0,FINESTEERING,2379,221683.000,02000000,5103,17136;0*00000000","time":{"common":1758209784.0}}},"time":{"common":1758209784.0000002}},
"00788A":{"event":"range","event_id":7,"observations":{},"range":{"cn":"IR5209","diag":{"dbv":[-32],"snr":[16],"xc":[52]},"range":0.33,"tat":520},"time":{"common":1758209784.33}},
"version":"1.2"}`

func TestReadQCPIN(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x_USGS.pin")
	if err := os.WriteFile(path, []byte(qcpin), 0o644); err != nil {
		t.Fatal(err)
	}
	events, bad, err := ReadQCPIN(path)
	if err != nil || bad != 0 {
		t.Fatalf("ReadQCPIN: bad=%d err=%v", bad, err)
	}
	if len(events) != 3 || events[0].Event != "interrogation" ||
		events[1].Range.CN != "IR5209" || events[2].Range.CN != "IR5210" {
		t.Fatalf("events = %+v", events)
	}
	if log, err := events[0].RangeLog(); err != nil || log == "" {
		t.Errorf("RangeLog = %q, %v", log, err)
	}
	if log, err := events[1].RangeLog(); err != nil || log != "" {
		t.Errorf("no NOV_RANGE block: RangeLog = %q, %v", log, err)
	}
	if tt := TravelTimes(events); len(tt) != 2 || tt[0].TravelTime != 0.33 {
		t.Errorf("TravelTimes = %+v", tt)
	}
}
