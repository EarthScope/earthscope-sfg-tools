package sfg_utils

import (
	"fmt"
	"os"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

// Level 1 Arrow batches hold only the parsed payload columns of each table in
// earthscope_sfg_tools.datamodels.iceberg.schemas.level1; the Python ingest
// layer stamps the scope and Log_Source columns and conforms the batch to the
// Iceberg schema, rejecting any column it does not know.

var utcMicros = &arrow.TimestampType{Unit: arrow.Microsecond, TimeZone: "UTC"}

var gnssObservationsSchema = arrow.NewSchema([]arrow.Field{
	{Name: "session_id", Type: arrow.BinaryTypes.String},
	{Name: "epoch", Type: utcMicros},
	{Name: "epoch_pico", Type: arrow.PrimitiveTypes.Int32},
	{Name: "clock_offset", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	{Name: "constellation", Type: arrow.BinaryTypes.String},
	{Name: "satellite", Type: arrow.PrimitiveTypes.Int32},
	{Name: "signal", Type: arrow.BinaryTypes.String},
	{Name: "code", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	{Name: "phase", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	{Name: "doppler", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	{Name: "snr", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	{Name: "loss_of_lock", Type: arrow.FixedWidthTypes.Boolean},
	{Name: "half_cycle_ambiguity", Type: arrow.FixedWidthTypes.Boolean},
	{Name: "boc_tracking", Type: arrow.FixedWidthTypes.Boolean},
	{Name: "lock_time", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	{Name: "epoch_flag", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
	{Name: "fcn", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
	{Name: "write_time", Type: utcMicros, Nullable: true},
}, nil)

var inspvaSchema = arrow.NewSchema([]arrow.Field{
	{Name: "time", Type: utcMicros},
	{Name: "lat", Type: arrow.PrimitiveTypes.Float64},
	{Name: "lon", Type: arrow.PrimitiveTypes.Float64},
	{Name: "alt", Type: arrow.PrimitiveTypes.Float64},
	{Name: "north", Type: arrow.PrimitiveTypes.Float64},
	{Name: "east", Type: arrow.PrimitiveTypes.Float64},
	{Name: "up", Type: arrow.PrimitiveTypes.Float64},
	{Name: "roll", Type: arrow.PrimitiveTypes.Float64},
	{Name: "pitch", Type: arrow.PrimitiveTypes.Float64},
	{Name: "heading", Type: arrow.PrimitiveTypes.Float64},
}, nil)

var insstdevSchema = arrow.NewSchema([]arrow.Field{
	{Name: "time", Type: utcMicros},
	{Name: "lat_sig", Type: arrow.PrimitiveTypes.Float64},
	{Name: "lon_sig", Type: arrow.PrimitiveTypes.Float64},
	{Name: "alt_sig", Type: arrow.PrimitiveTypes.Float64},
	{Name: "north_sig", Type: arrow.PrimitiveTypes.Float64},
	{Name: "east_sig", Type: arrow.PrimitiveTypes.Float64},
	{Name: "up_sig", Type: arrow.PrimitiveTypes.Float64},
	{Name: "cov_rr", Type: arrow.PrimitiveTypes.Float64},
	{Name: "cov_pp", Type: arrow.PrimitiveTypes.Float64},
	{Name: "cov_hh", Type: arrow.PrimitiveTypes.Float64},
	{Name: "ext_sol_stat", Type: arrow.BinaryTypes.String, Nullable: true},
	{Name: "time_since_update", Type: arrow.BinaryTypes.String, Nullable: true},
}, nil)

func appendNonZero(b *array.Float64Builder, v float64) {
	if v == 0 {
		b.AppendNull()
	} else {
		b.Append(v)
	}
}

func appendOptionalString(b *array.StringBuilder, v string) {
	if v == "" {
		b.AppendNull()
	} else {
		b.Append(v)
	}
}

// utcMicro converts a NovAtel header time (GPS time scale) to UTC microseconds.
func utcMicro(gps time.Time) arrow.Timestamp {
	return arrow.Timestamp(gnsstime.GPSTimeToUTC(gps).UnixMicro())
}

// GNSSObservationsBatch flattens epochs into one row per (epoch, satellite,
// signal), following the gnsstools gnssiceberg observations writer: epoch is
// kept in GPS time, and zero code/phase/doppler/snr mean "not tracked" (null).
// Unlike gnssiceberg, an FCN of 0 is written as a real GLONASS channel.
func GNSSObservationsBatch(epochs []observation.Epoch, sessionID string, writeTime time.Time) arrow.RecordBatch {
	b := array.NewRecordBuilder(memory.DefaultAllocator, gnssObservationsSchema)
	defer b.Release()

	session := b.Field(0).(*array.StringBuilder)
	epochCol := b.Field(1).(*array.TimestampBuilder)
	epochPico := b.Field(2).(*array.Int32Builder)
	clockOffset := b.Field(3).(*array.Float64Builder)
	constellation := b.Field(4).(*array.StringBuilder)
	satellite := b.Field(5).(*array.Int32Builder)
	signal := b.Field(6).(*array.StringBuilder)
	code := b.Field(7).(*array.Float64Builder)
	phase := b.Field(8).(*array.Float64Builder)
	doppler := b.Field(9).(*array.Float64Builder)
	snr := b.Field(10).(*array.Float64Builder)
	lossOfLock := b.Field(11).(*array.BooleanBuilder)
	halfCycle := b.Field(12).(*array.BooleanBuilder)
	boc := b.Field(13).(*array.BooleanBuilder)
	lockTime := b.Field(14).(*array.Float64Builder)
	epochFlag := b.Field(15).(*array.Int32Builder)
	fcn := b.Field(16).(*array.Int32Builder)
	writeTimeCol := b.Field(17).(*array.TimestampBuilder)

	wt := arrow.Timestamp(writeTime.UTC().UnixMicro())
	for _, epoch := range epochs {
		t := arrow.Timestamp(epoch.Time.UnixMicro())
		for _, sat := range epoch.Satellites {
			for _, obs := range sat.Observations {
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

				session.Append(sessionID)
				epochCol.Append(t)
				epochPico.Append(int32(epoch.EpochPico))
				appendNonZero(clockOffset, epoch.ReceiverClockOffset)
				constellation.Append(sat.System.String())
				satellite.Append(int32(sat.SatelliteID))
				signal.Append(obs.Code.String())
				appendNonZero(code, obs.Pseudorange)
				appendNonZero(phase, obs.Phase)
				appendNonZero(doppler, obs.Doppler)
				appendNonZero(snr, obs.SNR)
				lossOfLock.Append(flags.LossOfLock())
				halfCycle.Append(flags.HalfCycleAmbiguity())
				boc.Append(flags.BocTracking())
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
				if sat.System == gnss.GLONASS && sat.Fcn != observation.FcnUnknown {
					fcn.Append(int32(sat.Fcn))
				} else {
					fcn.AppendNull()
				}
				writeTimeCol.Append(wt)
			}
		}
	}
	return b.NewRecordBatch()
}

// INSPVABatch converts INSPVAA records to rows; times are converted to UTC.
func INSPVABatch(records []InspvaaRecord) arrow.RecordBatch {
	b := array.NewRecordBuilder(memory.DefaultAllocator, inspvaSchema)
	defer b.Release()
	for _, r := range records {
		b.Field(0).(*array.TimestampBuilder).Append(utcMicro(r.time))
		for i, v := range []float64{
			r.latitude, r.longitude, r.height,
			r.northVelocity, r.eastVelocity, r.upVelocity,
			r.roll, r.pitch, r.azimuth,
		} {
			b.Field(i + 1).(*array.Float64Builder).Append(v)
		}
	}
	return b.NewRecordBatch()
}

// INSSTDEVBatch converts INSSTDEVA records to rows; times are converted to UTC.
// The roll/pitch/azimuth values are the logged standard deviations [deg].
func INSSTDEVBatch(records []INSSTDEVARecord) arrow.RecordBatch {
	b := array.NewRecordBuilder(memory.DefaultAllocator, insstdevSchema)
	defer b.Release()
	for _, r := range records {
		b.Field(0).(*array.TimestampBuilder).Append(utcMicro(r.time))
		for i, v := range []float64{
			r.latitude_std, r.longitude_std, r.height_std,
			r.northVelocity_std, r.eastVelocity_std, r.upVelocity_std,
			r.roll_std, r.pitch_std, r.azimuth_std,
		} {
			b.Field(i + 1).(*array.Float64Builder).Append(v)
		}
		appendOptionalString(b.Field(10).(*array.StringBuilder), r.extSolStat)
		appendOptionalString(b.Field(11).(*array.StringBuilder), r.timeSinceUpdate)
	}
	return b.NewRecordBatch()
}

// WriteArrowIPC writes a record batch to path as an Arrow IPC file.
func WriteArrowIPC(path string, batch arrow.RecordBatch) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	w, err := ipc.NewFileWriter(f, ipc.WithSchema(batch.Schema()))
	if err != nil {
		return fmt.Errorf("opening Arrow IPC writer for %s: %w", path, err)
	}
	if err := w.Write(batch); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing Arrow IPC writer for %s: %w", path, err)
	}
	return f.Close()
}
