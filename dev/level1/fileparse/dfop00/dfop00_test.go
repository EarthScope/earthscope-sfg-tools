package dfop00

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// Two ping/reply cycles: the first reply is real, the second is a zero
// range (no reply). The second ping's AHRS block is an error code.
const sample = `{"event":"interrogation","event_id":1,"observations":{"AHRS":{"h":10.5,"p":1,"r":-2,"time":{"common":1780460559.981377,"instrument":8536.68,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3"},"sequence":1,"time":{"common":1780460560.0,"instrument":8536.69,"start_count":45,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":1,"observations":{"AHRS":{"h":10.5,"p":1,"r":-2,"time":{"common":1780460559.981377,"instrument":8536.68,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3"},"range":{"cn":"IR5209","diag":{"dbv":[-12.0],"snr":[18.5],"xc":[87.0]},"range":2.534127,"tat":200},"sequence":1,"time":{"common":1780460562.404127,"instrument":8539.1,"start_count":45,"status":"GPS_SYNC"},"uid":"0042D9"}
{"event":"interrogation","event_id":2,"observations":{"AHRS":"ERR1","GNSS":"ERR3"},"sequence":2,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":2,"observations":{"AHRS":{"h_mag":68.6,"time":{"common":1780460575.5,"instrument":8552.2,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3"},"range":{"cn":"IR5210","diag":{"dbv":0,"snr":0,"xc":21},"range":0.0,"tat":200},"sequence":2,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"uid":"0042DA"}
not json
`

func writeSample(t *testing.T) string {
	path := filepath.Join(t.TempDir(), "x_DFOP00.raw")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func col[T arrow.Array](t *testing.T, tbl arrow.Table, name string) T {
	t.Helper()
	return tbl.Column(tbl.Schema().FieldIndices(name)[0]).Data().Chunk(0).(T)
}

func TestTWTT(t *testing.T) {
	tbl, err := TWTT(writeSample(t), LogType)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := datamodels.ParsedSchema(datamodels.TWTT); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want TWTT parsed schema %v", tbl.Schema(), want)
	}
	if tbl.NumRows() != 1 {
		t.Fatalf("%d rows, want 1 (the zero range has no reply)", tbl.NumRows())
	}
	if got := col[*array.String](t, tbl, "MT_ID").Value(0); got != "IR5209" {
		t.Errorf("MT_ID = %q", got)
	}
	if got := col[*array.Timestamp](t, tbl, "T_transmit").Value(0); got != 1780460560000000 {
		t.Errorf("T_transmit = %d", got)
	}
	if got := col[*array.Timestamp](t, tbl, "T_receive").Value(0); got != 1780460562404127 {
		t.Errorf("T_receive = %d", got)
	}
	if got := col[*array.Float64](t, tbl, "TravelTime").Value(0); got != 2.534127 {
		t.Errorf("TravelTime = %v, want the raw range", got)
	}
	if got := col[*array.Float64](t, tbl, "tat").Value(0); got != 0.2 {
		t.Errorf("tat = %v, want 0.2 s", got)
	}
	if got := col[*array.Int32](t, tbl, "acc").Value(0); got != 87 {
		t.Errorf("acc = %v", got)
	}
	if got := col[*array.Float32](t, tbl, "dbV").Value(0); got != -12 {
		t.Errorf("dbV = %v", got)
	}
	if got := col[*array.String](t, tbl, "Log_Source").Value(0); got != LogType {
		t.Errorf("Log_Source = %q", got)
	}
}

func TestAttitude(t *testing.T) {
	tbl, err := AttitudeAHRS(writeSample(t), LogType)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := datamodels.ParsedSchema(datamodels.AttitudeAHRS); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want attitude_ahrs parsed schema %v", tbl.Schema(), want)
	}
	// Events 1 and 2 repeat one block, event 3 logs an error code.
	if tbl.NumRows() != 2 {
		t.Fatalf("%d rows, want 2", tbl.NumRows())
	}
	h := col[*array.Float64](t, tbl, "h")
	hMag := col[*array.Float64](t, tbl, "h_mag")
	if h.Value(0) != 10.5 || !hMag.IsNull(0) || !h.IsNull(1) || hMag.Value(1) != 68.6 {
		t.Errorf("h = %v, h_mag = %v", h, hMag)
	}
	if got := col[*array.Int32](t, tbl, "time_start_count").Value(1); got != 45 {
		t.Errorf("time_start_count = %d", got)
	}
}

func TestUnknownLogType(t *testing.T) {
	if _, err := TWTT(writeSample(t), "RANGEA"); err == nil {
		t.Fatal("want an error: DFOP00 files have no RANGEA logs")
	}
}
