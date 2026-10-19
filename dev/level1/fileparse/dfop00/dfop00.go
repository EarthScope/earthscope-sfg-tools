// Package dfop00 parses Sonardyne SV3 DFOP00 logs: one JSON event per line,
// "interrogation" (a ping) or "range" (a transponder reply), each carrying
// the sensor blocks (AHRS, GNSS, NOV_INS, ...) current at the event.
//
// Pairing and units follow sonardyne_tools/sv3_operations.py, except that
// Level 1 keeps the raw range and turn-around time (no trigger-delay
// correction) and both event times as logged.
package dfop00

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/tablebuild"
)

// LogSource is the Log_Source value for rows parsed from DFOP00 files.
const LogSource = "DFOP00"

// Tables are the tables DFOP00 files produce.
var Tables = []schemas.TableDef{schemas.TWTT, schemas.AttitudeAHRS}

// timeBlock is an SV3 time stamp. Common is UNIX seconds, UTC.
type timeBlock struct {
	Common     json.Number `json:"common"`
	Instrument float64     `json:"instrument"`
	StartCount *int32      `json:"start_count"`
	Status     string      `json:"status"`
}

// ahrsBlock is the AHRS sensor block; fields absent from the event are nil.
type ahrsBlock struct {
	Acx  *float64  `json:"acx"`
	Acy  *float64  `json:"acy"`
	Acz  *float64  `json:"acz"`
	H    *float64  `json:"h"`
	HMag *float64  `json:"h_mag"`
	P    *float64  `json:"p"`
	R    *float64  `json:"r"`
	Time timeBlock `json:"time"`
}

// diagValue is a range diagnostic, logged as a number or a one-element list.
type diagValue float64

func (d *diagValue) UnmarshalJSON(b []byte) error {
	var list []float64
	if err := json.Unmarshal(b, &list); err == nil {
		if len(list) != 1 {
			return fmt.Errorf("diagnostic list has %d values, want 1", len(list))
		}
		*d = diagValue(list[0])
		return nil
	}
	var v float64
	err := json.Unmarshal(b, &v)
	*d = diagValue(v)
	return err
}

type rangeBlock struct {
	CN   string `json:"cn"`
	Diag struct {
		DBV diagValue `json:"dbv"`
		SNR diagValue `json:"snr"`
		XC  diagValue `json:"xc"`
	} `json:"diag"`
	Range float64 `json:"range"` // raw two-way range [s]
	TAT   float64 `json:"tat"`   // turn-around time [ms]
}

type event struct {
	Event        string `json:"event"`
	EventID      int    `json:"event_id"`
	Observations struct {
		// A sensor with no data logs an error string (e.g. "ERR3") in place
		// of its block.
		AHRS json.RawMessage `json:"AHRS"`
	} `json:"observations"`
	Range *rangeBlock `json:"range"`
	Time  timeBlock   `json:"time"`
}

// ahrs returns the event's AHRS block, or nil when the sensor logged an error.
func (e *event) ahrs() (*ahrsBlock, error) {
	raw := bytes.TrimSpace(e.Observations.AHRS)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, nil
	}
	var a ahrsBlock
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// unixMicros converts decimal UNIX seconds to microseconds without the
// rounding a float64 round-trip adds at this magnitude.
func unixMicros(n json.Number) (int64, error) {
	s := n.String()
	if strings.ContainsAny(s, "eE") {
		f, err := strconv.ParseFloat(s, 64)
		return int64(math.Round(f * 1e6)), err
	}
	whole, frac, _ := strings.Cut(s, ".")
	sec, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	frac = (frac + "000000")[:6]
	us, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	if strings.HasPrefix(whole, "-") {
		us = -us
	}
	return sec*1_000_000 + us, nil
}

// ParseFile parses the DFOP00 file at path into def (one of Tables) for
// scope. The caller releases the table.
//
// TWTT: each range event pairs with the most recent preceding interrogation;
// a zero range (no reply detected) yields no row. AttitudeAHRS: one row per
// distinct AHRS block (consecutive events usually repeat the same block).
func ParseFile(path string, def schemas.TableDef, scope schemas.Scope) (arrow.Table, error) {
	var parse func([]event, schemas.Scope) (arrow.Table, error)
	switch def.Name {
	case schemas.TWTT.Name:
		parse = twttTable
	case schemas.AttitudeAHRS.Name:
		parse = attitudeTable
	default:
		return nil, tablebuild.Unsupported(LogSource, def, Tables...)
	}
	events, bad, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed DFOP00", "file", filepath.Base(path), "events", len(events), "bad_lines", bad)
	return parse(events, scope)
}

func readEvents(path string) (events []event, bad int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			bad++
			continue
		}
		events = append(events, e)
	}
	return events, bad, sc.Err()
}

func twttTable(events []event, scope schemas.Scope) (arrow.Table, error) {
	b, err := tablebuild.New(schemas.TWTT)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		sc         = b.Scope(scope, LogSource)
		mtID       = tablebuild.Col[*array.StringBuilder](b, "MT_ID")
		tTransmit  = tablebuild.Col[*array.TimestampBuilder](b, "T_transmit")
		tReceive   = tablebuild.Col[*array.TimestampBuilder](b, "T_receive")
		travelTime = tablebuild.Col[*array.Float64Builder](b, "TravelTime")
		dbV        = tablebuild.Col[*array.Float32Builder](b, "dbV")
		acc        = tablebuild.Col[*array.Int32Builder](b, "acc")
		aSNR       = tablebuild.Col[*array.Float64Builder](b, "aSNR")
		tat        = tablebuild.Col[*array.Float64Builder](b, "tat")
	)
	var (
		ping                      *event
		noPing, noReply, badTimes int
	)
	for i := range events {
		e := &events[i]
		switch {
		case e.Event == "interrogation":
			ping = e
			continue
		case e.Event != "range" || e.Range == nil:
			continue
		case ping == nil:
			noPing++
			continue
		case e.Range.Range == 0:
			noReply++
			continue
		}
		sent, err1 := unixMicros(ping.Time.Common)
		recv, err2 := unixMicros(e.Time.Common)
		if err1 != nil || err2 != nil {
			badTimes++
			continue
		}
		sc.Append()
		mtID.Append(e.Range.CN)
		tTransmit.Append(arrow.Timestamp(sent))
		tReceive.Append(arrow.Timestamp(recv))
		travelTime.Append(e.Range.Range)
		dbV.Append(float32(e.Range.Diag.DBV))
		acc.Append(int32(math.Round(float64(e.Range.Diag.XC))))
		aSNR.Append(float64(e.Range.Diag.SNR))
		tat.Append(e.Range.TAT / 1000)
	}
	slog.Debug("DFOP00 travel times", "no_interrogation", noPing, "no_reply", noReply, "bad_times", badTimes)
	return b.Table()
}

func attitudeTable(events []event, scope schemas.Scope) (arrow.Table, error) {
	b, err := tablebuild.New(schemas.AttitudeAHRS)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		sc           = b.Scope(scope, LogSource)
		tm           = tablebuild.Col[*array.TimestampBuilder](b, "time")
		tmInstrument = tablebuild.Col[*array.Float64Builder](b, "time_instrument")
		startCount   = tablebuild.Col[*array.Int32Builder](b, "time_start_count")
		status       = tablebuild.Col[*array.StringBuilder](b, "time_status")
	)
	var measurements []*array.Float64Builder
	for _, name := range []string{"acx", "acy", "acz", "h", "h_mag", "p", "r"} {
		measurements = append(measurements, tablebuild.Col[*array.Float64Builder](b, name))
	}
	var last json.Number
	bad := 0
	for i := range events {
		a, err := events[i].ahrs()
		if err != nil {
			bad++
			continue
		}
		if a == nil || a.Time.Common == last {
			continue
		}
		t, err := unixMicros(a.Time.Common)
		if err != nil {
			bad++
			continue
		}
		last = a.Time.Common
		sc.Append()
		tm.Append(arrow.Timestamp(t))
		tmInstrument.Append(a.Time.Instrument)
		if a.Time.StartCount != nil {
			startCount.Append(*a.Time.StartCount)
		} else {
			startCount.AppendNull()
		}
		if a.Time.Status != "" {
			status.Append(a.Time.Status)
		} else {
			status.AppendNull()
		}
		for j, v := range []*float64{a.Acx, a.Acy, a.Acz, a.H, a.HMag, a.P, a.R} {
			if v != nil {
				measurements[j].Append(*v)
			} else {
				measurements[j].AppendNull()
			}
		}
	}
	if bad > 0 {
		slog.Debug("DFOP00 bad AHRS blocks", "count", bad)
	}
	return b.Table()
}
