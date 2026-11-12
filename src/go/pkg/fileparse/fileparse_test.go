package fileparse

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/ahrs00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel770"
)

var testScope = datamodels.Scope{Network: "net", Station: "STA", Campaign: "2026_A_0001"}

func stringCol(t *testing.T, tbl arrow.Table, name string) []string {
	t.Helper()
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) != 1 {
		t.Fatalf("no column %q", name)
	}
	var out []string
	for _, c := range tbl.Column(idx[0]).Data().Chunks() {
		s := c.(*array.String)
		for i := range s.Len() {
			out = append(out, s.Value(i))
		}
	}
	return out
}

// A builder called as a parser calls it gives the table's full schema, with
// the parser's stream_edid on every row and no other identity column.
func TestParsedRowsCarryEDID(t *testing.T) {
	t0 := time.UnixMicro(1_700_000_000_000_000).UTC()
	p := parserFor(t, ahrs00.FileType, datamodels.AttitudeAHRS.Name)
	tbl, err := datamodels.BuildAttitudeAHRS([]datamodels.AttitudeAHRSRow{
		{Time: t0, TimeInstrument: 0},
		{Time: t0.Add(time.Microsecond), TimeInstrument: 1},
	}, p.StreamEDID(testScope, datamodels.LogAHRS))
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if err := datamodels.AttitudeAHRS.Validate(tbl); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Network", "Station", "Campaign", "Log_Source"} {
		if tbl.Schema().HasField(name) {
			t.Errorf("rows have a %s column", name)
		}
	}
	const want = "net.STA.2026_A_0001.AHRS00.AHRS"
	if got := stringCol(t, tbl, datamodels.StreamEDIDName); len(got) != 2 || got[0] != want || got[1] != want {
		t.Errorf("stream_edid = %q, want %q", got, want)
	}
}

// NOV770 rows from two antennas are two streams: each source's rows carry
// its own EDID.
func TestGNSSObsAntennaStreams(t *testing.T) {
	sat := observation.NewSatellite(gnss.GPS, 30)
	sat.Observations = append(sat.Observations,
		observation.Observation{Code: obsKey(t, gnss.GPS, "1C"), Pseudorange: 21007753.481})
	epoch := observation.Epoch{Time: time.Date(2023, 6, 21, 17, 11, 1, 0, time.UTC),
		Satellites: observation.Satellites{sat}}
	p := parserFor(t, novatel770.FileType, datamodels.GNSSObs.Name)
	second := datamodels.LogSource(novatel770.LogRANGECMP5B, 1)
	tbl, err := datamodels.BuildGNSSObs(p.FileType,
		datamodels.GNSSObsSource{EDID: p.StreamEDID(testScope, novatel770.LogRANGECMP5B), Epochs: []observation.Epoch{epoch}},
		datamodels.GNSSObsSource{EDID: p.StreamEDID(testScope, second), Epochs: []observation.Epoch{epoch}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if err := datamodels.GNSSObs.Validate(tbl); err != nil {
		t.Fatal(err)
	}
	got := stringCol(t, tbl, datamodels.StreamEDIDName)
	want := []string{"net.STA.2026_A_0001.NOV770.RANGECMP5B", "net.STA.2026_A_0001.NOV770.RANGECMP5B_1"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("stream_edid = %q, want %q", got, want)
	}
}

// TestParsers guards the registry: every parser has a ParseFile, and no
// (file type, table) is listed twice, which would parse and append its rows
// twice.
func TestParsers(t *testing.T) {
	seen := map[[2]string]bool{}
	for _, p := range Parsers {
		if p.ParseFile == nil {
			t.Errorf("%s %s: ParseFile is nil", p.FileType, p.Table.Name)
		}
		key := [2]string{p.FileType, p.Table.Name}
		if seen[key] {
			t.Errorf("%s %s: listed twice", p.FileType, p.Table.Name)
		}
		seen[key] = true
	}
}
