package datamodels

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

func f64(v float64) *float64 { return &v }
func i32(v int32) *int32     { return &v }
func strp(v string) *string  { return &v }

// checkParsed fails unless tbl has def's parsed schema and n rows.
func checkParsed(t *testing.T, tbl arrow.Table, def TableDef, n int64) {
	t.Helper()
	want, err := ParsedSchema(def)
	if err != nil {
		t.Fatal(err)
	}
	if !tbl.Schema().Equal(want) {
		t.Fatalf("%s: schema = %v, want %v", def.Name, tbl.Schema(), want)
	}
	if tbl.NumRows() != n {
		t.Fatalf("%s: %d rows, want %d", def.Name, tbl.NumRows(), n)
	}
}

func nulls(tbl arrow.Table, name string) int {
	return tbl.Column(tbl.Schema().FieldIndices(name)[0]).NullN()
}

func TestBuildAttitudeAHRS(t *testing.T) {
	t0 := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)
	tbl, err := BuildAttitudeAHRS([]AttitudeAHRSRow{
		{Time: t0, TimeInstrument: 1.5, Acx: f64(0.1), Acy: f64(0.2), Acz: f64(9.8), H: f64(10), HMag: f64(12), P: f64(1), R: f64(2)},
		{Time: t0.Add(100 * time.Millisecond), TimeInstrument: 1.6, TimeStartCount: i32(45), TimeStatus: strp("OK"), HMag: f64(13)},
	}, "AHRS00")
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkParsed(t, tbl, AttitudeAHRS, 2)
	for name, n := range map[string]int{"time_start_count": 1, "time_status": 1, "h": 1, "h_mag": 0, "acx": 1} {
		if got := nulls(tbl, name); got != n {
			t.Errorf("%s has %d nulls, want %d", name, got, n)
		}
	}
	tm := tbl.Column(tbl.Schema().FieldIndices("time")[0]).Data().Chunk(0).(*array.Timestamp)
	if got := int64(tm.Value(1)); got != t0.Add(100*time.Millisecond).UnixMicro() {
		t.Errorf("time = %d", got)
	}
}

func TestBuildTWTT(t *testing.T) {
	t0 := time.Date(2026, 6, 3, 4, 22, 40, 0, time.UTC)
	tbl, err := BuildTWTT([]TWTTRow{
		{MTID: "IR5209", TTransmit: t0, TReceive: t0.Add(2 * time.Second), TravelTime: 2.5, DBV: -12, Acc: 87, ASNR: f64(20), TAT: f64(0.2)},
		{MTID: "IR5210", TTransmit: t0, TReceive: t0.Add(3 * time.Second), TravelTime: 3.1, DBV: -10, Acc: 90},
	}, "DFOP00")
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkParsed(t, tbl, TWTT, 2)
	if nulls(tbl, "aSNR") != 1 || nulls(tbl, "tat") != 1 {
		t.Errorf("nil aSNR/tat should be null: %d, %d", nulls(tbl, "aSNR"), nulls(tbl, "tat"))
	}
}

func TestBuildINS(t *testing.T) {
	gps := time.Date(2023, 6, 21, 17, 11, 1, 50_000_000, time.UTC)
	tbl, err := BuildINSPVA([]INSPVARow{{Time: gps, Lat: 45.3, Lon: -124.9, Azimuth: 70.4}}, "INSPVAA")
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	checkParsed(t, tbl, INSPVA, 1)
	// 2023-06 GPS time is 18 s ahead of UTC.
	want := gps.Add(-18 * time.Second).UnixMicro()
	if got := tbl.Column(tbl.Schema().FieldIndices("time")[0]).Data().Chunk(0).(*array.Timestamp).Value(0); int64(got) != want {
		t.Errorf("time = %v, want UTC %v", time.UnixMicro(int64(got)).UTC(), time.UnixMicro(want).UTC())
	}

	tbl2, err := BuildINSSTDEV([]INSSTDEVRow{
		{Time: gps, AzimuthSig: 0.4, ExtSolStat: strp("2e000041"), TimeSinceUpdate: strp("0")},
		{Time: gps.Add(time.Second)},
	}, "INSSTDEVA")
	if err != nil {
		t.Fatal(err)
	}
	defer tbl2.Release()
	checkParsed(t, tbl2, INSSTDEV, 2)
	if nulls(tbl2, "ext_sol_stat") != 1 || nulls(tbl2, "time_since_update") != 1 {
		t.Error("nil ext_sol_stat/time_since_update should be null")
	}
}

// TestBuildersFillEveryColumn builds one fully populated record per table
// and checks every parsed column is non-null, so a column added to a
// TableDef but not to its record type and builder fails here.
func TestBuildersFillEveryColumn(t *testing.T) {
	t0 := time.Date(2023, 6, 21, 17, 11, 1, 0, time.UTC)
	lock := 12.5
	glo := observation.NewSatellite(gnss.GLONASS, 2)
	glo.Fcn = -3
	k, err := observation.ParseObservationKey(gnss.GLONASS, "1C")
	if err != nil {
		t.Fatal(err)
	}
	glo.Observations = append(glo.Observations, observation.Observation{Code: k,
		Pseudorange: 19660946.553, Phase: 105283811.1, Doppler: -1631.8, SNR: 44, LockTime: &lock})

	build := map[string]func() (arrow.Table, error){
		AttitudeAHRS.Name: func() (arrow.Table, error) {
			return BuildAttitudeAHRS([]AttitudeAHRSRow{{Time: t0, TimeInstrument: 1, TimeStartCount: i32(1), TimeStatus: strp("OK"),
				Acx: f64(0), Acy: f64(0), Acz: f64(0), H: f64(0), HMag: f64(0), P: f64(0), R: f64(0)}}, "x")
		},
		TWTT.Name: func() (arrow.Table, error) {
			return BuildTWTT([]TWTTRow{{MTID: "m", TTransmit: t0, TReceive: t0, TravelTime: 1, DBV: 1, Acc: 1, ASNR: f64(1), TAT: f64(1)}}, "x")
		},
		INSPVA.Name: func() (arrow.Table, error) {
			return BuildINSPVA([]INSPVARow{{Time: t0, INSStatus: strp("INS_SOLUTION_GOOD"), TimeStatus: strp("FINESTEERING")}}, "x")
		},
		INSSTDEV.Name: func() (arrow.Table, error) {
			return BuildINSSTDEV([]INSSTDEVRow{{Time: t0, ExtSolStat: strp("s"), TimeSinceUpdate: strp("0"), TimeStatus: strp("FINESTEERING")}}, "x")
		},
		SoundSpeed.Name: func() (arrow.Table, error) {
			return BuildSoundSpeed([]SoundSpeedRow{{Time: t0, Depth: 1, Speed: 1500}}, "x")
		},
		GNSSAntennaPositions.Name: func() (arrow.Table, error) {
			return BuildGNSSAntennaPositions([]GNSSAntennaPositionRow{{Time: t0, SigX: f64(1), SigY: f64(1), SigZ: f64(1), Q: 1,
				NumberOfSatellites: i32(9), PDOP: f64(1), WRMS: f64(1)}}, "x")
		},
		GNSSObs.Name: func() (arrow.Table, error) {
			return BuildGNSSObs("x", GNSSObsSource{LogSource: "x", Epochs: []observation.Epoch{{Time: t0,
				ReceiverClockOffset: 1e-7, EpochFlag: 1, Satellites: observation.Satellites{glo}}}})
		},
	}
	// Columns no Level 1 source supplies yet.
	alwaysNull := map[string]bool{"arrival_time": true}
	for _, def := range Level1Tables {
		f, ok := build[def.Name]
		if !ok {
			t.Errorf("%s: no builder in this test", def.Name)
			continue
		}
		tbl, err := f()
		if err != nil {
			t.Fatalf("%s: %v", def.Name, err)
		}
		for i, fld := range tbl.Schema().Fields() {
			if !alwaysNull[fld.Name] && tbl.Column(i).NullN() != 0 {
				t.Errorf("%s.%s is null in a fully populated row", def.Name, fld.Name)
			}
		}
		tbl.Release()
	}
}
