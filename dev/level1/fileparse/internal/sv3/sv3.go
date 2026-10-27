// Package sv3 holds the Sonardyne SV3 event decoding shared by the DFOP00 and
// QCPIN file parsers. Both log the same events, "interrogation" (a ping) and
// "range" (a transponder reply), each carrying the sensor blocks (AHRS, GNSS,
// NOV_RANGE, ...) current at the event: DFOP00 one event per line, QCPIN one
// ping's interrogation and replies as one JSON object.
//
// Pairing and units follow sonardyne_tools/sv3_operations.py, except that
// Level 1 keeps the raw range and turn-around time (no trigger-delay
// correction) and both event times as logged.
package sv3

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

// TimeBlock is an SV3 time stamp. Common is UNIX seconds, UTC.
type TimeBlock struct {
	Common     json.Number `json:"common"`
	Instrument float64     `json:"instrument"`
	StartCount *int32      `json:"start_count"`
	Status     string      `json:"status"`
}

// AHRSBlock is the AHRS sensor block; fields absent from the event are nil.
type AHRSBlock struct {
	Acx  *float64  `json:"acx"`
	Acy  *float64  `json:"acy"`
	Acz  *float64  `json:"acz"`
	H    *float64  `json:"h"`
	HMag *float64  `json:"h_mag"`
	P    *float64  `json:"p"`
	R    *float64  `json:"r"`
	Time TimeBlock `json:"time"`
}

// GNSSBlock is the GNSS sensor block: a position fix with ENU std devs.
// Missing position fields are nil.
type GNSSBlock struct {
	Latitude  *float64  `json:"latitude"`  // [deg]
	Longitude *float64  `json:"longitude"` // [deg]
	HAE       *float64  `json:"hae"`       // height above ellipsoid [m]
	Q         int32     `json:"q"`         // GPS fix quality (SV3GPSQuality)
	SDX       float64   `json:"sdx"`       // std dev east [m]; -1 when not available
	SDY       float64   `json:"sdy"`       // std dev north [m]
	SDZ       float64   `json:"sdz"`       // std dev up [m]
	Time      TimeBlock `json:"time"`
}

// rangeLogBlock is the NOV_RANGE block: a NovAtel RANGEA log as logged.
type rangeLogBlock struct {
	Raw string `json:"raw"`
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

// RangeBlock is a range event's reply.
type RangeBlock struct {
	CN   string `json:"cn"`
	Diag struct {
		DBV diagValue `json:"dbv"`
		SNR diagValue `json:"snr"`
		XC  diagValue `json:"xc"`
	} `json:"diag"`
	Range float64 `json:"range"` // raw two-way range [s]; 0 when no reply
	TAT   float64 `json:"tat"`   // turn-around time [ms]
}

// Event is one SV3 event.
type Event struct {
	Event        string `json:"event"`
	EventID      int    `json:"event_id"`
	Observations struct {
		// A sensor with no data logs an error string (e.g. "ERR3") in place
		// of its block.
		AHRS     json.RawMessage `json:"AHRS"`
		GNSS     json.RawMessage `json:"GNSS"`
		NOVRange json.RawMessage `json:"NOV_RANGE"`
	} `json:"observations"`
	Range *RangeBlock `json:"range"`
	Time  TimeBlock   `json:"time"`
}

// block decodes raw into v, reporting false when the sensor logged an error
// string (or nothing) in place of the block.
func block(raw json.RawMessage, v any) (bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false, nil
	}
	return true, json.Unmarshal(raw, v)
}

// AHRS returns the event's AHRS block, or nil when the sensor logged an error.
func (e *Event) AHRS() (*AHRSBlock, error) {
	var a AHRSBlock
	if ok, err := block(e.Observations.AHRS, &a); !ok || err != nil {
		return nil, err
	}
	return &a, nil
}

// GNSS returns the event's GNSS block, or nil when the sensor logged an error.
func (e *Event) GNSS() (*GNSSBlock, error) {
	var g GNSSBlock
	if ok, err := block(e.Observations.GNSS, &g); !ok || err != nil {
		return nil, err
	}
	return &g, nil
}

// RangeLog returns the NovAtel log in the event's NOV_RANGE block, or "" when
// there is none.
func (e *Event) RangeLog() (string, error) {
	var r rangeLogBlock
	if ok, err := block(e.Observations.NOVRange, &r); !ok || err != nil {
		return "", err
	}
	return r.Raw, nil
}

// ReadJSONL reads a DFOP00 file: one event per line. It returns how many
// non-blank lines were not valid JSON.
func ReadJSONL(path string) (events []Event, bad int, err error) {
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
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			bad++
			continue
		}
		events = append(events, e)
	}
	return events, bad, sc.Err()
}

// ReadQCPIN reads a QCPIN file: one JSON object holding a ping's
// "interrogation" event and its range events, one per reply under the
// replying transponder's uid. It returns the interrogation first, then the
// ranges in key order, and how many blocks were not valid events.
func ReadQCPIN(path string) (events []Event, bad int, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var blocks map[string]json.RawMessage
	if err := json.Unmarshal(b, &blocks); err != nil {
		return nil, 0, fmt.Errorf("QCPIN file: %w", err)
	}
	var ranges []Event
	for _, key := range slices.Sorted(maps.Keys(blocks)) {
		raw := bytes.TrimSpace(blocks[key])
		if len(raw) == 0 || raw[0] != '{' {
			continue
		}
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			bad++
			continue
		}
		switch {
		case key == "interrogation":
			events = append(events, e)
		case e.Event == "range":
			ranges = append(ranges, e)
		}
	}
	return append(events, ranges...), bad, nil
}

// UnixMicros converts decimal UNIX seconds to microseconds without the
// rounding a float64 round-trip adds at this magnitude.
func UnixMicros(n json.Number) (int64, error) {
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
