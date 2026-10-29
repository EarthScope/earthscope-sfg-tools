package datamodels

import (
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// GNSSSatelliteInViewRow is one gnss_satellites_in_view row: one satellite of
// a GSV group. Time is UTC.
type GNSSSatelliteInViewRow struct {
	Time       time.Time
	Talker     string
	SatsInView int32
	PRN        int32
	Elevation  *float64 // [deg]
	Azimuth    *float64 // [deg]
	SNR        *float64 // [dB-Hz]
}

// BuildGNSSSatellitesInView builds gnss_satellites_in_view rows from logSource.
func BuildGNSSSatellitesInView(rs []GNSSSatelliteInViewRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(GNSSSatellitesInView)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc     = col[*array.StringBuilder](b, LogSourceColumn)
		tm         = col[*array.TimestampBuilder](b, "time")
		talker     = col[*array.StringBuilder](b, "talker")
		satsInView = col[*array.Int32Builder](b, "sats_in_view")
		prn        = col[*array.Int32Builder](b, "prn")
		elevation  = col[*array.Float64Builder](b, "elevation")
		azimuth    = col[*array.Float64Builder](b, "azimuth")
		snr        = col[*array.Float64Builder](b, "snr")
	)
	for _, r := range rs {
		logSrc.Append(logSource)
		tm.Append(micros(r.Time))
		talker.Append(r.Talker)
		satsInView.Append(r.SatsInView)
		prn.Append(r.PRN)
		appendOpt(elevation, r.Elevation)
		appendOpt(azimuth, r.Azimuth)
		appendOpt(snr, r.SNR)
	}
	return b.Table()
}

// GNSSDOPRow is one gnss_dop row: one GSA sentence. Time is UTC.
type GNSSDOPRow struct {
	Time             time.Time
	Talker           string
	Mode             *string // M or A
	FixType          int32   // 1 none, 2 2D, 3 3D
	SatellitesUsed   string  // comma-separated IDs
	NumberUsed       int32
	PDOP, HDOP, VDOP *float64
}

// BuildGNSSDOP builds gnss_dop rows from logSource.
func BuildGNSSDOP(rs []GNSSDOPRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(GNSSDOP)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc  = col[*array.StringBuilder](b, LogSourceColumn)
		tm      = col[*array.TimestampBuilder](b, "time")
		talker  = col[*array.StringBuilder](b, "talker")
		mode    = col[*array.StringBuilder](b, "mode")
		fixType = col[*array.Int32Builder](b, "fix_type")
		used    = col[*array.StringBuilder](b, "satellites_used")
		nUsed   = col[*array.Int32Builder](b, "number_used")
		pdop    = col[*array.Float64Builder](b, "pdop")
		hdop    = col[*array.Float64Builder](b, "hdop")
		vdop    = col[*array.Float64Builder](b, "vdop")
	)
	for _, r := range rs {
		logSrc.Append(logSource)
		tm.Append(micros(r.Time))
		talker.Append(r.Talker)
		appendOpt(mode, r.Mode)
		fixType.Append(r.FixType)
		used.Append(r.SatellitesUsed)
		nUsed.Append(r.NumberUsed)
		appendOpt(pdop, r.PDOP)
		appendOpt(hdop, r.HDOP)
		appendOpt(vdop, r.VDOP)
	}
	return b.Table()
}

// GNSSCourseSpeedRow is one gnss_course_speed row: one VTG sentence. Time is
// UTC.
type GNSSCourseSpeedRow struct {
	Time                       time.Time
	Talker                     string
	CourseTrue, CourseMagnetic *float64 // [deg]
	SpeedKnots, SpeedKmh       *float64
	Mode                       *string
}

// BuildGNSSCourseSpeed builds gnss_course_speed rows from logSource.
func BuildGNSSCourseSpeed(rs []GNSSCourseSpeedRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(GNSSCourseSpeed)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc     = col[*array.StringBuilder](b, LogSourceColumn)
		tm         = col[*array.TimestampBuilder](b, "time")
		talker     = col[*array.StringBuilder](b, "talker")
		courseTrue = col[*array.Float64Builder](b, "course_true")
		courseMag  = col[*array.Float64Builder](b, "course_magnetic")
		knots      = col[*array.Float64Builder](b, "speed_knots")
		kmh        = col[*array.Float64Builder](b, "speed_kmh")
		mode       = col[*array.StringBuilder](b, "mode")
	)
	for _, r := range rs {
		logSrc.Append(logSource)
		tm.Append(micros(r.Time))
		talker.Append(r.Talker)
		appendOpt(courseTrue, r.CourseTrue)
		appendOpt(courseMag, r.CourseMagnetic)
		appendOpt(knots, r.SpeedKnots)
		appendOpt(kmh, r.SpeedKmh)
		appendOpt(mode, r.Mode)
	}
	return b.Table()
}
