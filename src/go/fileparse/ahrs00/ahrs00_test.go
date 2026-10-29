package ahrs00

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/internal/gpsa"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/internal/xsens"
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
	rs, stats, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d rows, %+v", filepath.Base(path), len(rs), stats)
	if len(rs) == 0 {
		t.Fatal("no rows")
	}
	if first := rs[0].Time; first.Year() < 2015 || first.After(time.Now()) {
		t.Errorf("implausible first time %v", first)
	}
	for i, r := range rs {
		if r.H != nil && (*r.H < 0 || *r.H >= 360 || math.Abs(*r.P) > 90 || math.Abs(*r.R) > 180) {
			t.Fatalf("row %d out of range: %+v", i, r)
		}
	}
}

func TestToAttitude(t *testing.T) {
	p := gpsa.Packet{InstrumentTime: 1_500_000, CommonTime: 1_700_000_000_000_000}
	full := toAttitude(p, xsens.MTData2{
		Euler:         xsens.Vec3{X: 2, Y: 1, Z: -10, Valid: true},
		Acceleration:  xsens.Vec3{X: 0.1, Y: 0.2, Z: 9.8, Valid: true},
		MagneticField: xsens.Vec3{X: 1, Y: 0, Valid: true},
	})
	if !full.Time.Equal(time.UnixMicro(1_700_000_000_000_000)) || full.TimeInstrument != 1.5 {
		t.Errorf("time = %v, %v", full.Time, full.TimeInstrument)
	}
	if full.H == nil || *full.H != 350 || *full.R != 2 || *full.Acz != 9.8 || *full.HMag != 0 {
		t.Errorf("full message: %+v", full)
	}
	if full.TimeStartCount != nil || full.TimeStatus != nil {
		t.Error("AHRS00 has no SV3 time block")
	}
	magOnly := toAttitude(p, xsens.MTData2{MagneticField: xsens.Vec3{X: 0, Y: -1, Valid: true}})
	if magOnly.H != nil || magOnly.Acx != nil || magOnly.HMag == nil || *magOnly.HMag != 90 {
		t.Errorf("magnetometer-only message: %+v", magOnly)
	}
}
