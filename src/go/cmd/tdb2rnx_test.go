package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/earthscope/gnsstools/geodata/gnsstiledb"
)

func TestFinalQueryRangeIncludesArrayMaximumOnlyForFinalSlice(t *testing.T) {
	start := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)
	day := gnsstiledb.TimeRange{Start: start, End: start.Add(74*time.Minute + 59*time.Second + 800*time.Millisecond)}
	intermediate := gnsstiledb.TimeRange{Start: start, End: start.Add(time.Hour)}
	final := gnsstiledb.TimeRange{Start: intermediate.End, End: day.End}

	if got := finalQueryRange(intermediate, day); !got.End.Equal(intermediate.End) {
		t.Fatalf("intermediate end changed: got %s, want %s", got.End, intermediate.End)
	}
	wantFinalEnd := day.End.Add(time.Millisecond)
	if got := finalQueryRange(final, day); !got.End.Equal(wantFinalEnd) {
		t.Fatalf("final end = %s, want %s", got.End, wantFinalEnd)
	}
}

// TileDB consumes settings JSON directly, without Python model validation.
func TestParseTdb2rnxSettingsNormalizesLegacyAntenna(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"antenna_model":"NOV850 NONE"}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := parseTdb2rnxSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AntennaModel != "NOV850          NONE" {
		t.Fatalf("incorrect antenna field: %q", settings.AntennaModel)
	}
}
