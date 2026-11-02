package sfg_utils

import (
	"testing"
	"time"

	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

func TestNormalizeRangeAEpochTimeRoundsFloatingPointArtifact(t *testing.T) {
	want := time.Date(2026, 8, 19, 2, 34, 25, 600_000_000, time.UTC)
	epoch := observation.Epoch{Time: want.Add(-time.Nanosecond)}

	normalizeRangeAEpochTime(&epoch)

	if !epoch.Time.Equal(want) {
		t.Fatalf("epoch time = %s, want %s", epoch.Time, want)
	}
}
