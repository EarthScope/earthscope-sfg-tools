package datamodels

import (
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

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

// BuildTWTT builds travel_times rows from logSource.
func BuildTWTT(rs []TWTTRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(TWTT)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc     = col[*array.StringBuilder](b, LogSourceColumn)
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
		logSrc.Append(logSource)
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
