package tcvr00

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// packet frames data as a GPSA packet stamped at t (UNIX UTC).
func packet(t time.Time, data string) []byte {
	body := make([]byte, 18, 18+len(data)+1)
	binary.BigEndian.PutUint16(body[0:2], 990)
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

type msg struct {
	at   time.Duration // after t0
	text string
}

var t0 = time.Date(2025, 9, 14, 6, 32, 41, 0, time.UTC)

// session is two pings of NBR1 2025 (the first with all three replies, one
// undetected), a settings exchange, and a stray result with no trigger.
var session = []msg{
	{0, "<US"},
	{100 * time.Millisecond, ">CS:2010,TAT640,BLK100,RXW3600,TXW100,NPL196,TPL196,LG26,CIS1,AT8,EC0,ME0,RSP0,PPR0"},
	{110 * time.Millisecond, ">PORT:2010,P0;BR9600;CF0;RS232,P1;BR9600;CF0;RS232"},
	{120 * time.Millisecond, ">US:2010,SN341391-001,TDR3,TYPE8290"},
	{130 * time.Millisecond, ">FS:2010,U007239,FL031D,FV3.12.08.09,PV2.03,TDR;MF;DIR;202;199;196;190;169"},
	{140 * time.Millisecond, ">DIAG:2010,IFL,DBV,SNR,XC,P0,DEV0,SYS0,MOD0,RX0,TX0,ERR0"},
	{150 * time.Millisecond, "\x00"},
	{181 * time.Millisecond, "<IR:,IR5209,IR5210,IR5211"},
	{329 * time.Millisecond, ">IR:2010,EVT0,RXW3600,IR5209;RG0;W3600;LG26,IR5210;RG0;W3600;LG26,IR5211;RG0;W3600;LG26"},
	{330 * time.Millisecond, "<IT:CI4805;D130.000000"},
	{411 * time.Millisecond, ">IT:2010,EVT0,CI4805;D130;NPL196"},
	{413 * time.Millisecond, "<TRG,BR"},
	{4692 * time.Millisecond, ">BR:2010,EVT0,SQ0,IR5209;R3464032[XC93,SNR25,DBV-20,TEL;NONE,DOP0.00]," +
		"IR5210;R3515879[XC92,SNR28,DBV-18,TEL;NONE,DOP0.00],IR5211;R0[XC22,SNR0,DBV0,TEL;NONE,DOP0.00]"},
	// A second ping to one transponder (SLT1 2026's wake-up tone variant).
	{15181 * time.Millisecond, "<IR:,IR5209"},
	{15259 * time.Millisecond, ">IR:2010,EVT0,RXW3600,IR5209;RG0;W3600;LG26"},
	{15260 * time.Millisecond, "<IT:WKT1;D50,CI4805;D130.000000"},
	{15343 * time.Millisecond, ">IT:2010,EVT0,CI4805;D130;NPL196"},
	{15344 * time.Millisecond, "<TRG,BR"},
	{19775 * time.Millisecond, ">BR:2010,EVT0,SQ0,IR5209;R0[XC21,SNR0,DBV0,TEL;NONE,DOP0.00]"},
	// No trigger since the last result: skipped.
	{20000 * time.Millisecond, ">BR:2010,EVT0,SQ0,IR5209;R3500000[XC90,SNR20,DBV-20,TEL;NONE,DOP0.00]"},
}

func writeSession(t *testing.T) string {
	t.Helper()
	var buf []byte
	for _, m := range session {
		buf = append(buf, packet(t0.Add(m.at), m.text)...)
	}
	path := filepath.Join(t.TempDir(), "x_TCVR00.bin")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func col[T arrow.Array](tbl arrow.Table, name string) T {
	return tbl.Column(tbl.Schema().FieldIndices(name)[0]).Data().Chunk(0).(T)
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

func TestInterrogations(t *testing.T) {
	tbl, err := Interrogations(writeSession(t))
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkSchema(t, tbl, datamodels.TransceiverInterrogations, 2)

	if tm := col[*array.Timestamp](tbl, "time"); int64(tm.Value(0)) != t0.Add(413*time.Millisecond).UnixMicro() {
		t.Errorf("time = %v, want the trigger's packet time", time.UnixMicro(int64(tm.Value(0))).UTC())
	}
	targets := col[*array.String](tbl, "targets")
	if targets.Value(0) != "IR5209,IR5210,IR5211" || targets.Value(1) != "IR5209" {
		t.Errorf("targets = %q, %q", targets.Value(0), targets.Value(1))
	}
	if s := col[*array.String](tbl, "target_settings").Value(1); s != "IR5209;RG0;W3600;LG26" {
		t.Errorf("target_settings = %q", s)
	}
	if u := col[*array.String](tbl, "unit").Value(0); u != "2010" {
		t.Errorf("unit = %q", u)
	}
	if ch := col[*array.String](tbl, "tx_channel").Value(1); ch != "CI4805" {
		t.Errorf("tx_channel = %q", ch)
	}
	if d := col[*array.Float64](tbl, "delay").Value(0); d != 130 {
		t.Errorf("delay = %v, want 130 ms", d)
	}
	if w, n := col[*array.Int32](tbl, "rx_window").Value(0), col[*array.Int32](tbl, "npl").Value(0); w != 3600 || n != 196 {
		t.Errorf("rx_window, npl = %d, %d", w, n)
	}
}

func TestRanges(t *testing.T) {
	tbl, err := Ranges(writeSession(t))
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkSchema(t, tbl, datamodels.TransceiverRanges, 4) // 3 + 1; the untriggered result is skipped

	tm, reply := col[*array.Timestamp](tbl, "time"), col[*array.Timestamp](tbl, "reply_time")
	if int64(tm.Value(0)) != t0.Add(413*time.Millisecond).UnixMicro() || int64(reply.Value(0)) != t0.Add(4692*time.Millisecond).UnixMicro() {
		t.Errorf("time/reply_time = %v / %v", time.UnixMicro(int64(tm.Value(0))).UTC(), time.UnixMicro(int64(reply.Value(0))).UTC())
	}
	id, det := col[*array.String](tbl, "MT_ID"), col[*array.Boolean](tbl, "detected")
	tt := col[*array.Float64](tbl, "TravelTime")
	// IR5209's range equals DFOP00's 3.464032 s for the same reply.
	if id.Value(0) != "IR5209" || !det.Value(0) || tt.Value(0) != 3.464032 {
		t.Errorf("row 0 = %s detected %v range %v", id.Value(0), det.Value(0), tt.Value(0))
	}
	if id.Value(2) != "IR5211" || det.Value(2) || !tt.IsNull(2) {
		t.Errorf("row 2 = %s detected %v, range null %v; want undetected with null range", id.Value(2), det.Value(2), tt.IsNull(2))
	}
	acc, snr, dbv := col[*array.Int32](tbl, "acc"), col[*array.Float64](tbl, "aSNR"), col[*array.Float32](tbl, "dbV")
	if acc.Value(1) != 92 || snr.Value(1) != 28 || dbv.Value(1) != -18 {
		t.Errorf("row 1 acc/snr/dbv = %v %v %v", acc.Value(1), snr.Value(1), dbv.Value(1))
	}
	if tel := col[*array.String](tbl, "telemetry").Value(0); tel != "NONE" {
		t.Errorf("telemetry = %q", tel)
	}
	if tm := col[*array.Timestamp](tbl, "time"); int64(tm.Value(3)) != t0.Add(15344*time.Millisecond).UnixMicro() {
		t.Errorf("row 3 time = %v, want the second trigger", time.UnixMicro(int64(tm.Value(3))).UTC())
	}
}

func TestSettings(t *testing.T) {
	tbl, err := Settings(writeSession(t))
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkSchema(t, tbl, datamodels.TransceiverSettings, 13+2+3+5+11) // CS, PORT; US, FS, DIAG
	ls, cmd := col[*array.String](tbl, datamodels.LogSourceColumn), col[*array.String](tbl, "command")
	key, val := col[*array.String](tbl, "key"), col[*array.String](tbl, "value")
	want := map[string]string{"CS": LogConfig, "PORT": LogConfig, "US": LogInfo, "FS": LogInfo, "DIAG": LogInfo}
	var tat, fv bool
	for i := 0; i < key.Len(); i++ {
		if ls.Value(i) != want[cmd.Value(i)] {
			t.Errorf("row %d: %s reply has Log_Source %q, want %q", i, cmd.Value(i), ls.Value(i), want[cmd.Value(i)])
		}
		tat = tat || (cmd.Value(i) == "CS" && key.Value(i) == "TAT" && val.Value(i) == "640")
		fv = fv || (cmd.Value(i) == "FS" && key.Value(i) == "FV" && val.Value(i) == "3.12.08.09")
	}
	if !tat || !fv {
		t.Errorf("missing CS TAT=640 (%v) or FS FV=3.12.08.09 (%v)", tat, fv)
	}
}

func TestKeyValue(t *testing.T) {
	for in, want := range map[string][2]string{
		"TAT640":              {"TAT", "640"},
		"SN341391-001":        {"SN", "341391-001"},
		"P0;BR9600;CF0;RS232": {"P0", "BR9600;CF0;RS232"},
		"TDR;MF;DIR;202":      {"TDR", "MF;DIR;202"},
		"IFL":                 {"IFL", ""},
		"DBV-20":              {"DBV", "-20"},
		"TEL;NONE":            {"TEL", "NONE"},
	} {
		if k, v := keyValue(in); k != want[0] || v != want[1] {
			t.Errorf("keyValue(%q) = %q, %q; want %q, %q", in, k, v, want[0], want[1])
		}
	}
}

// TestParseFile runs on a real TCVR00.bin when SFG_TCVR00 names one.
func TestParseFile(t *testing.T) {
	path := os.Getenv("SFG_TCVR00")
	if path == "" {
		t.Skip("set SFG_TCVR00 to a TCVR00.bin file")
	}
	for _, c := range []struct {
		name string
		f    func(string) (arrow.Table, error)
	}{{"interrogations", Interrogations}, {"ranges", Ranges}, {"settings", Settings}} {
		tbl, err := c.f(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s %s: %d rows", filepath.Base(path), c.name, tbl.NumRows())
		tbl.Release()
	}
}
