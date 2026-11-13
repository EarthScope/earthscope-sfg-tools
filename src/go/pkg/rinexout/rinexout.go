// Package rinexout writes GNSS observation epochs as daily RINEX observation
// files: it orders and de-duplicates epochs, optionally decimates them,
// splits them by day and names and writes one file per day. It takes epochs
// from any source, e.g. fileparse.ReadGNSSEpochs for raw files.
package rinexout

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

// Prepare orders epochs by time, keeps the first epoch of each time, flags
// cycle slips (see FlagSlips), and when moduloMillis > 0 decimates them (see
// Decimate). Epochs must come from one receiver antenna, e.g. one log source.
// It reorders epochs in place and sets flags on their observations.
func Prepare(epochs []observation.Epoch, moduloMillis int64) []observation.Epoch {
	epochs = Dedupe(epochs)
	FlagSlips(epochs)
	return Decimate(epochs, moduloMillis)
}

// Dedupe orders epochs by time and keeps the first epoch of each time (a
// stable sort, so "first" is input order). It reorders epochs in place.
func Dedupe(epochs []observation.Epoch) []observation.Epoch {
	sort.SliceStable(epochs, func(i, j int) bool { return epochs[i].Time.Before(epochs[j].Time) })
	return slices.CompactFunc(epochs, func(a, b observation.Epoch) bool { return a.Time.Equal(b.Time) })
}

// FlagSlips sets the loss-of-lock flag (RINEX LLI bit 0) where a signal's
// reported lock time shows the receiver re-locked since its previous
// observation (observation.LockTimeTracker, NovAtel conventions). The
// NovAtel decoders report lock times but set no slip flags, so without this
// no slip reaches RINEX. Epochs must be time-ordered and from one antenna;
// running over all of a day's files at once keeps arcs across file
// boundaries.
func FlagSlips(epochs []observation.Epoch) {
	var tracker observation.LockTimeTracker
	for i := range epochs {
		tracker.Apply(&epochs[i])
	}
}

// Decimate keeps the epochs whose time, rounded to the millisecond, falls on
// a multiple of moduloMillis (e.g. 1000 for 1 Hz, 15000 for 15 s); 0 or less
// keeps every epoch. Epochs must be in time order. It is
// observation.DecimateReader: a slip on a dropped epoch is carried to the
// signal's next kept observation, and receiver clock jumps are allowed for.
// It sets flags on the kept epochs' observations.
func Decimate(epochs []observation.Epoch, moduloMillis int64) []observation.Epoch {
	if moduloMillis <= 0 || len(epochs) == 0 {
		return epochs
	}
	r := observation.NewDecimateReader(&sliceReader{epochs: epochs}, time.Duration(moduloMillis)*time.Millisecond)
	var kept []observation.Epoch
	for {
		e, err := r.NextEpoch()
		if err != nil { // only io.EOF: sliceReader has no other error
			break
		}
		kept = append(kept, e)
	}
	slog.Debug("decimated epochs", "in", len(epochs), "out", len(kept), "modulo_ms", moduloMillis)
	return kept
}

// sliceReader is an observation.EpochReader over a slice.
type sliceReader struct {
	epochs []observation.Epoch
	next   int
}

func (r *sliceReader) NextEpoch() (observation.Epoch, error) {
	if r.next == len(r.epochs) {
		return observation.Epoch{}, io.EOF
	}
	r.next++
	return r.epochs[r.next-1], nil
}

// ByDay splits time-ordered epochs by the calendar day of their time, in day
// order; each day's epochs keep their order.
func ByDay(epochs []observation.Epoch) [][]observation.Epoch {
	days := map[time.Time][]observation.Epoch{}
	for _, e := range epochs {
		y, m, d := e.Time.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		days[day] = append(days[day], e)
	}
	keys := slices.SortedFunc(maps.Keys(days), time.Time.Compare)
	out := make([][]observation.Epoch, len(keys))
	for i, k := range keys {
		out[i] = days[k]
	}
	return out
}

// FileName is the RINEX 3 long name of a daily observation file of epochs
// (time-ordered, at least one): <marker>00USA_R_<yyyydddhhmm>_01D_<interval>_<system>O.rnx,
// with the interval estimated from the epochs and the system from
// settings.RinexSystem (M when unset). The start is the first epoch's, so
// files for the same day from different subsets of its raw files get
// different names.
func FileName(settings *rinex.Settings, epochs []observation.Epoch) string {
	t := epochs[0].Time
	sys := "M"
	if settings.RinexSystem != "" {
		sys = string(settings.RinexSystem[0])
	}
	return fmt.Sprintf("%s00USA_R_%04d%03d%02d%02d_01D_%s_%sO.rnx",
		settings.MarkerName, t.Year(), t.YearDay(), t.Hour(), t.Minute(), interval(epochs), sys)
}

// interval is the RINEX 3 file-name code of the epochs' median spacing (over
// the first 200 gaps): a frequency below one second (20Z for 20 Hz, 01C for
// 100 Hz), else a period (01S, 15S, 05M, 01H, 01D); 00U when unknown.
func interval(epochs []observation.Epoch) string {
	n := min(len(epochs)-1, 200)
	if n < 1 {
		return "00U"
	}
	gaps := make([]float64, n)
	for i := range n {
		gaps[i] = epochs[i+1].Time.Sub(epochs[i].Time).Seconds()
	}
	sort.Float64s(gaps)
	median := gaps[len(gaps)/2]
	switch {
	case median <= 0:
		return "00U"
	case median < 1:
		hz := int(math.Round(1 / median))
		if hz < 100 {
			return fmt.Sprintf("%02dZ", hz)
		}
		return fmt.Sprintf("%02dC", hz/100)
	case median < 60:
		return fmt.Sprintf("%02dS", int(math.Round(median)))
	case median < 3600:
		return fmt.Sprintf("%02dM", int(math.Round(median/60)))
	case median < 86400:
		return fmt.Sprintf("%02dH", int(math.Round(median/3600)))
	default:
		return fmt.Sprintf("%02dD", int(math.Round(median/86400)))
	}
}

// WriteFile writes one day's time-ordered epochs (at least one) to dir as a
// RINEX observation file named by FileName and returns its path. settings
// supplies the header. The file is written under a temporary name and
// renamed into place, so a failed write leaves no partial file and an
// existing file of that name is replaced only by a complete one.
func WriteFile(dir string, settings *rinex.Settings, epochs []observation.Epoch) (string, error) {
	if len(epochs) == 0 {
		return "", fmt.Errorf("no epochs to write")
	}
	path := filepath.Join(dir, FileName(settings, epochs))
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	fail := func(err error) (string, error) {
		f.Close()
		os.Remove(tmp)
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	w := bufio.NewWriter(f)
	ow := rinex.NewObsWriter(w, settings)
	for _, e := range epochs {
		if _, err := ow.Write(e); err != nil {
			return fail(err)
		}
	}
	if err := ow.Flush(); err != nil {
		return fail(err)
	}
	if err := w.Flush(); err != nil {
		return fail(err)
	}
	if err := f.Chmod(0o644); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}
