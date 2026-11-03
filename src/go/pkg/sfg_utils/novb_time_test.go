package sfg_utils

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	novatelbinary "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_binary"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

func timeTestRangeFrame(week uint16, ms uint32, status uint8) []byte {
	b := make([]byte, 28+48+4)
	copy(b, []byte{0xaa, 0x44, 0x12, 28})
	binary.LittleEndian.PutUint16(b[4:], 43)
	binary.LittleEndian.PutUint16(b[8:], 48)
	b[13] = status
	binary.LittleEndian.PutUint16(b[14:], week)
	binary.LittleEndian.PutUint32(b[16:], ms)
	binary.LittleEndian.PutUint32(b[28:], 1)
	binary.LittleEndian.PutUint16(b[32:], 3)
	binary.LittleEndian.PutUint64(b[36:], math.Float64bits(20000000))
	binary.LittleEndian.PutUint64(b[48:], math.Float64bits(100000000))
	binary.LittleEndian.PutUint32(b[68:], math.Float32bits(10))
	binary.LittleEndian.PutUint32(b[72:], 0xc00)
	binary.LittleEndian.PutUint32(b[76:], novatelbinary.CalculateBlockCRC32(76, b[:76]))
	return b
}

func TestNOVBTimeValidity(t *testing.T) {
	for _, status := range []uint8{0, 20, 60, 80, 200, 255} {
		if usableNOVBObservationTime(novatelbinary.Message{Week: 2434, TimeStatus: status}) {
			t.Fatalf("accepted status %d", status)
		}
	}
	for _, status := range []uint8{100, 120, 130, 140, 160, 170, 180} {
		msg := novatelbinary.Message{Week: 2434, TimeStatus: status, GPSMilliSecondsOfWeek: 85041000}
		if !usableNOVBObservationTime(msg) {
			t.Fatalf("rejected valid status %d", status)
		}
		msg.Week = 0
		if usableNOVBObservationTime(msg) {
			t.Fatal("accepted unset GPS week")
		}
		msg.Week = 2434
		msg.GPSMilliSecondsOfWeek = 604800000
		if usableNOVBObservationTime(msg) {
			t.Fatal("accepted invalid seconds of week")
		}
	}
}

func TestNOVBStartupCannotReorderFilesOrEnterEpochStream(t *testing.T) {
	dir := t.TempDir()
	early := filepath.Join(dir, "early.raw")
	late := filepath.Join(dir, "late.raw")
	// Real campaign pattern: later restart's 2.4 seconds sorts before the
	// earlier startup's 3 seconds if week-zero UNKNOWN frames are accepted.
	os.WriteFile(early, append(timeTestRangeFrame(0, 3000, 20), timeTestRangeFrame(2434, 85041000, 120)...), 0600)
	os.WriteFile(late, append(timeTestRangeFrame(0, 2400, 20), timeTestRangeFrame(2434, 339501600, 180)...), 0600)
	files, err := SortFilesByFirstEpochNOVB([]string{late, early})
	if err != nil || len(files) != 2 {
		t.Fatalf("sort: %v %v", files, err)
	}
	if files[0].Filename != early {
		t.Fatal("startup time corrupted file ordering")
	}
	var epochs []observation.Epoch
	processor := NewContinuousEpochProcessor(func(epoch observation.Epoch) error { epochs = append(epochs, epoch); return nil })
	for _, file := range files {
		failures, err := StreamFileNOVB(file.Filename, 0, processor.Add)
		if err != nil {
			t.Fatal(err)
		}
		if failures < 1 {
			t.Fatal("invalid time not included in rejected-frame count")
		}
	}
	if err := processor.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(epochs) != 2 {
		t.Fatalf("got %d epochs, want two established-time epochs", len(epochs))
	}
	if epochs[0].Time.Year() != 2026 || !epochs[0].Time.Before(epochs[1].Time) {
		t.Fatal("unexpected epoch times")
	}
}

func TestNOVBRejectsFileWithoutEstablishedTime(t *testing.T) {
	file := filepath.Join(t.TempDir(), "startup.raw")
	os.WriteFile(file, timeTestRangeFrame(0, 3000, 20), 0600)
	if _, err := GetFirstEpochTimeNOVB(file); err == nil {
		t.Fatal("accepted file containing only startup time")
	}
}

func TestNOVBRealCampaignStartup(t *testing.T) {
	dir := os.Getenv("SFG_NOVB_TEST_DIR")
	if dir == "" {
		t.Skip("set SFG_NOVB_TEST_DIR for read-only raw campaign validation")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*NOV770.raw"))
	if err != nil || len(paths) == 0 {
		t.Fatal("no raw test files")
	}
	files, err := SortFilesByFirstEpochNOVB(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(paths) {
		t.Fatalf("sorting omitted files: %d of %d", len(files), len(paths))
	}
	for i, file := range files {
		if file.Time.Year() != 2026 {
			t.Fatalf("invalid first time: %+v", file)
		}
		if i > 0 && file.Time.Before(files[i-1].Time) {
			t.Fatal("nonmonotonic sort")
		}
	}
	for _, name := range []string{"341517_001_20260830_233644_00078_NOV770.raw", "341517_001_20260902_221719_00082_NOV770.raw"} {
		count := 0
		var last time.Time
		failures, err := StreamFileNOVB(filepath.Join(dir, name), 0, func(epoch observation.Epoch) error {
			if !last.IsZero() && epoch.Time.Before(last) {
				t.Fatalf("out of order: %s follows %s", epoch.Time, last)
			}
			last = epoch.Time
			count++
			return nil
		})
		if err != nil || count == 0 {
			t.Fatalf("%s: count=%d err=%v", name, count, err)
		}
		t.Logf("%s: %d usable epochs; %d rejected records", name, count, failures)
	}
}
