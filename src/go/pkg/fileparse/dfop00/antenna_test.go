package dfop00

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/coordinates"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// Events 1 and 2 repeat one GNSS block; event 3 reports the -1 sigma
// sentinel; event 4's GNSS logged an error code; event 5's block has no
// latitude.
const gnssSample = `{"event":"interrogation","event_id":1,"observations":{"GNSS":{"hae":0,"latitude":0,"longitude":0,"q":4,"sdx":1,"sdy":2,"sdz":3,"separation":-30.1,"time":{"common":1757831562.0,"instrument":4974.79,"start_count":14,"status":"GPS_SYNC"}}},"time":{"common":1757831562.1,"instrument":4974.8,"start_count":14,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":1,"observations":{"GNSS":{"hae":0,"latitude":0,"longitude":0,"q":4,"sdx":1,"sdy":2,"sdz":3,"separation":-30.1,"time":{"common":1757831562.0,"instrument":4974.79,"start_count":14,"status":"GPS_SYNC"}}},"range":{"cn":"IR5209","diag":{"dbv":-12,"snr":18,"xc":87},"range":2.5,"tat":200},"time":{"common":1757831564.6,"instrument":4977.3,"start_count":14,"status":"GPS_SYNC"}}
{"event":"interrogation","event_id":2,"observations":{"GNSS":{"hae":-31.878,"latitude":42.130032858,"longitude":-125.111838148,"q":0,"sdx":-1.0,"sdy":-1.0,"sdz":-1.0,"separation":-30.1,"time":{"common":1757831577.25,"instrument":4990.0,"start_count":14,"status":"GPS_SYNC"}}},"time":{"common":1757831577.3,"instrument":4990.1,"start_count":14,"status":"GPS_SYNC"},"type":"BR"}
{"event":"interrogation","event_id":3,"observations":{"GNSS":"ERR3"},"time":{"common":1757831592.0,"instrument":5005.0,"start_count":14,"status":"GPS_SYNC"},"type":"BR"}
{"event":"interrogation","event_id":4,"observations":{"GNSS":{"hae":0,"longitude":0,"q":1,"sdx":1,"sdy":1,"sdz":1,"time":{"common":1757831607.0,"instrument":5020.0,"start_count":14,"status":"GPS_SYNC"}}},"time":{"common":1757831607.0,"instrument":5020.0,"start_count":14,"status":"GPS_SYNC"},"type":"BR"}
`

func TestAntennaPositions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x_DFOP00.raw")
	if err := os.WriteFile(path, []byte(gnssSample), 0o644); err != nil {
		t.Fatal(err)
	}
	tbl, err := AntennaPositions(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := datamodels.ParsedSchema(datamodels.GNSSAntennaPositions); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want gnss_antenna_positions parsed schema %v", tbl.Schema(), want)
	}
	if tbl.NumRows() != 2 {
		t.Fatalf("%d rows, want 2 (one per distinct usable fix)", tbl.NumRows())
	}

	x := col[*array.Float64](t, tbl, "ant_x")
	y := col[*array.Float64](t, tbl, "ant_y")
	z := col[*array.Float64](t, tbl, "ant_z")
	if x.Value(0) != coordinates.EarthEquatorialRadius || y.Value(0) != 0 || z.Value(0) != 0 {
		t.Errorf("row 0 ECEF = %v %v %v, want (a, 0, 0)", x.Value(0), y.Value(0), z.Value(0))
	}
	// pymap3d.geodetic2ecef(42.130032858, -125.111838148, -31.878)
	if !near(x.Value(1), -2724772.3609716753, 1e-6) || !near(y.Value(1), -3875256.6761967745, 1e-6) || !near(z.Value(1), 4256304.969726341, 1e-6) {
		t.Errorf("row 1 ECEF = %v %v %v", x.Value(1), y.Value(1), z.Value(1))
	}

	sigX := col[*array.Float64](t, tbl, "ant_sigx")
	sigY := col[*array.Float64](t, tbl, "ant_sigy")
	sigZ := col[*array.Float64](t, tbl, "ant_sigz")
	if sigX.Value(0) != 3 || sigY.Value(0) != 1 || sigZ.Value(0) != 2 {
		t.Errorf("row 0 sigmas = %v %v %v, want up/east/north = 3 1 2", sigX.Value(0), sigY.Value(0), sigZ.Value(0))
	}
	if !sigX.IsNull(1) || !sigY.IsNull(1) || !sigZ.IsNull(1) {
		t.Error("row 1: the -1 sentinel must null all three sigmas")
	}
	q := col[*array.Int32](t, tbl, "q")
	if q.Value(0) != 4 || q.Value(1) != 0 {
		t.Errorf("q = %v", q)
	}
	for _, name := range []string{"number_of_satellites", "pdop", "wrms"} {
		if c := tbl.Column(tbl.Schema().FieldIndices(name)[0]); c.NullN() != c.Len() {
			t.Errorf("%s: want all null (SV3 GNSS blocks do not report it)", name)
		}
	}
}
