package novatel000

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
)

// rangea is a RANGEA with GPS PRN 30 L1 C/A and L2 P(Y), its header carrying
// the given time status, week and receiver status. The checksum is not
// checked by the scanner.
func rangea(timeStatus, week, recStatus string) string {
	return "#RANGEA,USB2,0,69.0," + timeStatus + "," + week + ",188418.000," + recStatus + ",5103,15823;2," +
		"30,0,21007753.481,0.022,-110396605.995380,0.007,1631.778,51.5,7756.971,1810bc04," +
		"30,0,21007760.712,0.056,-86023381.342590,0.012,1271.514,51.4,7751.734,11303c0b" +
		"*bec38899\r\n"
}

// gpsaPacket frames data as one GPSA packet: DLE/STX, an 18-byte header,
// data, an XOR checksum, DLE/ETX, with DLE bytes stuffed.
func gpsaPacket(data string) []byte {
	body := make([]byte, 18, 18+len(data)+1)
	binary.BigEndian.PutUint16(body[0:2], 1)
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

func writeNOV000(t *testing.T, packets ...string) string {
	t.Helper()
	var buf []byte
	for _, p := range packets {
		buf = append(buf, gpsaPacket(p)...)
	}
	path := filepath.Join(t.TempDir(), "test_NOV000.bin")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func column(t *testing.T, tbl arrow.Table, name string) arrow.Array {
	t.Helper()
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) != 1 {
		t.Fatalf("no column %q", name)
	}
	return tbl.Column(idx[0]).Data().Chunk(0)
}

func TestGNSSObs(t *testing.T) {
	good := rangea("FINESTEERING", "2326", "024c0000") // hex receiver status: the scanner workaround
	path := writeNOV000(t,
		// Skipped: no time fix.
		rangea("UNKNOWN", "0", "02000000"),
		// Dropped: a garbled week puts the epoch ~190 years in the future.
		rangea("FINESTEERING", "9999", "02000000"),
		// A log truncated before its checksum and CRLF; only it is lost, and
		// the good log after it, split across two packets, is reassembled.
		"#RANGEA,USB2,0,69.0,FINESTEERING,2326,188417.000,02000000,5103,15823;2,30,0,2100"+good[:40],
		good[40:],
	)

	tbl, err := GNSSObs(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if n := tbl.NumRows(); n != 2 {
		t.Fatalf("%d rows, want 2 (one RANGEA, two signals)", n)
	}

	want := gnsstime.GPSWeekSecToTime(2326, 188418).UnixMicro()
	gpst := column(t, tbl, "time_gpst").(*array.Timestamp)
	cons := column(t, tbl, "constellation").(*array.String)
	sat := column(t, tbl, "satellite_id").(*array.Int32)
	sig := column(t, tbl, "signal").(*array.String)
	code := column(t, tbl, "code").(*array.Float64)
	for i := range 2 {
		if int64(gpst.Value(i)) != want {
			t.Errorf("row %d time_gpst = %v, want %v", i,
				time.UnixMicro(int64(gpst.Value(i))).UTC(), time.UnixMicro(want).UTC())
		}
		if cons.Value(i) != "G" || sat.Value(i) != 30 {
			t.Errorf("row %d = %s%02d, want G30", i, cons.Value(i), sat.Value(i))
		}
	}
	if sig.Value(0) != "1C" || code.Value(0) != 21007753.481 {
		t.Errorf("row 0 signal/code = %s %v, want 1C 21007753.481", sig.Value(0), code.Value(0))
	}
	if sig.Value(1) == "1C" || code.Value(1) != 21007760.712 {
		t.Errorf("row 1 signal/code = %s %v, want an L2 signal and 21007760.712", sig.Value(1), code.Value(1))
	}
}

// INSPVAA rows keep the logged INS status and header time status, so the
// posfilter export can apply GNATSS's FINESTEERING/INS_SOLUTION_GOOD filter.
func TestINSPVAStatus(t *testing.T) {
	inspva := func(timeStatus, sec, insStatus string) string {
		return "#INSPVAA,USB2,0,69.0," + timeStatus + ",2326," + sec + ",02000000,5103,15823;2326," + sec +
			",42.13003285,-125.11183814,-31.878,-0.2412,0.6377,0.2949,2.6278,0.2994,70.4168," + insStatus + "*12345678\r\n"
	}
	path := writeNOV000(t,
		inspva("FINESTEERING", "188418.000", "INS_SOLUTION_GOOD"),
		inspva("COARSESTEERING", "188419.000", "INS_ALIGNING"),
	)
	tbl, err := INSPVA(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if tbl.NumRows() != 2 {
		t.Fatalf("%d rows, want 2", tbl.NumRows())
	}
	ins := column(t, tbl, "ins_status").(*array.String)
	ts := column(t, tbl, "time_status").(*array.String)
	if ins.Value(0) != "INS_SOLUTION_GOOD" || ts.Value(0) != "FINESTEERING" ||
		ins.Value(1) != "INS_ALIGNING" || ts.Value(1) != "COARSESTEERING" {
		t.Errorf("statuses = %v %v", ins, ts)
	}
}

// A receiver adjusting its clock can log a known time status with week 0
// (NDP1 2023). Such a row lands in 1980 and widens the file's replace window
// back over every earlier file of its stream, so it is skipped like UNKNOWN.
func TestINSWeekZeroSkipped(t *testing.T) {
	path := writeNOV000(t,
		"#INSPVAA,USB2,0,49.0,COARSEADJUSTING,0,12.150,02440000,54e2,15823;0,12.150000000,0.00000000000,0.00000000000,0.0000,0.0000,0.0000,0.0000,0.000000000,0.000000000,0.000000000,WAITING_INITIALPOS*4633cf22\r\n",
		"#INSSTDEVA,USB2,0,49.0,COARSEADJUSTING,0,12.150,02440000,2396,15823;0.0000,0.0000,0.0000,0.0000,0.0000,0.0000,0.0000,0.0000,0.0000,00000000,1,0,7fd1b3,0*3af1509c\r\n",
		"#INSPVAA,USB2,0,69.0,FINESTEERING,2326,188418.000,02000000,5103,15823;2326,188418.000,42.13003285,-125.11183814,-31.878,-0.2412,0.6377,0.2949,2.6278,0.2994,70.4168,INS_SOLUTION_GOOD*12345678\r\n",
	)
	for name, parse := range map[string]func(string) (arrow.Table, error){"inspva": INSPVA, "insstdev": INSSTDEV} {
		tbl, err := parse(path)
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if name == "inspva" {
			want = 1
		}
		if n := tbl.NumRows(); n != want {
			t.Errorf("%s: %d rows, want %d (the week-0 log skipped)", name, n, want)
		}
		tbl.Release()
	}
}
