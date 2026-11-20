// Package sv3 holds the Sonardyne SV3 event decoding shared by the DFOP00 and
// QCPIN file parsers. Both log the same events, "interrogation" (a ping) and
// "range" (a transponder reply), each carrying the sensor blocks (AHRS, GNSS,
// NOV_RANGE, ...) current at the event: DFOP00 one event per line, QCPIN one
// ping's interrogation and replies as one JSON object.
//
// Pairing and units follow sonardyne_tools/sv3_operations.py, except that
// Level 0 keeps the raw range and turn-around time (no trigger-delay
// correction) and both event times as logged.
//
// The file also builds rows from the events (travel times, shots, attitudes,
// INSPVA, antenna positions); their row types and builders are in
// datamodels.
package sv3

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/earthscope/gnsstools/core/coordinates"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel"
)

// sv3TimeBlock is an SV3 time stamp. Common is UNIX seconds, UTC.
type sv3TimeBlock struct {
	Common     json.Number `json:"common"`
	Instrument float64     `json:"instrument"`
	StartCount *int32      `json:"start_count"`
	Status     string      `json:"status"`
}

// sv3AHRSBlock is the AHRS sensor block; fields absent from the event are nil.
type sv3AHRSBlock struct {
	Acx  *float64     `json:"acx"`
	Acy  *float64     `json:"acy"`
	Acz  *float64     `json:"acz"`
	H    *float64     `json:"h"`
	HMag *float64     `json:"h_mag"`
	P    *float64     `json:"p"`
	R    *float64     `json:"r"`
	Time sv3TimeBlock `json:"time"`
}

// sv3GNSSBlock is the GNSS sensor block: a position fix with ENU std devs.
// Missing position fields are nil.
type sv3GNSSBlock struct {
	Latitude  *float64     `json:"latitude"`  // [deg]
	Longitude *float64     `json:"longitude"` // [deg]
	HAE       *float64     `json:"hae"`       // height above ellipsoid [m]
	Q         int32        `json:"q"`         // GPS fix quality (SV3GPSQuality)
	SDX       float64      `json:"sdx"`       // std dev east [m]; -1 when not available
	SDY       float64      `json:"sdy"`       // std dev north [m]
	SDZ       float64      `json:"sdz"`       // std dev up [m]
	Time      sv3TimeBlock `json:"time"`
}

// sv3INSBlock is the NOV_INS block: the NovAtel INS solution (INSPVA) current at
// the event, with numeric status codes. Velocities are local-level north
// (vx), east (vy) and up (vz); north and east were checked against the
// positions' rate of change, up only weakly.
type sv3INSBlock struct {
	GPST         float64 `json:"gpst"`          // GPS seconds since 1980-01-06
	Latitude     float64 `json:"latitude"`      // [deg]
	Longitude    float64 `json:"longitude"`     // [deg]
	HAE          float64 `json:"hae"`           // height above ellipsoid [m]
	VX           float64 `json:"vx"`            // north [m/s]
	VY           float64 `json:"vy"`            // east [m/s]
	VZ           float64 `json:"vz"`            // up [m/s]
	R            float64 `json:"r"`             // roll [deg]
	P            float64 `json:"p"`             // pitch [deg]
	H            float64 `json:"h"`             // azimuth [deg]
	SolutionType int     `json:"solution_type"` // NovAtel inertial solution status
	TimeStatus   int     `json:"time_status"`   // NovAtel GPS reference time status
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

// sv3RangeBlock is a range event's reply.
type sv3RangeBlock struct {
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
		NOVINS   json.RawMessage `json:"NOV_INS"`
		NOVRange json.RawMessage `json:"NOV_RANGE"`
	} `json:"observations"`
	Range *sv3RangeBlock `json:"range"`
	Time  sv3TimeBlock   `json:"time"`
}

// sv3Block decodes raw into v, reporting false when the sensor logged an error
// string (or nothing) in place of the block.
func sv3Block(raw json.RawMessage, v any) (bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false, nil
	}
	return true, json.Unmarshal(raw, v)
}

// AHRS returns the event's AHRS block, or nil when the sensor logged an error.
func (e *Event) AHRS() (*sv3AHRSBlock, error) {
	var a sv3AHRSBlock
	if ok, err := sv3Block(e.Observations.AHRS, &a); !ok || err != nil {
		return nil, err
	}
	return &a, nil
}

// GNSS returns the event's GNSS block, or nil when the sensor logged an error.
func (e *Event) GNSS() (*sv3GNSSBlock, error) {
	var g sv3GNSSBlock
	if ok, err := sv3Block(e.Observations.GNSS, &g); !ok || err != nil {
		return nil, err
	}
	return &g, nil
}

// INS returns the event's NOV_INS block, or nil when there is none.
func (e *Event) INS() (*sv3INSBlock, error) {
	var b sv3INSBlock
	if ok, err := sv3Block(e.Observations.NOVINS, &b); !ok || err != nil {
		return nil, err
	}
	return &b, nil
}

// RangeLog returns the NovAtel log in the event's NOV_RANGE block, or "" when
// there is none.
func (e *Event) RangeLog() (string, error) {
	var r rangeLogBlock
	if ok, err := sv3Block(e.Observations.NOVRange, &r); !ok || err != nil {
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

// sv3UnixMicros converts decimal UNIX seconds to microseconds without the
// rounding a float64 round-trip adds at this magnitude.
func sv3UnixMicros(n json.Number) (int64, error) {
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

// --- Rows built from SV3 events ---

// A pair is a range event (a transponder reply) and the most recent
// interrogation (ping) before it, with both times in UNIX microseconds.
type sv3Pair struct {
	ping, reply *Event
	sent, recv  int64
}

// sv3Pairs pairs each range event with the most recent preceding
// interrogation. A zero range (no reply detected), a reply with no ping
// before it, and unreadable times are skipped and counted in the log.
func sv3Pairs(events []Event) []sv3Pair {
	var (
		out                       []sv3Pair
		ping                      *Event
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
		sent, err1 := sv3UnixMicros(ping.Time.Common)
		recv, err2 := sv3UnixMicros(e.Time.Common)
		if err1 != nil || err2 != nil {
			badTimes++
			continue
		}
		out = append(out, sv3Pair{ping: ping, reply: e, sent: sent, recv: recv})
	}
	slog.Debug("SV3 ping/reply pairs", "pairs", len(out), "no_interrogation", noPing, "no_reply", noReply, "bad_times", badTimes)
	return out
}

// TravelTimes is one travel_times row per ping/reply pair (see sv3Pairs), with
// the raw range as logged.
func TravelTimes(events []Event) []datamodels.TWTTRow {
	var out []datamodels.TWTTRow
	for _, p := range sv3Pairs(events) {
		r := p.reply.Range
		snr, tat := float64(r.Diag.SNR), r.TAT/1000
		out = append(out, datamodels.TWTTRow{
			MTID:       r.CN,
			TTransmit:  time.UnixMicro(p.sent).UTC(),
			TReceive:   time.UnixMicro(p.recv).UTC(),
			TravelTime: r.Range,
			DBV:        float32(r.Diag.DBV),
			Acc:        int32(math.Round(float64(r.Diag.XC))),
			ASNR:       &snr,
			TAT:        &tat,
		})
	}
	return out
}

// sv3TriggerDelay is the SV3 hardware trigger delay [s], part of every logged
// range (Python TRIGGER_DELAY_SV3).
const sv3TriggerDelay = 0.13

// Shot pairing rules, as the Python shot builder's (SV3PairingRules): a
// reply more than maxRoundTrip after its ping, or whose logged time is not
// its ping's plus the range less sv3TriggerDelay (within returnTolerance), is
// not that ping's reply.
const (
	maxRoundTrip    = 15 * time.Second
	returnTolerance = time.Microsecond
)

// Shots is one community_standard row per ping/reply pair (see sv3Pairs) that
// passes the shot pairing rules. The travel time is corrected: range - TAT -
// sv3TriggerDelay. The antenna position and std devs at each end come from
// that event's GNSS block, in ECEF (as gnss_antenna_positions); a pair
// without a usable GNSS block at either end is no shot, as in the Python
// builder. The attitude comes from the NOV_INS block, and is null where it
// was not logged (2024 DFOP00 files), where the Python builder drops the
// shot. Transducer positions stay null: they need lever arms.
func Shots(events []Event) []datamodels.CommunityStandardRow {
	var (
		out                       []datamodels.CommunityStandardRow
		tooLong, mismatch, noGNSS int
	)
	for _, p := range sv3Pairs(events) {
		r := p.reply.Range
		rtt := time.Duration(p.recv-p.sent) * time.Microsecond
		if rtt < 0 || rtt > maxRoundTrip {
			tooLong++
			continue
		}
		want := time.Duration(math.Round((r.Range-sv3TriggerDelay)*1e6)) * time.Microsecond
		if d := rtt - want; d > returnTolerance || d < -returnTolerance {
			mismatch++
			continue
		}
		ping, reply := antennaEpoch(p.ping), antennaEpoch(p.reply)
		if ping.X == nil || reply.X == nil {
			noGNSS++
			continue
		}
		snr, acc, dbv := float64(r.Diag.SNR), int32(math.Round(float64(r.Diag.XC))), float32(r.Diag.DBV)
		out = append(out, datamodels.CommunityStandardRow{
			MTID:       r.CN,
			TravelTime: r.Range - r.TAT/1000 - sv3TriggerDelay,
			TTransmit:  time.UnixMicro(p.sent).UTC(),
			TReceive:   time.UnixMicro(p.recv).UTC(),
			ASNR:       &snr,
			Acc:        &acc,
			DBV:        &dbv,
			Transmit:   ping,
			Receive:    reply,
		})
	}
	slog.Debug("SV3 shots", "shots", len(out), "round_trip_too_long", tooLong, "return_time_mismatch", mismatch,
		"no_gnss_position", noGNSS)
	return out
}

// antennaEpoch is e's antenna state: the GNSS block's position and std devs
// in ECEF, and the NOV_INS attitude. Missing or bad blocks leave nils.
func antennaEpoch(e *Event) datamodels.AntennaEpoch {
	var out datamodels.AntennaEpoch
	if g, err := e.GNSS(); err == nil && g != nil {
		if pos, err := antennaPosition(g); err == nil {
			out.X, out.Y, out.Z = &pos.X, &pos.Y, &pos.Z
			out.SigX, out.SigY, out.SigZ = pos.SigX, pos.SigY, pos.SigZ
		}
	}
	if ins, err := e.INS(); err == nil && ins != nil {
		h, pt, r := ins.H, ins.P, ins.R
		out.Heading, out.Pitch, out.Roll = &h, &pt, &r
	}
	return out
}

// Attitudes is one attitude_ahrs row per distinct AHRS block (consecutive
// events usually repeat the same block).
func Attitudes(events []Event) []datamodels.AttitudeAHRSRow {
	var (
		out  []datamodels.AttitudeAHRSRow
		last json.Number
		bad  int
	)
	for i := range events {
		a, err := events[i].AHRS()
		if err != nil {
			bad++
			continue
		}
		if a == nil || a.Time.Common == last {
			continue
		}
		t, err := sv3UnixMicros(a.Time.Common)
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
		slog.Debug("SV3 bad AHRS blocks", "count", bad)
	}
	return out
}

// INSPVA is one inspva row per distinct NOV_INS block, keyed by its GPS time
// (events repeat the current block). Blocks with no time fix (time status
// UNKNOWN) are skipped.
func INSPVA(events []Event) []datamodels.INSPVARow {
	var (
		out            []datamodels.INSPVARow
		last           float64
		bad, unknownTS int
	)
	for i := range events {
		b, err := events[i].INS()
		if err != nil {
			bad++
			continue
		}
		if b == nil || b.GPST == last {
			continue
		}
		last = b.GPST
		if b.TimeStatus == novatel.TimeStatusUnknown {
			unknownTS++
			continue
		}
		ins, ts := novatel.INSStatusName(b.SolutionType), novatel.TimeStatusName(b.TimeStatus)
		out = append(out, datamodels.INSPVARow{
			Time: gnsstime.GPSEpoch().Add(time.Duration(math.Round(b.GPST*1e6)) * time.Microsecond),
			Lat:  b.Latitude, Lon: b.Longitude, Alt: b.HAE,
			North: b.VX, East: b.VY, Up: b.VZ,
			Roll: b.R, Pitch: b.P, Azimuth: b.H,
			INSStatus: &ins, TimeStatus: &ts,
		})
	}
	if bad > 0 || unknownTS > 0 {
		slog.Debug("SV3 skipped NOV_INS blocks", "bad", bad, "unknown_time", unknownTS)
	}
	return out
}

// AntennaPositions is one gnss_antenna_positions row per distinct GNSS block,
// keyed by its fix time (events repeat the current block); the first block
// seen for a time wins. Blocks without a usable position, time or fix
// quality are skipped and counted.
func AntennaPositions(events []Event) []datamodels.GNSSAntennaPositionRow {
	var (
		out  []datamodels.GNSSAntennaPositionRow
		seen = map[json.Number]bool{}
		bad  int
	)
	for i := range events {
		g, err := events[i].GNSS()
		if err != nil {
			bad++
			continue
		}
		if g == nil || seen[g.Time.Common] {
			continue
		}
		seen[g.Time.Common] = true
		r, err := antennaPosition(g)
		if err != nil {
			bad++
			continue
		}
		out = append(out, r)
	}
	if bad > 0 {
		slog.Debug("SV3 bad GNSS blocks", "count", bad)
	}
	return out
}

// antennaPosition converts g to ECEF: the WGS84 position from latitude,
// longitude and hae, and the ENU std devs rotated to ECEF std devs
// (correlations dropped). A negative std dev is Sonardyne's -1.0 "not
// available" sentinel and nulls all three sigmas.
func antennaPosition(g *sv3GNSSBlock) (datamodels.GNSSAntennaPositionRow, error) {
	if g.Latitude == nil || g.Longitude == nil || g.HAE == nil {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GNSS block without a position")
	}
	lat, lon, hae := *g.Latitude, *g.Longitude, *g.HAE
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 || math.Abs(hae) > 1000 || g.Q < 0 || g.Q > 9 {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GNSS block out of range: lat %v lon %v hae %v q %d", lat, lon, hae, g.Q)
	}
	t, err := sv3UnixMicros(g.Time.Common)
	if err != nil {
		return datamodels.GNSSAntennaPositionRow{}, err
	}
	geo := coordinates.GeodeticCoord{Latitude: lat * math.Pi / 180, Longitude: lon * math.Pi / 180, Height: hae}
	ecef := coordinates.GeodeticToECEF(geo)
	r := datamodels.GNSSAntennaPositionRow{
		Time: time.UnixMicro(t).UTC(),
		X:    ecef[0], Y: ecef[1], Z: ecef[2],
		Q: g.Q,
	}
	if g.SDX >= 0 && g.SDY >= 0 && g.SDZ >= 0 {
		sig := enuSigmasToECEF(geo, g.SDX, g.SDY, g.SDZ)
		r.SigX, r.SigY, r.SigZ = &sig[0], &sig[1], &sig[2]
	}
	return r, nil
}

// enuSigmasToECEF rotates diagonal ENU std devs at geo to ECEF std devs.
// R (ECEF to ENU) is orthonormal, so ECEF = Rᵀ·ENU and the variance of ECEF
// axis i is Σⱼ R[j][i]²·var(ENU j).
func enuSigmasToECEF(geo coordinates.GeodeticCoord, sigE, sigN, sigU float64) [3]float64 {
	rot := coordinates.ComputeECEFToENUTransform(geo) // row-major, rows E, N, U
	variances := [3]float64{sigE * sigE, sigN * sigN, sigU * sigU}
	var out [3]float64
	for i := range 3 {
		var v float64
		for j := range 3 {
			r := rot[j*3+i]
			v += r * r * variances[j]
		}
		out[i] = math.Sqrt(v)
	}
	return out
}
