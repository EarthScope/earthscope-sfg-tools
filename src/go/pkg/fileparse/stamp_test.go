package fileparse

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
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

// TestStampAttitude stamps parsed attitude_ahrs rows: the result has the
// full schema, with one stream_edid from scope and Log_Source, and no
// Log_Source.
func TestStampAttitude(t *testing.T) {
	t0 := time.UnixMicro(1_700_000_000_000_000).UTC()
	parsed, err := datamodels.BuildAttitudeAHRS([]datamodels.AttitudeAHRSRow{
		{Time: t0, TimeInstrument: 0},
		{Time: t0.Add(time.Microsecond), TimeInstrument: 1},
	}, datamodels.LogAHRS)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Release()

	p := parserFor(t, ahrs00.FileType, datamodels.AttitudeAHRS.Name)
	tbl, err := p.stamp(parsed, testScope)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if err := datamodels.AttitudeAHRS.Validate(tbl); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Network", "Station", "Campaign", datamodels.LogSourceColumn} {
		if tbl.Schema().HasField(name) {
			t.Errorf("stamped table has a %s column", name)
		}
	}
	const want = "net.STA.2026_A_0001.AHRS00.AHRS"
	if got := stringCol(t, tbl, "stream_edid"); len(got) != 2 || got[0] != want || got[1] != want {
		t.Errorf("stream_edid = %q, want %q", got, want)
	}
}

// TestStampGNSSObsAntennas stamps NOV770 rows from two antennas: each row's
// stream_edid follows its Log_Source, and Log_Source is dropped.
func TestStampGNSSObsAntennas(t *testing.T) {
	sat := observation.NewSatellite(gnss.GPS, 30)
	sat.Observations = append(sat.Observations,
		observation.Observation{Code: obsKey(t, gnss.GPS, "1C"), Pseudorange: 21007753.481})
	epoch := observation.Epoch{Time: time.Date(2023, 6, 21, 17, 11, 1, 0, time.UTC),
		Satellites: observation.Satellites{sat}}
	p := parserFor(t, novatel770.FileType, datamodels.GNSSObs.Name)
	parsed, err := datamodels.BuildGNSSObs(p.FileType,
		datamodels.GNSSObsSource{LogSource: novatel770.LogRANGECMP5B, Epochs: []observation.Epoch{epoch}},
		datamodels.GNSSObsSource{LogSource: datamodels.LogSource(novatel770.LogRANGECMP5B, 1), Epochs: []observation.Epoch{epoch}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Release()

	tbl, err := p.stamp(parsed, testScope)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if err := datamodels.GNSSObs.Validate(tbl); err != nil {
		t.Fatal(err)
	}
	if tbl.Schema().HasField(datamodels.LogSourceColumn) {
		t.Error("stamp should drop Log_Source")
	}
	got := stringCol(t, tbl, "stream_edid")
	want := []string{"net.STA.2026_A_0001.NOV770.RANGECMP5B", "net.STA.2026_A_0001.NOV770.RANGECMP5B_1"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("stream_edid = %q, want %q", got, want)
	}
	if e := p.StreamEDID(testScope, datamodels.LogSource(novatel770.LogRANGECMP5B, 1)); e != want[1] {
		t.Errorf("EDID = %q, want %q", e, want[1])
	}
}

func TestStampMissingColumn(t *testing.T) {
	sc := arrow.NewSchema([]arrow.Field{{Name: datamodels.LogSourceColumn, Type: arrow.BinaryTypes.String}}, nil)
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	b.Field(0).(*array.StringBuilder).Append("AHRS00")
	rec := b.NewRecordBatch()
	defer rec.Release()
	parsed := array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
	defer parsed.Release()
	if tbl, err := parserFor(t, ahrs00.FileType, datamodels.AttitudeAHRS.Name).stamp(parsed, testScope); err == nil {
		tbl.Release()
		t.Fatal("want an error: parsed table has no time column")
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
