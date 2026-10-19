package novatel

import (
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/tablebuild"
)

// StreamEDID is the stream EDID of rows from logSource (the Log_Source
// value) in a file of type fileSource (e.g. NOV770):
// network:station:campaign:fileSource:logSource.
func StreamEDID(scope schemas.Scope, fileSource, logSource string) string {
	return strings.Join([]string{scope.Network, scope.Station, scope.Campaign, fileSource, logSource}, ":")
}

// GNSSObsSource is the epochs from one log source (file, or antenna of a
// file) to be written to GNSSObs.
type GNSSObsSource struct {
	LogSource string
	Epochs    []observation.Epoch
}

// GNSSObsTable flattens epochs into GNSSObs rows, one per (epoch, satellite,
// signal), following gnsstools' gnssiceberg observations writer: epoch times
// are GPS time (time_gpst is that wall clock, time_utc the UTC instant), and
// zero code/phase/doppler/snr mean "not tracked" (null; a null phase also
// nulls the LLI bits).
//
// fileSource is the type of the file the epochs were parsed from (e.g. NOV770).
func GNSSObsTable(scope schemas.Scope, fileSource string, sources ...GNSSObsSource) (arrow.Table, error) {
	b, err := tablebuild.New(schemas.GNSSObs)
	if err != nil {
		return nil, err
	}
	defer b.Release()

	var (
		streamEDID    = tablebuild.Col[*array.StringBuilder](b, "stream_edid")
		timeUTC       = tablebuild.Col[*array.TimestampBuilder](b, "time_utc")
		timeGPST      = tablebuild.Col[*array.TimestampBuilder](b, "time_gpst")
		timeSubUsPs   = tablebuild.Col[*array.Int32Builder](b, "time_sub_us_ps")
		clockOffset   = tablebuild.Col[*array.Float64Builder](b, "clock_offset")
		constellation = tablebuild.Col[*array.StringBuilder](b, "constellation")
		satelliteID   = tablebuild.Col[*array.Int32Builder](b, "satellite_id")
		signal        = tablebuild.Col[*array.StringBuilder](b, "signal")
		code          = tablebuild.Col[*array.Float64Builder](b, "code")
		phase         = tablebuild.Col[*array.Float64Builder](b, "phase")
		doppler       = tablebuild.Col[*array.Float64Builder](b, "doppler")
		snr           = tablebuild.Col[*array.Float64Builder](b, "snr")
		lossOfLock    = tablebuild.Col[*array.BooleanBuilder](b, "loss_of_lock")
		halfCycle     = tablebuild.Col[*array.BooleanBuilder](b, "half_cycle_ambiguity")
		boc           = tablebuild.Col[*array.BooleanBuilder](b, "boc_tracking")
		lockTime      = tablebuild.Col[*array.Float64Builder](b, "lock_time")
		epochFlag     = tablebuild.Col[*array.Int32Builder](b, "epoch_flag")
		fcn           = tablebuild.Col[*array.Int32Builder](b, "fcn")
		writeTime     = tablebuild.Col[*array.TimestampBuilder](b, "write_time")
	)
	wt := arrow.Timestamp(time.Now().UTC().UnixMicro())

	for _, src := range sources {
		sc := b.Scope(scope, src.LogSource)
		edid := StreamEDID(scope, fileSource, src.LogSource)
		for _, epoch := range src.Epochs {
			utc := arrow.Timestamp(gnsstime.GPSTimeToUTC(epoch.Time).UnixMicro())
			gpst := arrow.Timestamp(epoch.Time.UnixMicro())
			subUsPs := int32(epoch.Time.Nanosecond()%1000)*1000 + int32(epoch.EpochPico)
			for _, sat := range epoch.Satellites {
				for _, obs := range sat.Observations {
					sc.Append()
					streamEDID.Append(edid)
					timeUTC.Append(utc)
					timeGPST.Append(gpst)
					timeSubUsPs.Append(subUsPs)
					tablebuild.Float64(clockOffset, epoch.ReceiverClockOffset, epoch.ReceiverClockOffset != 0)
					constellation.Append(sat.System.Char())
					satelliteID.Append(int32(sat.SatelliteID))
					signal.Append(obs.Code.String())
					tablebuild.Float64(code, obs.Pseudorange, obs.Pseudorange != 0)
					tablebuild.Float64(phase, obs.Phase, obs.Phase != 0)
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
					tablebuild.Float64(doppler, obs.Doppler, obs.Doppler != 0)
					tablebuild.Float64(snr, obs.SNR, obs.SNR != 0)
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
	}
	return b.Table()
}
