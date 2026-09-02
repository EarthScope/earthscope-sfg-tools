package main

import (
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
