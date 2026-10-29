package gnss00

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/coordinates"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// nmea adds the checksum to "$<body>".
func nmea(body string) string {
	var x byte
	for i := 1; i < len(body); i++ {
		x ^= body[i]
	}
	return fmt.Sprintf("%s*%02X\r\n", body, x)
}

// packet frames data as a GPSA packet stamped at t (UNIX UTC).
func packet(t time.Time, data string) []byte {
	body := make([]byte, 18, 18+len(data)+1)
	binary.BigEndian.PutUint16(body[0:2], 991)
	binary.LittleEndian.PutUint64(body[10:18], uint64(t.UnixMicro()))
	body = append(body, data...)
	var crc byte
	for _, b := range body {
		crc ^= b
	}
	body = append(body, crc)
	out := []byte{0x10, 0x02}
	for _, b := range body {
		if b == 0x10 {
			out = append(out, 0x10)
		}
		out = append(out, b)
	}
	return append(out, 0x10, 0x03)
}

func col[T arrow.Array](tbl arrow.Table, name string) T {
	return tbl.Column(tbl.Schema().FieldIndices(name)[0]).Data().Chunk(0).(T)
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestAntennaPositions(t *testing.T) {
	day := time.Date(2025, 9, 15, 0, 0, 0, 0, time.UTC)
	gga := nmea("$GPGGA,000406.00,4207.8181904,N,12506.6609759,W,1,15,0.7,0.462,M,-30.10,M,,")
	badSum := "$GPGGA,000500.00,4207.8181904,N,12506.6609759,W,1,15,0.7,0.462,M,-30.10,M,,*00\r\n"
	var buf []byte
	for _, p := range []struct {
		at   time.Time
		data string
	}{
		{day.Add(4*time.Minute + 5*time.Second), nmea("$GPGSA,M,3,21,07,09,11,30,04,27,03,16,08,,,1.1,0.7,0.8")},
		{day.Add(4*time.Minute + 5900*time.Millisecond), nmea("$GPVTG,254.647,T,254.647,M,1.325,N,2.453,K,A")},
		{day.Add(4*time.Minute + 6100*time.Millisecond), gga},
		// The same fix again: one row.
		{day.Add(4*time.Minute + 6200*time.Millisecond), gga},
		// No fix, then a corrupted sentence: both skipped.
		{day.Add(4*time.Minute + 30*time.Second), nmea("$GPGGA,000436.00,,,,,0,00,99.9,,,,,,")},
		{day.Add(5 * time.Minute), badSum},
		// A fix just before midnight in a packet stamped just after it: the
		// previous day's date. No GSA within the window, so PDOP is null.
		{day.Add(24*time.Hour + 500*time.Millisecond), nmea("$GPGGA,235959.80,4207.8181904,N,12506.6609759,W,6,08,1.2,0.5,M,-30.10,M,,")},
	} {
		buf = append(buf, packet(p.at, p.data)...)
	}
	path := filepath.Join(t.TempDir(), "x_GNSS00.bin")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}

	tbl, err := AntennaPositions(path, LogGGA)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := datamodels.ParsedSchema(datamodels.GNSSAntennaPositions); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want gnss_antenna_positions parsed schema %v", tbl.Schema(), want)
	}
	if tbl.NumRows() != 2 {
		t.Fatalf("%d rows, want 2", tbl.NumRows())
	}

	tm := col[*array.Timestamp](tbl, "time")
	if got, want := int64(tm.Value(0)), day.Add(4*time.Minute+6*time.Second).UnixMicro(); got != want {
		t.Errorf("row 0 time = %v, want %v", time.UnixMicro(got).UTC(), time.UnixMicro(want).UTC())
	}
	if got, want := int64(tm.Value(1)), day.Add(24*time.Hour-200*time.Millisecond).UnixMicro(); got != want {
		t.Errorf("row 1 time = %v, want %v (the day before the packet)", time.UnixMicro(got).UTC(), time.UnixMicro(want).UTC())
	}

	// 42°07.8181904' N, 125°06.6609759' W, hae 0.462 - 30.10.
	lat, lon := 42+7.8181904/60, -(125 + 6.6609759/60)
	want := coordinates.GeodeticToECEF(coordinates.GeodeticCoord{
		Latitude: lat * math.Pi / 180, Longitude: lon * math.Pi / 180, Height: 0.462 - 30.10,
	})
	x, y, z := col[*array.Float64](tbl, "ant_x"), col[*array.Float64](tbl, "ant_y"), col[*array.Float64](tbl, "ant_z")
	if !near(x.Value(0), want[0], 1e-6) || !near(y.Value(0), want[1], 1e-6) || !near(z.Value(0), want[2], 1e-6) {
		t.Errorf("row 0 ECEF = %v %v %v, want %v", x.Value(0), y.Value(0), z.Value(0), want)
	}

	q := col[*array.Int32](tbl, "q")
	n := col[*array.Int32](tbl, "number_of_satellites")
	pdop := col[*array.Float64](tbl, "pdop")
	if q.Value(0) != 1 || n.Value(0) != 15 || pdop.Value(0) != 1.1 {
		t.Errorf("row 0 q/sats/pdop = %v %v %v, want 1 15 1.1", q.Value(0), n.Value(0), pdop.Value(0))
	}
	if q.Value(1) != 6 || !pdop.IsNull(1) {
		t.Errorf("row 1 q = %v, pdop null = %v; want 6, true", q.Value(1), pdop.IsNull(1))
	}
	for _, name := range []string{"ant_sigx", "ant_sigy", "ant_sigz", "wrms"} {
		if c := tbl.Column(tbl.Schema().FieldIndices(name)[0]); c.NullN() != c.Len() {
			t.Errorf("%s: want all null (no GST logged)", name)
		}
	}
}

func TestDegrees(t *testing.T) {
	for _, c := range []struct {
		v, hemi string
		digits  int
		want    float64
		ok      bool
	}{
		{"4207.8181904", "N", 2, 42 + 7.8181904/60, true},
		{"12506.6609759", "W", 3, -(125 + 6.6609759/60), true},
		{"0030.0", "S", 2, -0.5, true},
		{"4260.0", "N", 2, 0, false}, // 60 minutes
		{"4207.8", "X", 2, 0, false},
		{"42", "N", 2, 0, false},
	} {
		got, err := degrees(c.v, c.hemi, "N", "S", c.digits)
		if c.hemi == "W" || c.hemi == "E" {
			got, err = degrees(c.v, c.hemi, "E", "W", c.digits)
		}
		if (err == nil) != c.ok || (c.ok && !near(got, c.want, 1e-12)) {
			t.Errorf("degrees(%q, %q) = %v, %v", c.v, c.hemi, got, err)
		}
	}
}

func TestWrongLogType(t *testing.T) {
	if tbl, err := AntennaPositions("unused", "GSV"); err == nil {
		tbl.Release()
		t.Error("want an error for a log type other than GGA")
	}
}

// TestParseFile runs on a real GNSS00.bin when SFG_GNSS00 names one.
func TestParseFile(t *testing.T) {
	path := os.Getenv("SFG_GNSS00")
	if path == "" {
		t.Skip("set SFG_GNSS00 to a GNSS00.bin file")
	}
	tbl, err := AntennaPositions(path, LogGGA)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	t.Logf("%s: %d fixes", filepath.Base(path), tbl.NumRows())
	if tbl.NumRows() == 0 {
		t.Fatal("no fixes")
	}
}
