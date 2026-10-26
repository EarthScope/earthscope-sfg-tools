package datamodels

import (
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
)

// INSPVARow is one INSPVAA log. Time is the log header time (GPS time scale).
type INSPVARow struct {
	Time                 time.Time
	Lat, Lon, Alt        float64 // [deg], [deg], [m]
	North, East, Up      float64 // [m/s]
	Roll, Pitch, Azimuth float64 // [deg]
}

// INSSTDEVRow is one INSSTDEVA log. Time is the log header time (GPS time scale).
type INSSTDEVRow struct {
	Time                          time.Time
	LatSig, LonSig, AltSig        float64 // [m]
	NorthSig, EastSig, UpSig      float64 // [m/s]
	RollSig, PitchSig, AzimuthSig float64 // [deg]
	ExtSolStat, TimeSinceUpdate   *string
}

// utcMicros is the GPS-scale time gps as a UTC microsecond timestamp.
func utcMicros(gps time.Time) arrow.Timestamp {
	return arrow.Timestamp(gnsstime.GPSTimeToUTC(gps).UnixMicro())
}

// BuildINSPVA converts records from logSource to INSPVARow rows (see
// ParsedSchema); times become UTC.
func BuildINSPVA(records []INSPVARow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(INSPVA)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	logSrc := col[*array.StringBuilder](b, LogSourceColumn)
	tm := col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat", "lon", "alt", "north", "east", "up", "roll", "pitch", "heading"} {
		cols = append(cols, col[*array.Float64Builder](b, name))
	}
	for _, r := range records {
		logSrc.Append(logSource)
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.Lat, r.Lon, r.Alt, r.North, r.East, r.Up, r.Roll, r.Pitch, r.Azimuth} {
			cols[i].Append(v)
		}
	}
	return b.Table()
}

// BuildINSSTDEV converts records from logSource to INSSTDEVRow rows (see
// ParsedSchema); times become UTC. The
// cov_* columns hold the logged attitude std devs (see INSSTDEV).
func BuildINSSTDEV(records []INSSTDEVRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(INSSTDEV)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	logSrc := col[*array.StringBuilder](b, LogSourceColumn)
	tm := col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat_sig", "lon_sig", "alt_sig", "north_sig", "east_sig", "up_sig", "cov_rr", "cov_pp", "cov_hh"} {
		cols = append(cols, col[*array.Float64Builder](b, name))
	}
	extSolStat := col[*array.StringBuilder](b, "ext_sol_stat")
	sinceUpdate := col[*array.StringBuilder](b, "time_since_update")
	for _, r := range records {
		logSrc.Append(logSource)
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.LatSig, r.LonSig, r.AltSig, r.NorthSig, r.EastSig, r.UpSig, r.RollSig, r.PitchSig, r.AzimuthSig} {
			cols[i].Append(v)
		}
		appendOpt(extSolStat, r.ExtSolStat)
		appendOpt(sinceUpdate, r.TimeSinceUpdate)
	}
	return b.Table()
}
