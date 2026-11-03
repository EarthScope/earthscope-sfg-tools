package datamodels

import (
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// TransceiverInterrogationRow is one transceiver_interrogations row: one
// ping. Time is UTC.
type TransceiverInterrogationRow struct {
	Time           time.Time // trigger request
	Unit           *string
	Event          *int32
	Targets        string // comma-separated addresses
	TargetSettings *string
	RxWindow       *int32
	TxChannel      *string
	Delay          *float64 // [ms]
	NPL            *int32
}

// BuildTransceiverInterrogations builds transceiver_interrogations rows from
// logSource.
func BuildTransceiverInterrogations(rs []TransceiverInterrogationRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(TransceiverInterrogations)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc    = col[*array.StringBuilder](b, LogSourceColumn)
		tm        = col[*array.TimestampBuilder](b, "time")
		unit      = col[*array.StringBuilder](b, "unit")
		event     = col[*array.Int32Builder](b, "event")
		targets   = col[*array.StringBuilder](b, "targets")
		settings  = col[*array.StringBuilder](b, "target_settings")
		rxWindow  = col[*array.Int32Builder](b, "rx_window")
		txChannel = col[*array.StringBuilder](b, "tx_channel")
		delay     = col[*array.Float64Builder](b, "delay")
		npl       = col[*array.Int32Builder](b, "npl")
	)
	for _, r := range rs {
		logSrc.Append(logSource)
		tm.Append(micros(r.Time))
		appendOpt(unit, r.Unit)
		appendOpt(event, r.Event)
		targets.Append(r.Targets)
		appendOpt(settings, r.TargetSettings)
		appendOpt(rxWindow, r.RxWindow)
		appendOpt(txChannel, r.TxChannel)
		appendOpt(delay, r.Delay)
		appendOpt(npl, r.NPL)
	}
	return b.Table()
}

// TransceiverRangeRow is one transceiver_ranges row: one transponder's result
// for one ping. Times are UTC.
type TransceiverRangeRow struct {
	Time       time.Time // trigger request of the ping
	ReplyTime  time.Time // result packet
	Unit       *string
	Event      *int32
	Sequence   *int32
	MTID       string
	Detected   bool
	TravelTime *float64 // [s]; nil when not detected
	Acc        *int32   // [%]
	ASNR       *float64 // [dB]
	DBV        *float32 // [dBV]
	Telemetry  *string
	Doppler    *float64
}

// BuildTransceiverRanges builds transceiver_ranges rows from logSource.
func BuildTransceiverRanges(rs []TransceiverRangeRow, logSource string) (arrow.Table, error) {
	b, err := newBuilder(TransceiverRanges)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc     = col[*array.StringBuilder](b, LogSourceColumn)
		tm         = col[*array.TimestampBuilder](b, "time")
		replyTime  = col[*array.TimestampBuilder](b, "reply_time")
		unit       = col[*array.StringBuilder](b, "unit")
		event      = col[*array.Int32Builder](b, "event")
		sequence   = col[*array.Int32Builder](b, "sequence")
		mtID       = col[*array.StringBuilder](b, "MT_ID")
		detected   = col[*array.BooleanBuilder](b, "detected")
		travelTime = col[*array.Float64Builder](b, "TravelTime")
		acc        = col[*array.Int32Builder](b, "acc")
		aSNR       = col[*array.Float64Builder](b, "aSNR")
		dbV        = col[*array.Float32Builder](b, "dbV")
		telemetry  = col[*array.StringBuilder](b, "telemetry")
		doppler    = col[*array.Float64Builder](b, "doppler")
	)
	for _, r := range rs {
		logSrc.Append(logSource)
		tm.Append(micros(r.Time))
		replyTime.Append(micros(r.ReplyTime))
		appendOpt(unit, r.Unit)
		appendOpt(event, r.Event)
		appendOpt(sequence, r.Sequence)
		mtID.Append(r.MTID)
		detected.Append(r.Detected)
		appendOpt(travelTime, r.TravelTime)
		appendOpt(acc, r.Acc)
		appendOpt(aSNR, r.ASNR)
		appendOpt(dbV, r.DBV)
		appendOpt(telemetry, r.Telemetry)
		appendOpt(doppler, r.Doppler)
	}
	return b.Table()
}

// TransceiverSettingRow is one transceiver_settings row: one key of a
// transceiver reply. Time is UTC.
type TransceiverSettingRow struct {
	LogSource string // the reply type's log source, e.g. TCVR_CONFIG
	Time      time.Time
	Unit      *string
	Command   string // CS, PORT, US, FS, DIAG
	Key       string
	Value     string
}

// BuildTransceiverSettings builds transceiver_settings rows, each from its own
// LogSource: one file's settings replies span two log sources.
func BuildTransceiverSettings(rs []TransceiverSettingRow) (arrow.Table, error) {
	b, err := newBuilder(TransceiverSettings)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		logSrc  = col[*array.StringBuilder](b, LogSourceColumn)
		tm      = col[*array.TimestampBuilder](b, "time")
		unit    = col[*array.StringBuilder](b, "unit")
		command = col[*array.StringBuilder](b, "command")
		key     = col[*array.StringBuilder](b, "key")
		value   = col[*array.StringBuilder](b, "value")
	)
	for _, r := range rs {
		logSrc.Append(r.LogSource)
		tm.Append(micros(r.Time))
		appendOpt(unit, r.Unit)
		command.Append(r.Command)
		key.Append(r.Key)
		value.Append(r.Value)
	}
	return b.Table()
}
