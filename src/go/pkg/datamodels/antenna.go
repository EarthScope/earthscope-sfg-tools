package datamodels

import (
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

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

// BuildGNSSAntennaPositions builds gnss_antenna_positions rows from logSource.
func BuildGNSSAntennaPositions(rs []GNSSAntennaPositionRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(GNSSAntennaPositions)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc = col[*array.StringBuilder](b, LogSourceColumn)
		tm     = col[*array.TimestampBuilder](b, "time")
		x      = col[*array.Float64Builder](b, "ant_x")
		y      = col[*array.Float64Builder](b, "ant_y")
		z      = col[*array.Float64Builder](b, "ant_z")
		sigX   = col[*array.Float64Builder](b, "ant_sigx")
		sigY   = col[*array.Float64Builder](b, "ant_sigy")
		sigZ   = col[*array.Float64Builder](b, "ant_sigz")
		q      = col[*array.Int32Builder](b, "q")
		nSats  = col[*array.Int32Builder](b, "number_of_satellites")
		pdop   = col[*array.Float64Builder](b, "pdop")
		wrms   = col[*array.Float64Builder](b, "wrms")
	)
	for _, r := range rs {
		logSrc.Append(logSource)
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
