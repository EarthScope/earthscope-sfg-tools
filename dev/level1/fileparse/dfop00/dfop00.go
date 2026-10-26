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
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
)

const (
	// FileType names the file type in stream EDIDs.
	FileType = "DFOP00"
	// LogType is the one log type DFOP00 files carry (SV3 JSON events), and
	// the Log_Source value of their rows.
	LogType = "DFOP00"
)

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

// TWTT parses the logType (LogType) events of the DFOP00 file at path into
// travel_times rows (see datamodels.ParsedSchema):
// each range event pairs with the most recent preceding interrogation, and a
// zero range (no reply detected) yields no row. The caller releases the
// table.
func TWTT(path, logType string) (arrow.Table, error) {
	events, err := parseEvents(path, logType)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildTWTT(travelTimes(events), LogType)
}

// AttitudeAHRS parses the logType (LogType) events of the DFOP00 file at path
// into attitude_ahrs rows (see datamodels.ParsedSchema): one row per distinct AHRS block (consecutive events usually repeat
// the same block). The caller releases the table.
func AttitudeAHRS(path, logType string) (arrow.Table, error) {
	events, err := parseEvents(path, logType)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildAttitudeAHRS(attitudes(events), LogType)
}

// parseEvents reads the events of the DFOP00 file at path, logging how many
// lines were not valid JSON. logType must be LogType.
func parseEvents(path, logType string) ([]event, error) {
	if err := logtype.Check(FileType, logType, LogType); err != nil {
		return nil, err
	}
	events, bad, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed DFOP00", "file", filepath.Base(path), "events", len(events), "bad_lines", bad)
	return events, nil
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

// travelTimes pairs each range event with the most recent preceding
// interrogation; a zero range (no reply detected) yields no row.
func travelTimes(events []event) []datamodels.TWTTRow {
	var (
		out                       []datamodels.TWTTRow
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
		snr, tat := float64(e.Range.Diag.SNR), e.Range.TAT/1000
		out = append(out, datamodels.TWTTRow{
			MTID:       e.Range.CN,
			TTransmit:  time.UnixMicro(sent).UTC(),
			TReceive:   time.UnixMicro(recv).UTC(),
			TravelTime: e.Range.Range,
			DBV:        float32(e.Range.Diag.DBV),
			Acc:        int32(math.Round(float64(e.Range.Diag.XC))),
			ASNR:       &snr,
			TAT:        &tat,
		})
	}
	slog.Debug("DFOP00 travel times", "no_interrogation", noPing, "no_reply", noReply, "bad_times", badTimes)
	return out
}

// attitudes is one attitude_ahrs row per distinct AHRS block (consecutive
// events usually repeat the same block).
func attitudes(events []event) []datamodels.AttitudeAHRSRow {
	var (
		out  []datamodels.AttitudeAHRSRow
		last json.Number
		bad  int
	)
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
		r := datamodels.AttitudeAHRSRow{
			Time:           time.UnixMicro(t).UTC(),
			TimeInstrument: a.Time.Instrument,
			TimeStartCount: a.Time.StartCount,
			Acx:            a.Acx, Acy: a.Acy, Acz: a.Acz,
			H: a.H, HMag: a.HMag, P: a.P, R: a.R,
		}
		if a.Time.Status != "" {
			r.TimeStatus = &a.Time.Status
		}
		out = append(out, r)
	}
	if bad > 0 {
		slog.Debug("DFOP00 bad AHRS blocks", "count", bad)
	}
	return out
}
