package rinex

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// ErrMultipleStreams is the error of rows from more than one stream: a RINEX
// file holds one antenna's observations, so the stream must be chosen (by
// log source) before writing. Writer.Write and ToEpochs wrap it with the
// streams' EDIDs.
var ErrMultipleStreams = errors.New("more than one gnss_observations stream")

// obsColumns are the gnss_observations columns ToEpochs reads.
var obsColumns = []string{
	catalog.StreamEDIDName, "time_gpst", "time_sub_us_ps", "clock_offset", "constellation",
	"satellite_id", "signal", "code", "phase", "doppler", "snr", "loss_of_lock",
	"half_cycle_ambiguity", "boc_tracking", "lock_time", "epoch_flag", "fcn",
}

// strCol is a String or LargeString column.
type strCol interface {
	IsNull(int) bool
	Value(int) string
}

// ToEpochs is the inverse of datamodels.BuildGNSSObs: it groups the rows of a
// gnss_observations table back into epochs, in time order, and returns the
// one stream_edid they all carry ("" and no epochs for a table with no rows).
// Rows of more than one stream are an error wrapping ErrMultipleStreams that
// names the first two.
//
// An epoch is the rows of one (time_gpst, time_sub_us_ps); its Time is that
// GPS wall clock (UTC location), with the sub-microsecond part split back
// into the time's nanoseconds and EpochPico. A satellite is the rows of one
// (constellation, satellite_id), its observations in row order. Null code,
// phase, doppler and snr are zero ("not tracked"), a null clock_offset or
// epoch_flag is zero, the loss-of-lock, half-cycle and BOC flags are set
// where true, lock_time is kept when not null, and a GLONASS satellite's
// frequency channel is fcn when not null and unknown otherwise.
//
// What BuildGNSSObs does not store is not restored: epochs and satellites
// without observations, the epoch clock-steering, external-clock and
// smoothing indicators (folded into the flags it wrote), the Event, antenna
// index and SSI. The range-indicator upper bound of the lock time is
// restored for the NovAtel RANGECMP5B and RANGECMPB streams (restoreLockBounds).
func ToEpochs(t arrow.Table) (streamEDID string, epochs []observation.Epoch, err error) {
	if t.NumRows() == 0 {
		return "", nil, nil
	}
	sch := t.Schema()
	idx := make(map[string]int, len(obsColumns))
	for _, name := range obsColumns {
		i := sch.FieldIndices(name)
		if len(i) != 1 {
			return "", nil, fmt.Errorf("gnss_observations table: want one %s column, have %d", name, len(i))
		}
		idx[name] = i[0]
	}

	type epochKey struct{ gpst, sub int64 }
	type satKey struct {
		epoch int
		key   observation.SatelliteKey
	}
	byEpoch := map[epochKey]int{}
	sats := map[satKey]*observation.Satellite{}

	rdr := array.NewTableReader(t, -1)
	defer rdr.Release()
	for rdr.Next() {
		rec := rdr.RecordBatch()
		col := func(name string) arrow.Array { return rec.Column(idx[name]) }
		var (
			edidCol, constCol, sigCol strCol
			gpst                      *array.Timestamp
			sub, satID, epochFlag     *array.Int32
			fcn                       *array.Int32
			clock, code, phase        *array.Float64
			doppler, snr, lock        *array.Float64
			lol, half, boc            *array.Boolean
		)
		var ok bool
		for _, c := range []struct {
			name string
			dst  any
		}{
			{catalog.StreamEDIDName, &edidCol}, {"constellation", &constCol}, {"signal", &sigCol},
			{"time_gpst", &gpst}, {"time_sub_us_ps", &sub}, {"satellite_id", &satID},
			{"epoch_flag", &epochFlag}, {"fcn", &fcn}, {"clock_offset", &clock}, {"code", &code},
			{"phase", &phase}, {"doppler", &doppler}, {"snr", &snr}, {"lock_time", &lock},
			{"loss_of_lock", &lol}, {"half_cycle_ambiguity", &half}, {"boc_tracking", &boc},
		} {
			a := col(c.name)
			switch dst := c.dst.(type) {
			case *strCol:
				*dst, ok = a.(strCol)
			case **array.Timestamp:
				*dst, ok = a.(*array.Timestamp)
			case **array.Int32:
				*dst, ok = a.(*array.Int32)
			case **array.Float64:
				*dst, ok = a.(*array.Float64)
			case **array.Boolean:
				*dst, ok = a.(*array.Boolean)
			}
			if !ok {
				return "", nil, fmt.Errorf("gnss_observations column %s has type %s", c.name, a.DataType())
			}
		}

		for i := range int(rec.NumRows()) {
			if gpst.IsNull(i) || edidCol.IsNull(i) {
				return "", nil, fmt.Errorf("gnss_observations row %d: null %s or time_gpst", i, catalog.StreamEDIDName)
			}
			stream := edidCol.Value(i)
			switch {
			case streamEDID == "":
				streamEDID = stream
			case stream != streamEDID:
				return "", nil, fmt.Errorf("%w: %s and %s", ErrMultipleStreams, streamEDID, stream)
			}

			k := epochKey{gpst: int64(gpst.Value(i))}
			if !sub.IsNull(i) {
				k.sub = int64(sub.Value(i))
			}
			ei, seen := byEpoch[k]
			if !seen {
				e := observation.NewEpoch()
				// sub is the time's nanoseconds below a microsecond, in
				// picoseconds, plus EpochPico.
				e.Time = time.UnixMicro(k.gpst).UTC().Add(time.Duration(k.sub / 1000))
				e.EpochPico = uint16(k.sub % 1000)
				if !clock.IsNull(i) {
					e.ReceiverClockOffset = clock.Value(i)
				}
				if !epochFlag.IsNull(i) {
					e.EpochFlag = observation.EpochFlag(epochFlag.Value(i))
				}
				ei = len(epochs)
				byEpoch[k] = ei
				epochs = append(epochs, e)
			}

			sys, err := gnss.ParseSystemChar(constCol.Value(i))
			if err != nil {
				return "", nil, fmt.Errorf("gnss_observations row %d: %w", i, err)
			}
			obsKey, err := observation.ParseObservationKey(sys, sigCol.Value(i))
			if err != nil {
				return "", nil, fmt.Errorf("gnss_observations row %d: %w", i, err)
			}
			sk := satKey{ei, observation.SatelliteKey{System: sys, SatelliteID: observation.SatelliteID(satID.Value(i))}}
			sat, ok := sats[sk]
			if !ok {
				sat = observation.NewSatellite(sk.key.System, sk.key.SatelliteID)
				sats[sk] = sat
				epochs[ei].Satellites = append(epochs[ei].Satellites, sat)
			}
			if sys == gnss.GLONASS && !fcn.IsNull(i) {
				sat.Fcn = int8(fcn.Value(i))
			}

			obs := observation.Observation{
				Code:        obsKey,
				Pseudorange: valueOrZero(code, i),
				Phase:       valueOrZero(phase, i),
				Doppler:     valueOrZero(doppler, i),
				SNR:         valueOrZero(snr, i),
			}
			if !lock.IsNull(i) {
				v := lock.Value(i)
				obs.LockTime = &v
			}
			if !lol.IsNull(i) && lol.Value(i) {
				obs.Flags.Set(observation.FlagLossOfLock)
			}
			if !half.IsNull(i) && half.Value(i) {
				obs.Flags.Set(observation.FlagHalfCycleAmbiguity)
			}
			if !boc.IsNull(i) && boc.Value(i) {
				obs.Flags.Set(observation.FlagBocTracking)
			}
			sat.AddObservation(obs)
		}
	}
	if err := rdr.Err(); err != nil {
		return "", nil, err
	}
	slices.SortStableFunc(epochs, func(a, b observation.Epoch) int {
		return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.EpochPico, b.EpochPico))
	})
	restoreLockBounds(streamEDID, epochs)
	return streamEDID, epochs, nil
}

// restoreLockBounds sets Observation.LockTimeMax, which gnss_observations
// does not store, where the stream's log source reports lock time as a range
// indicator and the bound follows from the stored lock time, so that slip
// detection (observation.LockTimeTracker) gives the result it gives on the
// decoder's epochs:
//
//   - RANGECMP5B*: the lock time is the lower bound of a 4-bit bucket,
//     [0, 16 ms), [16*2^(n-1), 16*2^n) ms for n = 1..14 and [262.144 s, +Inf);
//   - RANGECMPB*: a 21-bit counter in 1/32 s that saturates at 2097151, an
//     open-ended lower bound.
//
// RANGEA sources (NOV000, NVA, QCPIN) carry exact lock times and are left
// alone. The allowlist is explicit: RANGEA is exact; RANGECMPB* and
// RANGECMP5B* are bounded; any other source, or an EDID that cannot be
// split, is left exact and logged with a warning once per stream. A lock
// time that is no bucket's lower bound is left exact.
func restoreLockBounds(streamEDID string, epochs []observation.Epoch) {
	fields, ok := edid.Split(streamEDID)
	if !ok {
		warnLockBounds(streamEDID, "stream_edid cannot be split")
		return
	}
	var bound func(lock float64) *float64
	switch name := fields[4]; {
	case name == "RANGEA":
		return
	case strings.HasPrefix(name, "RANGECMP5B"):
		bound = rangecmp5Bound
	case strings.HasPrefix(name, "RANGECMPB"):
		bound = rangecmpBound
	default:
		warnLockBounds(streamEDID, "log source not known to report exact or bounded lock times")
		return
	}
	for i := range epochs {
		for _, sat := range epochs[i].Satellites {
			for j := range sat.Observations {
				if o := &sat.Observations[j]; o.LockTime != nil {
					o.LockTimeMax = bound(*o.LockTime)
				}
			}
		}
	}
}

var warnedStreams sync.Map

// warnLockBounds logs, once per stream, that lock times are left exact.
func warnLockBounds(stream, why string) {
	if _, seen := warnedStreams.LoadOrStore(stream, true); !seen {
		slog.Warn("lock time bounds not restored; slip detection treats lock times as exact", "stream", stream, "reason", why)
	}
}

// rangecmp5Bound is the exclusive upper bound of the RANGECMP5 lock time
// bucket whose lower bound is lock, nil if lock is none's.
func rangecmp5Bound(lock float64) *float64 {
	if lock == 0 {
		return ptr(0.016)
	}
	for n := uint(1); n <= 15; n++ {
		if float64(uint32(16)<<(n-1))/1000.0 != lock {
			continue
		}
		if n == 15 {
			return ptr(math.Inf(1))
		}
		return ptr(float64(uint32(16)<<n) / 1000.0)
	}
	return nil
}

// rangecmpBound is +Inf for the saturated RANGECMP lock time counter, else nil.
func rangecmpBound(lock float64) *float64 {
	if lock == float64(2097151)/32.0 {
		return ptr(math.Inf(1))
	}
	return nil
}

func ptr(v float64) *float64 { return &v }

func valueOrZero(a *array.Float64, i int) float64 {
	if a.IsNull(i) {
		return 0
	}
	return a.Value(i)
}
