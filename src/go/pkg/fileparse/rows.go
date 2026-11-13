// Row builders turn parsed records into Arrow tables with a table's full
// schema (catalog.TableDef), one record type and builder per table: XRow,
// e.g. INSPVARow, and BuildX. Parsers decode their wire format into records
// and call the builder, so a table fed by several file types (attitude_ahrs
// from AHRS00 and DFOP00, gnss_observations from NOV000 and NOV770) has one
// record type and one builder. This file holds the Builder and the rows and
// builders of such shared tables; the rows and builders of a table fed by a
// single file type live in that file type's file. The exception is a record
// type produced by a shared decoder, which stays beside it (INSSTDEVRow here,
// CommunityStandardRow in sv3.go).
//
// Each builder takes the stream EDID of its rows (package edid), which the
// parser computes from the scope, its file type and the rows' log source.
//
// In a record, a nullable column is a pointer field (nil: not logged) and a
// required column is a plain value. Times are time.Time; each record type
// says which time scale. Builders append to the columns they know, by name;
// untouched nullable columns are filled with nulls.

package fileparse

import (
	"fmt"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"
)

// Builder accumulates rows for one table.
type Builder struct {
	def catalog.TableDef
	sc  *arrow.Schema
	rb  *array.RecordBuilder
}

// newBuilder returns a Builder for def's full Arrow schema, stream_edid
// included: parsers fill every column.
func newBuilder(def catalog.TableDef) (*Builder, error) {
	sc, err := def.ArrowSchema()
	if err != nil {
		return nil, err
	}
	return &Builder{def: def, sc: sc, rb: array.NewRecordBuilder(memory.DefaultAllocator, sc)}, nil
}

// Release frees the builder's buffers.
func (b *Builder) Release() { b.rb.Release() }

// col returns the builder of the named column as T, e.g.
// col[*array.Float64Builder](b, "lat"). It panics on an unknown column or a
// wrong builder type: both are programming errors in the parser.
func col[T array.Builder](b *Builder, name string) T {
	idx := b.sc.FieldIndices(name)
	if len(idx) != 1 {
		panic(fmt.Sprintf("%s: no column %q", b.def.Name, name))
	}
	col, ok := b.rb.Field(idx[0]).(T)
	if !ok {
		panic(fmt.Sprintf("%s.%s: builder is %T", b.def.Name, name, b.rb.Field(idx[0])))
	}
	return col
}

// Table finishes the rows into an Arrow table and resets the builder.
// Untouched nullable columns become all-null; an untouched required column
// or columns of different lengths are an error.
func (b *Builder) Table() (arrow.Table, error) {
	n := 0
	for i := range b.sc.NumFields() {
		n = max(n, b.rb.Field(i).Len())
	}
	for i, f := range b.sc.Fields() {
		col := b.rb.Field(i)
		switch {
		case col.Len() == n:
		case col.Len() == 0 && f.Nullable:
			col.AppendNulls(n)
		case col.Len() == 0:
			return nil, fmt.Errorf("%s: required column %q was not filled", b.def.Name, f.Name)
		default:
			return nil, fmt.Errorf("%s: column %q has %d rows, want %d", b.def.Name, f.Name, col.Len(), n)
		}
	}
	rec := b.rb.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(b.sc, []arrow.RecordBatch{rec}), nil
}

// float64OrNull appends v, or null when ok is false.
func float64OrNull(col *array.Float64Builder, v float64, ok bool) {
	if ok {
		col.Append(v)
	} else {
		col.AppendNull()
	}
}

// micros is t as an Arrow microsecond timestamp.
func micros(t time.Time) arrow.Timestamp { return arrow.Timestamp(t.UnixMicro()) }

// appendOpt appends *v to col, or null when v is nil.
func appendOpt[T any, B interface {
	Append(T)
	AppendNull()
}](col B, v *T) {
	if v == nil {
		col.AppendNull()
	} else {
		col.Append(*v)
	}
}

// TWTTRow is one travel_times row: a transponder reply paired with its ping.
// Times are UTC.
type TWTTRow struct {
	MTID       string // transponder ID
	TTransmit  time.Time
	TReceive   time.Time
	TravelTime float64  // raw two-way range as logged [s]
	DBV        float32  // signal strength [dBV]
	Acc        int32    // cross-correlation accuracy [%]
	ASNR       *float64 // signal-to-noise ratio [dB]
	TAT        *float64 // beacon turn-around time [s]
}

// BuildTWTT builds travel_times rows of stream edid.
func BuildTWTT(rs []TWTTRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.TWTT)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol    = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		mtID       = col[*array.StringBuilder](b, "MT_ID")
		tTransmit  = col[*array.TimestampBuilder](b, "T_transmit")
		tReceive   = col[*array.TimestampBuilder](b, "T_receive")
		travelTime = col[*array.Float64Builder](b, "TravelTime")
		dbV        = col[*array.Float32Builder](b, "dbV")
		acc        = col[*array.Int32Builder](b, "acc")
		aSNR       = col[*array.Float64Builder](b, "aSNR")
		tat        = col[*array.Float64Builder](b, "tat")
	)
	for _, r := range rs {
		edidCol.Append(edid)
		mtID.Append(r.MTID)
		tTransmit.Append(micros(r.TTransmit))
		tReceive.Append(micros(r.TReceive))
		travelTime.Append(r.TravelTime)
		dbV.Append(r.DBV)
		acc.Append(r.Acc)
		appendOpt(aSNR, r.ASNR)
		appendOpt(tat, r.TAT)
	}
	return b.Table()
}

// INSPVARow is one INSPVAA log. Time is the log header time (GPS time scale).
type INSPVARow struct {
	Time                 time.Time
	Lat, Lon, Alt        float64 // [deg], [deg], [m]
	North, East, Up      float64 // [m/s]
	Roll, Pitch, Azimuth float64 // [deg]
	INSStatus            *string // e.g. INS_SOLUTION_GOOD
	TimeStatus           *string // log header time status, e.g. FINESTEERING
}

// INSSTDEVRow is one INSSTDEVA log. Time is the log header time (GPS time scale).
type INSSTDEVRow struct {
	Time                          time.Time
	LatSig, LonSig, AltSig        float64 // [m]
	NorthSig, EastSig, UpSig      float64 // [m/s]
	RollSig, PitchSig, AzimuthSig float64 // [deg]
	ExtSolStat, TimeSinceUpdate   *string
	TimeStatus                    *string // log header time status, e.g. FINESTEERING
}

// utcMicros is the GPS-scale time gps as a UTC microsecond timestamp.
func utcMicros(gps time.Time) arrow.Timestamp {
	return arrow.Timestamp(gnsstime.GPSTimeToUTC(gps).UnixMicro())
}

// BuildINSPVA converts records to inspva rows of stream edid; times become
// UTC.
func BuildINSPVA(records []INSPVARow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.INSPVA)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	edidCol := col[*array.StringBuilder](b, datamodels.StreamEDIDName)
	tm := col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat", "lon", "alt", "north", "east", "up", "roll", "pitch", "heading"} {
		cols = append(cols, col[*array.Float64Builder](b, name))
	}
	insStatus := col[*array.StringBuilder](b, "ins_status")
	timeStatus := col[*array.StringBuilder](b, "time_status")
	for _, r := range records {
		edidCol.Append(edid)
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.Lat, r.Lon, r.Alt, r.North, r.East, r.Up, r.Roll, r.Pitch, r.Azimuth} {
			cols[i].Append(v)
		}
		appendOpt(insStatus, r.INSStatus)
		appendOpt(timeStatus, r.TimeStatus)
	}
	return b.Table()
}

// GNSSObsSource is the epochs from one stream (a file's log source, or one
// antenna of a file) to be written to GNSSObs.
type GNSSObsSource struct {
	EDID   string // the stream's EDID (see StreamEDID)
	Epochs []observation.Epoch
}

// BuildGNSSObs flattens epochs into GNSSObs rows, one per (epoch, satellite,
// signal), following gnsstools' gnssiceberg observations writer: epoch times
// are GPS time (time_gpst is that wall clock, time_utc the UTC instant), and
// zero code/phase/doppler/snr mean "not tracked" (null; a null phase also
// nulls the LLI bits).
//
// time_gpst is required and the partition source, so an epoch time that
// fails gnssiceberg.ValidateEpoch (zero, before 1980, or in the future) is an
// error here: parsers drop and count such epochs before calling this.
//
// Each row's stream_edid is its source's EDID.
//
// fileSource is the type of the file the epochs were parsed from (e.g.
// NOV770), for errors.
func BuildGNSSObs(fileSource string, sources ...GNSSObsSource) (arrow.Table, error) {
	for _, src := range sources {
		for i, epoch := range src.Epochs {
			if err := gnssiceberg.ValidateEpoch(epoch.Time); err != nil {
				return nil, fmt.Errorf("%s %s epoch %d: %w", fileSource, src.EDID, i, err)
			}
		}
	}
	b, err := newBuilder(datamodels.GNSSObs)
	if err != nil {
		return nil, err
	}
	defer b.Release()

	var (
		edidCol       = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
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

	for _, src := range sources {
		for _, epoch := range src.Epochs {
			utc := arrow.Timestamp(gnsstime.GPSTimeToUTC(epoch.Time).UnixMicro())
			gpst := arrow.Timestamp(epoch.Time.UnixMicro())
			subUsPs := int32(epoch.Time.Nanosecond()%1000)*1000 + int32(epoch.EpochPico)
			for _, sat := range epoch.Satellites {
				for _, obs := range sat.Observations {
					edidCol.Append(src.EDID)
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
	}
	return b.Table()
}

// AttitudeAHRSRow is one attitude_ahrs row (AHRS00 XSENS messages, DFOP00 AHRS
// blocks). Time is UTC.
type AttitudeAHRSRow struct {
	Time           time.Time
	TimeInstrument float64  // seconds since instrument power-on
	TimeStartCount *int32   // SV3 time block only
	TimeStatus     *string  // SV3 time block only
	Acx, Acy, Acz  *float64 // [m/s^2]
	H              *float64 // heading [deg], 0..360
	HMag           *float64 // magnetic heading [deg], 0..360
	P, R           *float64 // pitch, roll [deg]
}

// BuildAttitudeAHRS builds attitude_ahrs rows of stream edid.
func BuildAttitudeAHRS(rs []AttitudeAHRSRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.AttitudeAHRS)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol      = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		tm           = col[*array.TimestampBuilder](b, "time")
		tmInstrument = col[*array.Float64Builder](b, "time_instrument")
		startCount   = col[*array.Int32Builder](b, "time_start_count")
		status       = col[*array.StringBuilder](b, "time_status")
	)
	var measurements []*array.Float64Builder
	for _, name := range []string{"acx", "acy", "acz", "h", "h_mag", "p", "r"} {
		measurements = append(measurements, col[*array.Float64Builder](b, name))
	}
	for _, r := range rs {
		edidCol.Append(edid)
		tm.Append(micros(r.Time))
		tmInstrument.Append(r.TimeInstrument)
		appendOpt(startCount, r.TimeStartCount)
		appendOpt(status, r.TimeStatus)
		for i, v := range []*float64{r.Acx, r.Acy, r.Acz, r.H, r.HMag, r.P, r.R} {
			appendOpt(measurements[i], v)
		}
	}
	return b.Table()
}

// GNSSAntennaPositionRow is one gnss_antenna_positions row: an antenna fix in
// ECEF. Time is UTC.
type GNSSAntennaPositionRow struct {
	Time               time.Time
	X, Y, Z            float64  // ECEF [m]
	SigX, SigY, SigZ   *float64 // ECEF std devs [m]; nil when not reported
	Q                  int32    // GPS fix quality (SV3GPSQuality)
	NumberOfSatellites *int32
	PDOP, WRMS         *float64
}

// BuildGNSSAntennaPositions builds gnss_antenna_positions rows of stream edid.
func BuildGNSSAntennaPositions(rs []GNSSAntennaPositionRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.GNSSAntennaPositions)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		tm      = col[*array.TimestampBuilder](b, "time")
		x       = col[*array.Float64Builder](b, "ant_x")
		y       = col[*array.Float64Builder](b, "ant_y")
		z       = col[*array.Float64Builder](b, "ant_z")
		sigX    = col[*array.Float64Builder](b, "ant_sigx")
		sigY    = col[*array.Float64Builder](b, "ant_sigy")
		sigZ    = col[*array.Float64Builder](b, "ant_sigz")
		q       = col[*array.Int32Builder](b, "q")
		nSats   = col[*array.Int32Builder](b, "number_of_satellites")
		pdop    = col[*array.Float64Builder](b, "pdop")
		wrms    = col[*array.Float64Builder](b, "wrms")
	)
	for _, r := range rs {
		edidCol.Append(edid)
		tm.Append(micros(r.Time))
		x.Append(r.X)
		y.Append(r.Y)
		z.Append(r.Z)
		appendOpt(sigX, r.SigX)
		appendOpt(sigY, r.SigY)
		appendOpt(sigZ, r.SigZ)
		q.Append(r.Q)
		appendOpt(nSats, r.NumberOfSatellites)
		appendOpt(pdop, r.PDOP)
		appendOpt(wrms, r.WRMS)
	}
	return b.Table()
}

// SoundSpeedRow is one sound_speed row: one depth of a profile. Time is UTC.
type SoundSpeedRow struct {
	Time  time.Time // profile time, shared by every row of the profile
	Depth float64   // positive down [m]
	Speed float64   // [m/s]
}

// BuildSoundSpeed builds sound_speed rows of stream edid.
func BuildSoundSpeed(rs []SoundSpeedRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.SoundSpeed)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		tm      = col[*array.TimestampBuilder](b, "time")
		depth   = col[*array.Float64Builder](b, "depth")
		speed   = col[*array.Float64Builder](b, "speed")
	)
	for _, r := range rs {
		edidCol.Append(edid)
		tm.Append(micros(r.Time))
		depth.Append(r.Depth)
		speed.Append(r.Speed)
	}
	return b.Table()
}

// optString is v, or nil when v is empty.
func optString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
