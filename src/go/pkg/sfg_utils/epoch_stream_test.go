package sfg_utils

import (
	"testing"
	"time"

	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

func streamTestEpoch(at time.Time, sat observation.SatelliteID, lock float64) observation.Epoch {
	epoch := observation.NewEpoch()
	epoch.Time = at
	lockCopy := lock
	epoch.AddObservation(gnss.GPS, sat, observation.Observation{
		Code:     observation.ObservationKey{Frequency: observation.GPS_L1, SignalType: observation.C},
		Phase:    1,
		LockTime: &lockCopy,
	})
	return epoch
}

func TestContinuousEpochProcessorMergesAndTracksAcrossFlushBoundaries(t *testing.T) {
	t0 := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	var got []observation.Epoch
	processor := NewContinuousEpochProcessor(func(epoch observation.Epoch) error {
		got = append(got, epoch)
		return nil
	})

	if err := processor.Add(streamTestEpoch(t0, 1, 100)); err != nil {
		t.Fatal(err)
	}
	// A second message at the same receiver epoch must be merged, not dropped.
	if err := processor.Add(streamTestEpoch(t0, 2, 100)); err != nil {
		t.Fatal(err)
	}
	// Simulate a later source file: a 10 s span with only 1 s lock proves a reset.
	if err := processor.Add(streamTestEpoch(t0.Add(10*time.Second), 1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := processor.Flush(); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("emitted %d epochs, want 2", len(got))
	}
	if len(got[0].Satellites) != 2 {
		t.Fatalf("merged epoch has %d satellites, want 2", len(got[0].Satellites))
	}
	obs := got[1].Satellites[0].Observations[0]
	if !obs.Flags.LossOfLock() {
		t.Error("lock reset across logical source-file boundary was not flagged")
	}
}

func TestContinuousEpochProcessorRejectsOutOfOrderInput(t *testing.T) {
	t0 := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	processor := NewContinuousEpochProcessor(func(observation.Epoch) error { return nil })
	if err := processor.Add(streamTestEpoch(t0, 1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := processor.Add(streamTestEpoch(t0.Add(-time.Second), 1, 1)); err == nil {
		t.Fatal("out-of-order input accepted")
	}
}
