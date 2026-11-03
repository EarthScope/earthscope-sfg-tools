package ctd

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

func TestProfileTime(t *testing.T) {
	for name, want := range map[string]int{
		"CTD_NBR1_2023_fit": 2023,
		"GCC1_CTD_2021_fit": 2021,
		"NTH1_2025_CTD_fit": 2025,
		"/a/b/2019_CTD_fit": 2019,
		"CTD_2022":          2022,
	} {
		got, err := ProfileTime(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if w := time.Date(want, 1, 1, 0, 0, 0, 0, time.UTC); !got.Equal(w) {
			t.Errorf("%s: %v, want %v", name, got, w)
		}
	}
	// No year token: digits inside another token (a station or campaign
	// number) do not count.
	for _, name := range []string{"CTD_fit", "CTD_NBR12023_fit", "CTD_1126_fit"} {
		if _, err := ProfileTime(name); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func column(t *testing.T, tbl arrow.Table, name string) arrow.Array {
	t.Helper()
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) != 1 {
		t.Fatalf("no column %q", name)
	}
	return tbl.Column(idx[0]).Data().Chunk(0)
}

func TestSoundSpeed(t *testing.T) {
	// The formats seen in the *_fit files (padded fixed-point, tabs), plus a
	// comma line, a blank line, a non-numeric line, a three-field line and a
	// depth above the surface, the last three skipped.
	data := "   0.0000000000000000        1505.0899999999999     \n" +
		"  -1.0000000000000000        1505.7890623200001     \n" +
		"-2\t1505.8165\n" +
		"-3.5,1505.85\n" +
		"\n" +
		"depth speed\n" +
		"-4 1505.9 7\n" +
		"5 1505.0\n" +
		" -3130.000    1509.3728 \n"
	path := filepath.Join(t.TempDir(), "CTD_TEST_2023_fit")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	tbl, err := SoundSpeed(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	want := []struct{ depth, speed float64 }{
		{0, 1505.0899999999999}, {1, 1505.7890623200001}, {2, 1505.8165},
		{3.5, 1505.85}, {3130, 1509.3728},
	}
	if n := int(tbl.NumRows()); n != len(want) {
		t.Fatalf("%d rows, want %d", n, len(want))
	}
	depth := column(t, tbl, "depth").(*array.Float64)
	speed := column(t, tbl, "speed").(*array.Float64)
	tm := column(t, tbl, "time").(*array.Timestamp)
	src := column(t, tbl, "Log_Source").(*array.String)
	at := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC).UnixMicro()
	for i, w := range want {
		if depth.Value(i) != w.depth || speed.Value(i) != w.speed {
			t.Errorf("row %d = %v m %v m/s, want %v m %v m/s", i, depth.Value(i), speed.Value(i), w.depth, w.speed)
		}
		if int64(tm.Value(i)) != at || src.Value(i) != LogType {
			t.Errorf("row %d time/Log_Source = %v %q", i, tm.Value(i), src.Value(i))
		}
	}
	if math.Signbit(depth.Value(0)) {
		t.Error("surface depth is -0, want +0")
	}
}

func TestSoundSpeedErrors(t *testing.T) {
	dir := t.TempDir()
	noYear := filepath.Join(dir, "CTD_fit")
	if err := os.WriteFile(noYear, []byte("-1 1500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tbl, err := SoundSpeed(noYear); err == nil {
		tbl.Release()
		t.Error("no year in the name: want an error")
	}
}
