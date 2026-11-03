package gnss00

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// writeFile frames sentences as GPSA packets one second apart from t0.
func writeFile(t *testing.T, t0 time.Time, sentences ...string) string {
	t.Helper()
	var buf []byte
	for i, s := range sentences {
		buf = append(buf, packet(t0.Add(time.Duration(i)*time.Second), s)...)
	}
	path := filepath.Join(t.TempDir(), "x_GNSS00.bin")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func checkSchema(t *testing.T, tbl arrow.Table, def datamodels.TableDef, rows int64) {
	t.Helper()
	if want, _ := datamodels.ParsedSchema(def); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want %s parsed schema %v", tbl.Schema(), def.Name, want)
	}
	if tbl.NumRows() != rows {
		t.Fatalf("%d rows, want %d", tbl.NumRows(), rows)
	}
}

func TestSatellitesInView(t *testing.T) {
	t0 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	path := writeFile(t, t0,
		nmea("$GPGSV,2,1,05,02,45,120,41,10,30,200,38,23,11,089,,03,04,284,"),
		nmea("$GPGSV,2,2,05,27,,,"),
		nmea("$GPVTG,303.995,T,303.995,M,1.651,N,3.058,K,A"), // other types ignored
		nmea("$GPGSV,1,1,00,,,,"),                            // nothing in view: no rows
	)
	tbl, err := SatellitesInView(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkSchema(t, tbl, datamodels.GNSSSatellitesInView, 5)

	tm := col[*array.Timestamp](tbl, "time")
	if int64(tm.Value(4)) != t0.UnixMicro() {
		t.Errorf("second sentence's satellite has time %v, want its group's first sentence time %v",
			time.UnixMicro(int64(tm.Value(4))).UTC(), t0)
	}
	prn, snr, el := col[*array.Int32](tbl, "prn"), col[*array.Float64](tbl, "snr"), col[*array.Float64](tbl, "elevation")
	if prn.Value(0) != 2 || snr.Value(0) != 41 || el.Value(0) != 45 {
		t.Errorf("row 0 prn/snr/el = %v %v %v, want 2 41 45", prn.Value(0), snr.Value(0), el.Value(0))
	}
	if !snr.IsNull(2) || !el.IsNull(4) {
		t.Error("want null snr for an untracked satellite and null elevation when not reported")
	}
	if n := col[*array.Int32](tbl, "sats_in_view").Value(0); n != 5 {
		t.Errorf("sats_in_view = %d, want 5", n)
	}
	if tk := col[*array.String](tbl, "talker").Value(0); tk != "GP" {
		t.Errorf("talker = %q", tk)
	}
}

func TestDOP(t *testing.T) {
	t0 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	path := writeFile(t, t0,
		nmea("$GPGSA,M,3,02,10,,,,,,,,,,,3.6,1.5,3.3"),
		nmea("$GPGSA,,1,,,,,,,,,,,,,,,"), // no fix: skipped
		nmea("$GPGSA,A,2,07,09,11,,,,,,,,,,2.0,1.0,"),
	)
	tbl, err := DOP(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkSchema(t, tbl, datamodels.GNSSDOP, 2)
	used, n := col[*array.String](tbl, "satellites_used"), col[*array.Int32](tbl, "number_used")
	if used.Value(0) != "2,10" || n.Value(0) != 2 || used.Value(1) != "7,9,11" || n.Value(1) != 3 {
		t.Errorf("satellites used = %q (%d), %q (%d)", used.Value(0), n.Value(0), used.Value(1), n.Value(1))
	}
	pdop, vdop := col[*array.Float64](tbl, "pdop"), col[*array.Float64](tbl, "vdop")
	if pdop.Value(0) != 3.6 || vdop.Value(0) != 3.3 || !vdop.IsNull(1) {
		t.Errorf("pdop/vdop = %v %v, vdop row 1 null %v", pdop.Value(0), vdop.Value(0), vdop.IsNull(1))
	}
	if ft := col[*array.Int32](tbl, "fix_type"); ft.Value(0) != 3 || ft.Value(1) != 2 {
		t.Errorf("fix_type = %d %d", ft.Value(0), ft.Value(1))
	}
	if m := col[*array.String](tbl, "mode"); m.Value(0) != "M" || m.Value(1) != "A" {
		t.Errorf("mode = %q %q", m.Value(0), m.Value(1))
	}
}

func TestCourseSpeed(t *testing.T) {
	t0 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	path := writeFile(t, t0,
		nmea("$GPVTG,303.995,T,303.995,M,1.651,N,3.058,K,A"),
		nmea("$GPVTG,,,,,,,,,N"),             // no fix: skipped
		nmea("$GPVTG,10.5,T,,M,0.2,N,0.4,K"), // NMEA 2.0: no mode
	)
	tbl, err := CourseSpeed(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkSchema(t, tbl, datamodels.GNSSCourseSpeed, 2)
	ct, cm := col[*array.Float64](tbl, "course_true"), col[*array.Float64](tbl, "course_magnetic")
	knots, kmh := col[*array.Float64](tbl, "speed_knots"), col[*array.Float64](tbl, "speed_kmh")
	if ct.Value(0) != 303.995 || knots.Value(0) != 1.651 || kmh.Value(0) != 3.058 {
		t.Errorf("row 0 = %v %v %v", ct.Value(0), knots.Value(0), kmh.Value(0))
	}
	if !cm.IsNull(1) || ct.Value(1) != 10.5 {
		t.Errorf("row 1 course_magnetic null %v, course_true %v", cm.IsNull(1), ct.Value(1))
	}
	if m := col[*array.String](tbl, "mode"); m.Value(0) != "A" || !m.IsNull(1) {
		t.Errorf("mode = %q, row 1 null %v", m.Value(0), m.IsNull(1))
	}
	if tm := col[*array.Timestamp](tbl, "time"); int64(tm.Value(1)) != t0.Add(2*time.Second).UnixMicro() {
		t.Errorf("row 1 time = %v, want its packet time", time.UnixMicro(int64(tm.Value(1))).UTC())
	}
}
