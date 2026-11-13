// tcvr00: parses Sonardyne TCVR00.bin logs: the SV3's text dialogue
// with its acoustic transceiver, one message per GPSA packet, each packet
// stamped with the SV3's UTC time. Requests from the SV3 start with "<" and
// the transceiver's replies with ">", e.g.
//
//	<IR:,IR5209,IR5210                          interrogate these transponders
//	>IR:2010,EVT0,RXW3600,IR5209;RG0;W3600;LG26,...
//	<IT:CI4805;D130.000000                      transmit channel and delay [ms]
//	>IT:2010,EVT0,CI4805;D130;NPL196
//	<TRG,BR                                     trigger the ping
//	>BR:2010,EVT0,SQ0,IR5209;R3464032[XC93,SNR25,DBV-20,TEL;NONE,DOP0.00],...
//	>CS:2010,TAT640,BLK100,RXW3600,...          settings
//	>US:2010,SN341391-001,TDR3,TYPE8290         unit identity
//
// Its log sources:
//   - INTERROGATE: one transceiver_interrogations row per trigger.
//   - RANGE_RESULT: one transceiver_ranges row per transponder of each BR
//     result. R is the two-way range in microseconds, the range DFOP00 logs
//     for the same reply; R0 means no reply was detected.
//   - TCVR_CONFIG (CS, PORT replies) and TCVR_INFO (US, FS, DIAG replies):
//     transceiver_settings rows, one per key; parseTCVR00Settings returns both.
//
// The transceiver's own unit ID (e.g. 2010) leads every reply.

package fileparse

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

const (
	// FileTypeTCVR00 names the file type in stream EDIDs.
	FileTypeTCVR00 = "TCVR00"
	// LogInterrogate is the log source of transceiver_interrogations rows.
	LogInterrogate = "INTERROGATE"
	// LogRangeResult is the log source of transceiver_ranges rows.
	LogRangeResult = "RANGE_RESULT"
	// LogConfig is the log source of the CS and PORT transceiver_settings rows.
	LogConfig = "TCVR_CONFIG"
	// LogInfo is the log source of the US, FS and DIAG transceiver_settings rows.
	LogInfo = "TCVR_INFO"
)

// settingsLogSource is the log source of each settings reply type.
var settingsLogSource = map[string]string{
	"CS": LogConfig, "PORT": LogConfig,
	"US": LogInfo, "FS": LogInfo, "DIAG": LogInfo,
}

// tcvr00Message is one request or reply and the time of the packet carrying it.
type tcvr00Message struct {
	at      time.Time // packet common time, UTC
	reply   bool      // ">" (transceiver) rather than "<" (SV3)
	command string    // e.g. IR, BR, CS
	body    string    // after the command and its ":" separator
}

// parseTCVR00Interrogations parses the triggers of the TCVR00.bin file at path into
// transceiver_interrogations rows (stamped with stream_edid for scope), each with
// the targets and settings of the IR and IT exchange before it. Triggers
// with no known targets are skipped and counted. The caller releases the
// table.
func parseTCVR00Interrogations(path string, scope edid.Scope) (arrow.Table, error) {
	msgs, corrupt, err := readTCVR00File(path)
	if err != nil {
		return nil, err
	}
	rs, noTargets := interrogations(msgs)
	slog.Debug("parsed TCVR00 interrogations", "file", filepath.Base(path), "rows", len(rs),
		"no_targets", noTargets, "corrupt_packets", corrupt)
	return BuildTransceiverInterrogations(rs, edid.Stream(scope, FileTypeTCVR00, LogInterrogate))
}

// parseTCVR00Ranges parses the BR results of the TCVR00.bin file at path into
// transceiver_ranges rows (stamped with stream_edid for scope), one per transponder,
// timed by the trigger they answer. Results with no trigger before them, and
// targets that do not parse, are skipped and counted. The caller releases the
// table.
func parseTCVR00Ranges(path string, scope edid.Scope) (arrow.Table, error) {
	msgs, corrupt, err := readTCVR00File(path)
	if err != nil {
		return nil, err
	}
	rs, noTrigger, bad := ranges(msgs)
	slog.Debug("parsed TCVR00 ranges", "file", filepath.Base(path), "rows", len(rs),
		"no_trigger", noTrigger, "bad_targets", bad, "corrupt_packets", corrupt)
	return BuildTransceiverRanges(rs, edid.Stream(scope, FileTypeTCVR00, LogRangeResult))
}

// parseTCVR00Settings parses the settings replies of the TCVR00.bin file at path into
// transceiver_settings rows (stamped with stream_edid for scope), one per key: the
// CS and PORT replies with log source LogConfig, the US, FS and DIAG replies
// with LogInfo. The caller releases the table.
func parseTCVR00Settings(path string, scope edid.Scope) (arrow.Table, error) {
	msgs, corrupt, err := readTCVR00File(path)
	if err != nil {
		return nil, err
	}
	rs := settings(msgs, scope)
	slog.Debug("parsed TCVR00 settings", "file", filepath.Base(path), "rows", len(rs), "corrupt_packets", corrupt)
	return BuildTransceiverSettings(rs)
}

// readTCVR00File reads the messages of the TCVR00.bin file at path.
func readTCVR00File(path string) ([]tcvr00Message, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	return readMessages(f)
}

// readMessages reads the messages of a GPSA stream; corrupt counts packets
// dropped by the GPSA reader. Lines that are neither requests nor replies
// (e.g. empty packets) are skipped.
func readMessages(r io.Reader) (out []tcvr00Message, corrupt int, err error) {
	g := newGPSAReader(r)
	for {
		p, err := g.Next()
		if errors.Is(err, io.EOF) {
			return out, g.Corrupt, nil
		}
		if err != nil {
			return nil, 0, err
		}
		at := time.UnixMicro(int64(p.CommonTime)).UTC()
		sc := bufio.NewScanner(strings.NewReader(string(p.Data)))
		for sc.Scan() {
			if m, ok := parseMessage(strings.TrimSpace(strings.Trim(sc.Text(), "\x00"))); ok {
				m.at = at
				out = append(out, m)
			}
		}
	}
}

// parseMessage splits "<CMD:body", "<CMD,body" or ">CMD:body" into its parts.
func parseMessage(s string) (tcvr00Message, bool) {
	if len(s) < 2 || (s[0] != '<' && s[0] != '>') {
		return tcvr00Message{}, false
	}
	rest := s[1:]
	n := 0
	for n < len(rest) && rest[n] >= 'A' && rest[n] <= 'Z' {
		n++
	}
	if n == 0 {
		return tcvr00Message{}, false
	}
	body := strings.TrimPrefix(strings.TrimPrefix(rest[n:], ":"), ",")
	return tcvr00Message{reply: s[0] == '>', command: rest[:n], body: body}, true
}

// splitTop splits s on commas outside square brackets.
func splitTop(s string) []string {
	var (
		out   []string
		depth int
		start int
	)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// unitAndItems splits a reply body into the transceiver unit ID (its first,
// all-digit item) and the remaining items.
func unitAndItems(body string) (*string, []string) {
	items := splitTop(body)
	if len(items) > 0 && items[0] != "" && strings.Trim(items[0], "0123456789") == "" {
		u := items[0]
		return &u, items[1:]
	}
	return nil, items
}

// keyValue splits an item into its leading upper-case key and the rest, e.g.
// TAT640 -> TAT, 640 and TEL;NONE -> TEL, NONE. An item with a ";" whose
// first part is more than a key (P0;BR9600;CF0) keys on that first part.
func keyValue(item string) (string, string) {
	if k, v, ok := strings.Cut(item, ";"); ok {
		return k, v
	}
	return leadingKey(item)
}

// leadingKey splits s into its leading upper-case letters and the rest, e.g.
// D130 -> D, 130.
func leadingKey(s string) (string, string) {
	n := 0
	for n < len(s) && s[n] >= 'A' && s[n] <= 'Z' {
		n++
	}
	return s[:n], s[n:]
}

func optInt32(v string) *int32 {
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return nil
	}
	x := int32(n)
	return &x
}

func tryFloat(v string) *float64 {
	x, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &x
}

// interrogations is one row per trigger (<TRG), carrying the targets of the
// last <IR request and the settings of the last >IR and >IT replies, all
// cleared by the next <IR. noTargets counts triggers with no known targets.
func interrogations(msgs []tcvr00Message) (rs []TransceiverInterrogationRow, noTargets int) {
	var (
		cur      TransceiverInterrogationRow
		targets  []string
		settings []string
	)
	for _, m := range msgs {
		switch {
		case !m.reply && m.command == "IR":
			cur, targets, settings = TransceiverInterrogationRow{}, nil, nil
			for _, t := range strings.Split(m.body, ",") {
				if t != "" {
					targets = append(targets, t)
				}
			}
		case m.reply && m.command == "IR":
			unit, items := unitAndItems(m.body)
			cur.Unit = unit
			for _, it := range items {
				switch k, v := keyValue(it); {
				case strings.Contains(it, ";"):
					settings = append(settings, it)
				case k == "EVT":
					cur.Event = optInt32(v)
				case k == "RXW":
					cur.RxWindow = optInt32(v)
				}
			}
		case m.reply && m.command == "IT":
			unit, items := unitAndItems(m.body)
			if cur.Unit == nil {
				cur.Unit = unit
			}
			// e.g. EVT0, CI4805;D130;NPL196: the channel's settings follow it
			// after semicolons.
			for _, it := range items {
				for i, part := range strings.Split(it, ";") {
					k, v := leadingKey(part)
					switch {
					case k == "CI" && i == 0:
						cur.TxChannel = optString(part)
					case k == "D":
						cur.Delay = tryFloat(v)
					case k == "NPL":
						cur.NPL = optInt32(v)
					case k == "EVT" && cur.Event == nil:
						cur.Event = optInt32(v)
					}
				}
			}
		case !m.reply && m.command == "TRG":
			if len(targets) == 0 {
				noTargets++
				continue
			}
			r := cur
			r.Time = m.at
			r.Targets = strings.Join(targets, ",")
			if len(settings) > 0 {
				r.TargetSettings = optString(strings.Join(settings, ","))
			}
			rs = append(rs, r)
		}
	}
	return rs, noTargets
}

// ranges is one row per transponder of each >BR result, timed by the <TRG
// before it. noTrigger counts results with no trigger since the previous
// result; bad counts target entries that do not parse.
func ranges(msgs []tcvr00Message) (rs []TransceiverRangeRow, noTrigger, bad int) {
	var trigger *time.Time
	for _, m := range msgs {
		switch {
		case !m.reply && m.command == "TRG":
			t := m.at
			trigger = &t
		case m.reply && m.command == "BR":
			if trigger == nil {
				noTrigger++
				continue
			}
			unit, items := unitAndItems(m.body)
			var event, seq *int32
			for _, it := range items {
				k, v := keyValue(it)
				switch {
				case k == "EVT" && !strings.Contains(it, ";"):
					event = optInt32(v)
				case k == "SQ" && !strings.Contains(it, ";"):
					seq = optInt32(v)
				case strings.Contains(it, ";"):
					r, ok := rangeTarget(it)
					if !ok {
						bad++
						continue
					}
					r.Time, r.ReplyTime = *trigger, m.at
					r.Unit, r.Event, r.Sequence = unit, event, seq
					rs = append(rs, r)
				}
			}
			trigger = nil
		}
	}
	return rs, noTrigger, bad
}

// rangeTarget parses one BR target, IR5209;R3464032[XC93,SNR25,DBV-20,TEL;NONE,DOP0.00].
func rangeTarget(item string) (TransceiverRangeRow, bool) {
	addr, rest, _ := strings.Cut(item, ";")
	open, closing := strings.IndexByte(rest, '['), strings.LastIndexByte(rest, ']')
	if addr == "" || !strings.HasPrefix(rest, "R") || open < 0 || closing < open {
		return TransceiverRangeRow{}, false
	}
	us, err := strconv.ParseInt(rest[1:open], 10, 64)
	if err != nil || us < 0 {
		return TransceiverRangeRow{}, false
	}
	r := TransceiverRangeRow{MTID: addr, Detected: us != 0}
	if us != 0 {
		s := float64(us) / 1e6
		r.TravelTime = &s
	}
	for _, kv := range strings.Split(rest[open+1:closing], ",") {
		k, v := keyValue(kv)
		switch k {
		case "XC":
			r.Acc = optInt32(v)
		case "SNR":
			r.ASNR = tryFloat(v)
		case "DBV":
			if x := tryFloat(v); x != nil {
				f := float32(*x)
				r.DBV = &f
			}
		case "TEL":
			r.Telemetry = optString(v)
		case "DOP":
			r.Doppler = tryFloat(v)
		}
	}
	return r, true
}

// settings is one row per key of each settings reply, in the stream of the
// reply type's log source for scope.
func settings(msgs []tcvr00Message, scope edid.Scope) (rs []TransceiverSettingRow) {
	for _, m := range msgs {
		logSource, ok := settingsLogSource[m.command]
		if !m.reply || !ok {
			continue
		}
		unit, items := unitAndItems(m.body)
		for _, it := range items {
			k, v := keyValue(it)
			if k == "" {
				continue
			}
			rs = append(rs, TransceiverSettingRow{EDID: edid.Stream(scope, FileTypeTCVR00, logSource), Time: m.at, Unit: unit, Command: m.command, Key: k, Value: v})
		}
	}
	return rs
}

// --- Transceiver rows and builders ---

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

// BuildTransceiverInterrogations builds transceiver_interrogations rows of
// stream edid.
func BuildTransceiverInterrogations(rs []TransceiverInterrogationRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.TransceiverInterrogations)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol   = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
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
		edidCol.Append(edid)
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

// BuildTransceiverRanges builds transceiver_ranges rows of stream edid.
func BuildTransceiverRanges(rs []TransceiverRangeRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.TransceiverRanges)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol    = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
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
		edidCol.Append(edid)
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
	EDID    string // the stream EDID of the reply type's log source, e.g. ...TCVR00.TCVR_CONFIG
	Time    time.Time
	Unit    *string
	Command string // CS, PORT, US, FS, DIAG
	Key     string
	Value   string
}

// BuildTransceiverSettings builds transceiver_settings rows, each of its own
// stream (EDID): one file's settings replies span two log sources.
func BuildTransceiverSettings(rs []TransceiverSettingRow) (arrow.Table, error) {
	b, err := newBuilder(datamodels.TransceiverSettings)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		tm      = col[*array.TimestampBuilder](b, "time")
		unit    = col[*array.StringBuilder](b, "unit")
		command = col[*array.StringBuilder](b, "command")
		key     = col[*array.StringBuilder](b, "key")
		value   = col[*array.StringBuilder](b, "value")
	)
	for _, r := range rs {
		edidCol.Append(r.EDID)
		tm.Append(micros(r.Time))
		appendOpt(unit, r.Unit)
		command.Append(r.Command)
		key.Append(r.Key)
		value.Append(r.Value)
	}
	return b.Table()
}
