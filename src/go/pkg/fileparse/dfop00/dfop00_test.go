package dfop00

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Two ping/reply cycles: the first reply is real, the second is a zero
// range (no reply). The second ping's AHRS block is an error code. The first
// ping and its reply repeat one NOV_INS block; the second ping's has no time
// fix (time status UNKNOWN), and its reply's has an INS status code outside
// NovAtel's table.
const sample = `{"event":"interrogation","event_id":1,"observations":{"AHRS":{"h":10.5,"p":1,"r":-2,"time":{"common":1780460559.981377,"instrument":8536.68,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3","NOV_INS":{"gpst":1464495778.0,"h":24.151555565,"hae":-30.2147,"latitude":41.68552296236,"longitude":-125.53111825632,"p":-3.373163567,"r":5.863288138,"receiver_status":"02000000","solution_type":3,"time":{"common":1780460560.0027198,"instrument":8536.69,"start_count":45,"status":"GPS_SYNC"},"time_status":180,"vx":0.1632,"vy":0.2666,"vz":0.4615}},"sequence":1,"time":{"common":1780460560.0,"instrument":8536.69,"start_count":45,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":1,"observations":{"AHRS":{"h":10.5,"p":1,"r":-2,"time":{"common":1780460559.981377,"instrument":8536.68,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3","NOV_INS":{"gpst":1464495778.0,"h":24.151555565,"hae":-30.2147,"latitude":41.68552296236,"longitude":-125.53111825632,"p":-3.373163567,"r":5.863288138,"receiver_status":"02000000","solution_type":3,"time":{"common":1780460560.0027198,"instrument":8536.69,"start_count":45,"status":"GPS_SYNC"},"time_status":180,"vx":0.1632,"vy":0.2666,"vz":0.4615}},"range":{"cn":"IR5209","diag":{"dbv":[-12.0],"snr":[18.5],"xc":[87.0]},"range":2.534127,"tat":200},"sequence":1,"time":{"common":1780460562.404127,"instrument":8539.1,"start_count":45,"status":"GPS_SYNC"},"uid":"0042D9"}
{"event":"interrogation","event_id":2,"observations":{"AHRS":"ERR1","GNSS":"ERR3","NOV_INS":{"gpst":1464495790.0,"h":1,"hae":1,"latitude":1,"longitude":1,"p":1,"r":1,"solution_type":0,"time":{"common":1780460572.0,"instrument":8548.7,"start_count":45,"status":"GPS_SYNC"},"time_status":20,"vx":0,"vy":0,"vz":0}},"sequence":2,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":2,"observations":{"AHRS":{"h_mag":68.6,"time":{"common":1780460575.5,"instrument":8552.2,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3","NOV_INS":{"gpst":1464495793.0,"h":25.5,"hae":-30.1,"latitude":41.6855,"longitude":-125.5311,"p":-3.0,"r":5.0,"receiver_status":"02000000","solution_type":99,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"time_status":160,"vx":0.1,"vy":0.2,"vz":0.3}},"range":{"cn":"IR5210","diag":{"dbv":0,"snr":0,"xc":21},"range":0.0,"tat":200},"sequence":2,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"uid":"0042DA"}
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
	tbl, err := TWTT(writeSample(t))
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
	if got := col[*array.String](t, tbl, "Log_Source").Value(0); got != datamodels.LogINTGRPLY {
		t.Errorf("Log_Source = %q", got)
	}
}

func TestINSPVA(t *testing.T) {
	tbl, err := INSPVA(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := datamodels.ParsedSchema(datamodels.INSPVA); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want inspva parsed schema %v", tbl.Schema(), want)
	}
	if tbl.NumRows() != 2 {
		t.Fatalf("%d rows, want 2 (one repeated block, one with no time fix)", tbl.NumRows())
	}
	// gpst is GPS seconds; time is UTC, 18 leap seconds behind.
	if got := col[*array.Timestamp](t, tbl, "time").Value(0); got != 1780460560000000 {
		t.Errorf("time = %d", got)
	}
	for name, want := range map[string]float64{
		"lat": 41.68552296236, "lon": -125.53111825632, "alt": -30.2147,
		"north": 0.1632, "east": 0.2666, "up": 0.4615,
		"roll": 5.863288138, "pitch": -3.373163567, "heading": 24.151555565,
	} {
		if got := col[*array.Float64](t, tbl, name).Value(0); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	ins, ts := col[*array.String](t, tbl, "ins_status"), col[*array.String](t, tbl, "time_status")
	if ins.Value(0) != "INS_SOLUTION_GOOD" || ts.Value(0) != "FINESTEERING" {
		t.Errorf("statuses = %q, %q", ins.Value(0), ts.Value(0))
	}
	if ins.Value(1) != "99" || ts.Value(1) != "FINE" {
		t.Errorf("statuses = %q, %q, want the unknown code as logged", ins.Value(1), ts.Value(1))
	}
	if got := col[*array.String](t, tbl, datamodels.LogSourceColumn).Value(0); got != datamodels.LogINSPVAA {
		t.Errorf("Log_Source = %q", got)
	}
}
