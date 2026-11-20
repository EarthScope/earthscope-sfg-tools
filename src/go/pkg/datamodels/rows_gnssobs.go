package datamodels

import (
	"fmt"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// GNSSObsRow is one epoch of one log source (a file's log source, or one
// antenna of a file) to be written to gnss_observations. Its builder
// flattens it into one row per (satellite, signal).
type GNSSObsRow struct {
	LogSource edid.LogSource
	Epoch     observation.Epoch
}

// BuildGNSSObs flattens epochs into gnss_observations rows, one per (epoch,
// satellite, signal), following gnsstools' gnssiceberg observations writer:
// epoch times are GPS time (time_gpst is that wall clock, time_utc the UTC
// instant), and zero code/phase/doppler/snr mean "not tracked" (null; a null
// phase also nulls the LLI bits).
//
// time_gpst is required and the partition source, so an epoch time that
// fails gnssiceberg.ValidateEpoch (zero, before 1980, or in the future) is an
// error here: parsers drop and count such epochs before calling this.
//
// Each row's stream_edid is edid.Stream(scope, its LogSource); an error names
// that log source and the row's index.
func BuildGNSSObs(rows []GNSSObsRow, scope edid.Scope) (arrow.Table, error) {
	for i, r := range rows {
		if err := gnssiceberg.ValidateEpoch(r.Epoch.Time); err != nil {
			return nil, fmt.Errorf("%s epoch %d: %w", r.LogSource, i, err)
		}
	}
	b, err := newBuilder(GNSSObs)
	if err != nil {
		return nil, err
	}
	defer b.Release()

	var (
		edidCol       = col[*array.StringBuilder](b, StreamEDIDName)
		timeUTC       = col[*array.TimestampBuilder](b, "time_utc")
		timeGPST      = col[*array.TimestampBuilder](b, "time_gpst")
		timeSubUsPs   = col[*array.Int32Builder](b, "time_sub_us_ps")
		clockOffset   = col[*array.Float64Builder](b, "clock_offset")
		constellation = col[*array.StringBuilder](b, "constellation")
		satelliteID   = col[*array.Int32Builder](b, "satellite_id")
		signal        = col[*array.StringBuilder](b, "signal")
		code          = col[*array.Float64Builder](b, "code")
		phase         = col[*array.Float64Builder](b, "phase")
		doppler       = col[*array.Float64Builder](b, "doppler")
		snr           = col[*array.Float64Builder](b, "snr")
		lossOfLock    = col[*array.BooleanBuilder](b, "loss_of_lock")
		halfCycle     = col[*array.BooleanBuilder](b, "half_cycle_ambiguity")
		boc           = col[*array.BooleanBuilder](b, "boc_tracking")
		lockTime      = col[*array.Float64Builder](b, "lock_time")
		epochFlag     = col[*array.Int32Builder](b, "epoch_flag")
		fcn           = col[*array.Int32Builder](b, "fcn")
		writeTime     = col[*array.TimestampBuilder](b, "write_time")
	)
	wt := arrow.Timestamp(time.Now().UTC().UnixMicro())

	for _, r := range rows {
		epoch := r.Epoch
		utc := arrow.Timestamp(gnsstime.GPSTimeToUTC(epoch.Time).UnixMicro())
		gpst := arrow.Timestamp(epoch.Time.UnixMicro())
		stream := edid.Stream(scope, r.LogSource)
		subUsPs := int32(epoch.Time.Nanosecond()%1000)*1000 + int32(epoch.EpochPico)
		for _, sat := range epoch.Satellites {
			for _, obs := range sat.Observations {
				edidCol.Append(stream)
				timeUTC.Append(utc)
				timeGPST.Append(gpst)
				timeSubUsPs.Append(subUsPs)
				float64OrNull(clockOffset, epoch.ReceiverClockOffset, epoch.ReceiverClockOffset != 0)
				constellation.Append(sat.System.Char())
				satelliteID.Append(int32(sat.SatelliteID))
				signal.Append(obs.Code.String())
				float64OrNull(code, obs.Pseudorange, obs.Pseudorange != 0)
				float64OrNull(phase, obs.Phase, obs.Phase != 0)
				if obs.Phase == 0 {
					lossOfLock.AppendNull()
					halfCycle.AppendNull()
					boc.AppendNull()
				} else {
					flags := obs.Flags
					if epoch.ClockSteeringIndicator == 1 {
						flags.Set(observation.FlagClockSteering)
					}
					if epoch.ExternalClockIndicator == 1 {
						flags.Set(observation.FlagExternalClock)
					}
					if epoch.SmoothingIndicator == 1 {
						flags.Set(observation.FlagSmoothingApplied)
					}
					lossOfLock.Append(flags.LossOfLock())
					halfCycle.Append(flags.HalfCycleAmbiguity())
					boc.Append(flags.BocTracking())
				}
				float64OrNull(doppler, obs.Doppler, obs.Doppler != 0)
				float64OrNull(snr, obs.SNR, obs.SNR != 0)
				if obs.LockTime != nil {
					lockTime.Append(*obs.LockTime)
				} else {
					lockTime.AppendNull()
				}
				if epoch.EpochFlag != 0 {
					epochFlag.Append(int32(epoch.EpochFlag))
				} else {
					epochFlag.AppendNull()
				}
				if sat.System == gnss.GLONASS && observation.FcnValid(sat.Fcn) {
					fcn.Append(int32(sat.Fcn))
				} else {
					fcn.AppendNull()
				}
				writeTime.Append(wt)
			}
		}
	}
	return b.Table()
}
