package datamodels

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

// column returns the single chunk of the named column of tbl.
func column(t *testing.T, tbl arrow.Table, name string) arrow.Array {
	t.Helper()
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) != 1 {
		t.Fatalf("no column %q", name)
	}
	chunks := tbl.Column(idx[0]).Data().Chunks()
	if len(chunks) != 1 {
		t.Fatalf("%s has %d chunks, want 1", name, len(chunks))
	}
	return chunks[0]
}

func obsKey(t *testing.T, sys gnss.System, code string) observation.ObservationKey {
	t.Helper()
	k, err := observation.ParseObservationKey(sys, code)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestBuildGNSSObs(t *testing.T) {
	// A GPST wall clock, 18 s ahead of UTC in 2023, with a sub-microsecond tail.
	gpst := time.Date(2023, 6, 21, 17, 11, 1, 50_000_123, time.UTC)
	lock := 12.5

	gps := observation.NewSatellite(gnss.GPS, 30)
	gps.Observations = append(gps.Observations,
		observation.Observation{Code: obsKey(t, gnss.GPS, "1C"), Pseudorange: 21007753.481,
			Phase: 110396605.995, Doppler: -1631.778, SNR: 51.5, LockTime: &lock,
			Flags: observation.FlagLossOfLock},
		// Code only: phase, doppler, snr and the LLI bits are null.
		observation.Observation{Code: obsKey(t, gnss.GPS, "2W"), Pseudorange: 21007760.712},
	)
	gloUnknown := observation.NewSatellite(gnss.GLONASS, 1) // FcnUnknown
	gloUnknown.Observations = append(gloUnknown.Observations,
		observation.Observation{Code: obsKey(t, gnss.GLONASS, "1C"), Pseudorange: 19818845.789})
	gloZero := observation.NewSatellite(gnss.GLONASS, 2)
	gloZero.Fcn = 0 // a real channel, not "absent"
	gloZero.Observations = append(gloZero.Observations,
		observation.Observation{Code: obsKey(t, gnss.GLONASS, "1C"), Pseudorange: 19660946.553})

	epoch := observation.Epoch{Time: gpst, EpochPico: 456,
		Satellites: observation.Satellites{gps, gloUnknown}}
	epoch2 := observation.Epoch{Time: gpst.Add(time.Second), ReceiverClockOffset: 1e-7, EpochFlag: 1,
		Satellites: observation.Satellites{gloZero}}

	tbl, err := BuildGNSSObs("NOV770",
		GNSSObsSource{LogSource: "RANGECMP5B", Epochs: []observation.Epoch{epoch}},
		GNSSObsSource{LogSource: "RANGECMP5B_1", Epochs: []observation.Epoch{epoch2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if want, _ := ParsedSchema(GNSSObs); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want GNSSObs parsed schema %v", tbl.Schema(), want)
	}
	if n := tbl.NumRows(); n != 4 {
		t.Fatalf("%d rows, want 4: one per (epoch, satellite, signal)", n)
	}

	logSource := column(t, tbl, "Log_Source").(*array.String)
	if logSource.Value(0) != "RANGECMP5B" || logSource.Value(3) != "RANGECMP5B_1" {
		t.Errorf("Log_Source = %q, %q", logSource.Value(0), logSource.Value(3))
	}

	timeGPST := column(t, tbl, "time_gpst").(*array.Timestamp)
	timeUTC := column(t, tbl, "time_utc").(*array.Timestamp)
	subUsPs := column(t, tbl, "time_sub_us_ps").(*array.Int32)
	if got, want := int64(timeGPST.Value(0)), gpst.UnixMicro(); got != want {
		t.Errorf("time_gpst = %d, want %d", got, want)
	}
	if got, want := int64(timeUTC.Value(0)), gpst.Add(-18*time.Second).UnixMicro(); got != want {
		t.Errorf("time_utc = %d, want %d (GPST - 18 s)", got, want)
	}
	if got := subUsPs.Value(0); got != 123_456 {
		t.Errorf("time_sub_us_ps = %d, want 123456 (123 ns + 456 ps)", got)
	}

	cons := column(t, tbl, "constellation").(*array.String)
	sat := column(t, tbl, "satellite_id").(*array.Int32)
	sig := column(t, tbl, "signal").(*array.String)
	for i, want := range []struct {
		cons   string
		sat    int32
		signal string
	}{{"G", 30, "1C"}, {"G", 30, "2W"}, {"R", 1, "1C"}, {"R", 2, "1C"}} {
		if cons.Value(i) != want.cons || sat.Value(i) != want.sat || sig.Value(i) != want.signal {
			t.Errorf("row %d = %s%02d %s, want %s%02d %s", i,
				cons.Value(i), sat.Value(i), sig.Value(i), want.cons, want.sat, want.signal)
		}
	}

	phase := column(t, tbl, "phase").(*array.Float64)
	doppler := column(t, tbl, "doppler").(*array.Float64)
	snr := column(t, tbl, "snr").(*array.Float64)
	lli := column(t, tbl, "loss_of_lock").(*array.Boolean)
	half := column(t, tbl, "half_cycle_ambiguity").(*array.Boolean)
	lockTime := column(t, tbl, "lock_time").(*array.Float64)
	if phase.Value(0) != 110396605.995 || doppler.Value(0) != -1631.778 || snr.Value(0) != 51.5 ||
		lockTime.Value(0) != lock {
		t.Errorf("row 0 phase/doppler/snr/lock_time = %v %v %v %v",
			phase.Value(0), doppler.Value(0), snr.Value(0), lockTime.Value(0))
	}
	if !lli.Value(0) || half.Value(0) {
		t.Errorf("row 0 loss_of_lock/half_cycle = %v %v, want true false", lli.Value(0), half.Value(0))
	}
	for _, c := range []arrow.Array{phase, doppler, snr, lli, half, lockTime} {
		if !c.IsNull(1) {
			t.Errorf("row 1 (code only): %s not null", c.DataType())
		}
	}

	clock := column(t, tbl, "clock_offset").(*array.Float64)
	flag := column(t, tbl, "epoch_flag").(*array.Int32)
	if !clock.IsNull(0) || !flag.IsNull(0) {
		t.Error("row 0: zero clock offset and epoch flag must be null")
	}
	if clock.Value(3) != 1e-7 || flag.Value(3) != 1 {
		t.Errorf("row 3 clock_offset/epoch_flag = %v %v", clock.Value(3), flag.Value(3))
	}

	fcn := column(t, tbl, "fcn").(*array.Int32)
	if !fcn.IsNull(0) || !fcn.IsNull(2) {
		t.Error("fcn must be null for GPS and for an unknown GLONASS channel")
	}
	if fcn.IsNull(3) || fcn.Value(3) != 0 {
		t.Error("fcn 0 is a real GLONASS channel, not null")
	}
	if column(t, tbl, "write_time").NullN() != 0 {
		t.Error("write_time has nulls")
	}
}

// time_gpst is the partition source, so an unusable epoch is an error rather
// than a row in a bogus partition; parsers drop such epochs first.
func TestBuildGNSSObsRejectsBadEpochTime(t *testing.T) {
	sat := observation.NewSatellite(gnss.GPS, 1)
	sat.Observations = append(sat.Observations,
		observation.Observation{Code: obsKey(t, gnss.GPS, "1C"), Pseudorange: 2e7})
	for name, tm := range map[string]time.Time{
		"zero":        {},
		"before 1980": time.Date(1979, 1, 1, 0, 0, 0, 0, time.UTC),
		"far future":  time.Now().AddDate(10, 0, 0),
	} {
		epoch := observation.Epoch{Time: tm, Satellites: observation.Satellites{sat}}
		if tbl, err := BuildGNSSObs("NOV000",
			GNSSObsSource{LogSource: "RANGEA", Epochs: []observation.Epoch{epoch}}); err == nil {
			tbl.Release()
			t.Errorf("%s: want an error", name)
		}
	}
}
