package datamodels

import (
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// AttitudeAHRSRow is one attitude_ahrs row (AHRS00 XSENS messages, DFOP00 AHRS
// blocks). Time is UTC.
type AttitudeAHRSRow struct {
	LogSource      edid.LogSource
	Time           time.Time
	TimeInstrument float64  // seconds since instrument power-on
	TimeStartCount *int32   // SV3 time block only
	TimeStatus     *string  // SV3 time block only
	Acx, Acy, Acz  *float64 // [m/s^2]
	H              *float64 // heading [deg], 0..360
	HMag           *float64 // magnetic heading [deg], 0..360
	P, R           *float64 // pitch, roll [deg]
}

// BuildAttitudeAHRS builds attitude_ahrs.
func BuildAttitudeAHRS(rs []AttitudeAHRSRow, scope edid.Scope) (arrow.Table, error) {
	b, err := newBuilder(AttitudeAHRS)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol      = col[*array.StringBuilder](b, StreamEDIDName)
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
		edidCol.Append(edid.Stream(scope, r.LogSource))
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
