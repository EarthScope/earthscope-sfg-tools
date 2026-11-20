package datamodels

import (
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// AntennaEpoch is the antenna's state at one end of a shot (ping or reply):
// its ECEF position and std devs, and the platform attitude. A field is nil
// when the event did not log it.
type AntennaEpoch struct {
	X, Y, Z              *float64 // antenna position, ECEF [m]
	SigX, SigY, SigZ     *float64 // its std devs [m]
	Heading, Pitch, Roll *float64 // [deg]
}

// CommunityStandardRow is one community_standard row: a GNSS-A shot, one
// transponder's reply paired with the ping that triggered it. Times are UTC.
type CommunityStandardRow struct {
	LogSource  edid.LogSource
	MTID       string    // transponder ID
	TravelTime float64   // raw two-way range - TAT - trigger delay [s]
	TTransmit  time.Time // ping
	TReceive   time.Time // reply
	// Transducer positions, ECEF [m]: nil until lever arms are applied
	// (always, when parsed).
	XTransmit, YTransmit, ZTransmit *float64
	XReceive, YReceive, ZReceive    *float64
	TDCID                           *string  // transducer ID
	ASNR                            *float64 // signal-to-noise ratio [dB]
	Acc                             *int32   // cross-correlation accuracy [%]
	DBV                             *float32 // signal strength [dBV]
	QualityFlag                     *string
	Transmit, Receive               AntennaEpoch // at ping (suffix 0) and reply (suffix 1)
}

// BuildCommunityStandard builds community_standard.
func BuildCommunityStandard(rs []CommunityStandardRow, scope edid.Scope) (arrow.Table, error) {
	b, err := newBuilder(CommunityStandard)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol    = col[*array.StringBuilder](b, StreamEDIDName)
		mtID       = col[*array.StringBuilder](b, "MT_ID")
		travelTime = col[*array.Float64Builder](b, "TravelTime")
		tTransmit  = col[*array.TimestampBuilder](b, "T_transmit")
		tReceive   = col[*array.TimestampBuilder](b, "T_receive")
		tdcID      = col[*array.StringBuilder](b, "TDC_ID")
		aSNR       = col[*array.Float64Builder](b, "aSNR")
		acc        = col[*array.Int32Builder](b, "acc")
		dbV        = col[*array.Float32Builder](b, "dbV")
		quality    = col[*array.StringBuilder](b, "quality_flag")
	)
	transducer := func(suffix string) [3]*array.Float64Builder {
		return [3]*array.Float64Builder{
			col[*array.Float64Builder](b, "X_"+suffix),
			col[*array.Float64Builder](b, "Y_"+suffix),
			col[*array.Float64Builder](b, "Z_"+suffix),
		}
	}
	xdTransmit, xdReceive := transducer("transmit"), transducer("receive")
	epoch := func(suffix string) [9]*array.Float64Builder {
		var out [9]*array.Float64Builder
		for i, name := range []string{"ant_X", "ant_Y", "ant_Z", "ant_sigX", "ant_sigY", "ant_sigZ", "heading", "pitch", "roll"} {
			out[i] = col[*array.Float64Builder](b, name+suffix)
		}
		return out
	}
	ep0, ep1 := epoch("0"), epoch("1")
	appendEpoch := func(cols [9]*array.Float64Builder, e AntennaEpoch) {
		for i, v := range []*float64{e.X, e.Y, e.Z, e.SigX, e.SigY, e.SigZ, e.Heading, e.Pitch, e.Roll} {
			appendOpt(cols[i], v)
		}
	}
	for _, r := range rs {
		edidCol.Append(edid.Stream(scope, r.LogSource))
		mtID.Append(r.MTID)
		travelTime.Append(r.TravelTime)
		tTransmit.Append(micros(r.TTransmit))
		tReceive.Append(micros(r.TReceive))
		for i, v := range []*float64{r.XTransmit, r.YTransmit, r.ZTransmit} {
			appendOpt(xdTransmit[i], v)
		}
		for i, v := range []*float64{r.XReceive, r.YReceive, r.ZReceive} {
			appendOpt(xdReceive[i], v)
		}
		appendOpt(tdcID, r.TDCID)
		appendOpt(aSNR, r.ASNR)
		appendOpt(acc, r.Acc)
		appendOpt(dbV, r.DBV)
		appendOpt(quality, r.QualityFlag)
		appendEpoch(ep0, r.Transmit)
		appendEpoch(ep1, r.Receive)
	}
	return b.Table()
}
