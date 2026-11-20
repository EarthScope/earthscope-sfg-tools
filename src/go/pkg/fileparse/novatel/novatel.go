// Package novatel holds the NovAtel decoding shared by NOV000, NVA, QCPIN and the
// SV3 event rows (internal/sv3): the ASCII log scanner (plain or GPSA-wrapped) with
// workarounds for gnsstools scanner bugs, INSPVA/INSSTDEV decoding, RANGEA
// epoch decoding, and time and INS status names. The tables are built by the
// datamodels row builders.
//
// The ASCII scanner workarounds were adapted from the former pkg/sfg_utils.
package novatel

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/gpsa"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
)

// Workarounds for two bugs in gnsstools' novatel_ascii scanner (v0.88-v0.105);
// remove once gnsstools fixes them.
//
//  1. It parses the hex receiver-status header field with strconv.ParseFloat,
//     dropping every long message whose status contains a hex letter (e.g.
//     "024c0000"). The scanner does not verify the CRC, so rewriting the
//     field is safe.
//  2. DeserializeMessage slices and indexes the header without bounds checks,
//     so a corrupted line (e.g. "#...*" with no ';') panics and kills the
//     whole file.

// ASCIIScanner yields NovAtel ASCII logs.
type ASCIIScanner struct {
	scanner novatelascii.Scanner
}

// NewASCIIScanner scans a plain NovAtel ASCII log stream.
func NewASCIIScanner(r io.Reader) *ASCIIScanner {
	return &ASCIIScanner{scanner: novatelascii.NewScanner(newReceiverStatusReader(r))}
}

// NewGPSAASCIIScanner scans NovAtel ASCII logs carried in a GPSA binary
// stream (NOV000.bin).
func NewGPSAASCIIScanner(r io.Reader) *ASCIIScanner {
	return NewASCIIScanner(&gpsaASCIIReader{g: gpsa.NewReader(r)})
}

// Next returns the next log, an error for a log that failed to parse (the
// scanner resumes at the following line), or io.EOF.
func (s *ASCIIScanner) Next() (msg novatelascii.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("corrupt NovAtel ASCII message: %v", r)
		}
	}()
	return s.scanner.NextMessage()
}

// decimalReceiverStatus rewrites the receiver-status field of a NovAtel ASCII
// long-message header from hex to decimal; other lines are returned unchanged.
func decimalReceiverStatus(line string) string {
	start := strings.IndexByte(line, '#')
	if start == -1 {
		return line
	}
	end := strings.IndexByte(line[start:], ';')
	if end == -1 {
		return line
	}
	end += start
	fields := strings.Split(line[start+1:end], ",")
	if len(fields) < 10 {
		return line
	}
	status, err := strconv.ParseUint(fields[7], 16, 32)
	if err != nil {
		return line
	}
	fields[7] = strconv.FormatUint(status, 10)
	return line[:start+1] + strings.Join(fields, ",") + line[end:]
}

// maxNovatelLine is the longest line receiverStatusReader passes on: below
// the 64 KiB token limit of the bufio.Scanner inside gnsstools' NovAtel
// ASCII scanner, and far above any real log line.
const maxNovatelLine = 60 << 10

// receiverStatusReader applies decimalReceiverStatus to each line of r, and
// replaces a line longer than maxNovatelLine with an empty one.
type receiverStatusReader struct {
	r   *bufio.Reader
	buf []byte
}

func newReceiverStatusReader(r io.Reader) *receiverStatusReader {
	return &receiverStatusReader{r: bufio.NewReader(r)}
}

func (s *receiverStatusReader) Read(p []byte) (int, error) {
	if len(s.buf) == 0 {
		line, err := s.r.ReadString('\n')
		if len(line) == 0 {
			return 0, err
		}
		if len(line) > maxNovatelLine {
			// gnsstools' scanner cannot hold the line and retries it
			// forever (bufio.ErrTooLong); no real log is this long, so it
			// is corrupt: pass on an empty line instead.
			line = "\n"
		}
		s.buf = []byte(decimalReceiverStatus(line))
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

// gpsaASCIIReader flattens GPSA packets into the continuous NovAtel ASCII
// byte stream they carry.
//
// GPSA chunks queued ASCII logs into packets with no regard for log
// boundaries: one large log can span several packets and short logs can be
// interleaved between its fragments, so packet boundaries carry no framing
// meaning. The recording can also drop a log's checksum and CRLF; to stop
// the next log being absorbed into the truncated one, every sync char is
// forced onto its own line, so only the truncated log fails to parse.
type gpsaASCIIReader struct {
	g           *gpsa.Reader
	buf         []byte
	lastByte    byte
	haveEmitted bool
}

func (r *gpsaASCIIReader) appendChunk(chunk []byte) {
	for _, b := range chunk {
		if (b == '#' || b == '%') && r.haveEmitted && r.lastByte != '\n' {
			r.buf = append(r.buf, '\n')
		}
		r.buf = append(r.buf, b)
		r.lastByte = b
		r.haveEmitted = true
	}
}

func (r *gpsaASCIIReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		pkt, err := r.g.Next()
		if err != nil {
			return 0, err
		}
		r.appendChunk(pkt.Data)
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// parseFloats parses fields[from:from+len(dst)] into dst.
func parseFloats(log string, fields []string, from int, dst ...*float64) error {
	for i, d := range dst {
		v, err := strconv.ParseFloat(strings.TrimSpace(fields[from+i]), 64)
		if err != nil {
			return fmt.Errorf("%s field %d: %w", log, from+i, err)
		}
		*d = v
	}
	return nil
}

// parseNovatelINSPVA decodes an INSPVAA data section:
// week,seconds,lat,lon,height,north,east,up,roll,pitch,azimuth,status
func parseNovatelINSPVA(data string, t time.Time) (datamodels.INSPVARow, error) {
	f := strings.Split(data, ",")
	if len(f) < 12 {
		return datamodels.INSPVARow{}, fmt.Errorf("INSPVAA: %d fields, want 12", len(f))
	}
	r := datamodels.INSPVARow{Time: t, INSStatus: rowutil.OptString(strings.TrimSpace(f[11]))}
	err := parseFloats("INSPVAA", f, 2, &r.Lat, &r.Lon, &r.Alt, &r.North, &r.East, &r.Up, &r.Roll, &r.Pitch, &r.Azimuth)
	return r, err
}

// parseNovatelINSSTDEV decodes an INSSTDEVA data section:
// lat_sig,lon_sig,height_sig,north_sig,east_sig,up_sig,roll_sig,pitch_sig,azimuth_sig,ext_sol_stat,time_since_update,...
func parseNovatelINSSTDEV(data string, t time.Time) (datamodels.INSSTDEVRow, error) {
	f := strings.Split(data, ",")
	if len(f) < 9 {
		return datamodels.INSSTDEVRow{}, fmt.Errorf("INSSTDEVA: %d fields, want at least 9", len(f))
	}
	r := datamodels.INSSTDEVRow{Time: t}
	if err := parseFloats("INSSTDEVA", f, 0, &r.LatSig, &r.LonSig, &r.AltSig, &r.NorthSig, &r.EastSig, &r.UpSig,
		&r.RollSig, &r.PitchSig, &r.AzimuthSig); err != nil {
		return r, err
	}
	if len(f) > 10 {
		r.ExtSolStat = rowutil.OptString(strings.TrimSpace(f[9]))
		r.TimeSinceUpdate = rowutil.OptString(strings.TrimSpace(f[10]))
	}
	return r, nil
}

// ErrBadEpochTime marks a RANGEA whose epoch time fails
// gnssiceberg.ValidateEpoch: a decode error (the ASCII CRC is not checked),
// not data.
var ErrBadEpochTime = errors.New("bad epoch time")

// --- NovAtel ASCII log scanning (NOV000, NVA) ---

// The NovAtel ASCII log types the parsers read (NOV000, NVA, QCPIN), one per
// table. Each is also the log source value of its rows.
const (
	LogRANGEA    = "RANGEA"        // GNSSObs
	LogINSPVAA   = edid.LogINSPVAA // INSPVA
	LogINSSTDEVA = "INSSTDEVA"     // INSSTDEV
)

// Records are the decoded logs of one NovAtel ASCII scan.
type Records struct {
	Epochs   []observation.Epoch
	INSPVA   []datamodels.INSPVARow
	INSSTDEV []datamodels.INSSTDEVRow
}

// Scan decodes the want logs of the fileType file at path, read
// with newScanner (NewGPSAASCIIScanner for GPSA-wrapped logs such as
// NOV000.bin, NewASCIIScanner for plain ones such as NVA). Logs with no
// time fix (header time status UNKNOWN, or week 0, which a receiver still
// adjusting its clock logs under other statuses), and RANGEA logs whose epoch
// time fails gnssiceberg.ValidateEpoch, are skipped.
func Scan(path, fileType string, newScanner func(io.Reader) *ASCIIScanner, want string) (Records, error) {
	f, err := os.Open(path)
	if err != nil {
		return Records{}, err
	}
	defer f.Close()

	var (
		r       Records
		failed  int
		skipped int
		badTime int
	)
	s := newScanner(f)
	for {
		msg, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failed++
			continue
		}
		m, ok := msg.(novatelascii.LongMessage)
		if !ok || m.Msg != want {
			continue
		}
		if m.TimeStatus == "UNKNOWN" || m.Week == 0 {
			skipped++
			continue
		}
		if err := decodeNovatelASCII(m, &r.Epochs, &r.INSPVA, &r.INSSTDEV); err != nil {
			if errors.Is(err, ErrBadEpochTime) {
				badTime++
			} else {
				failed++
			}
			slog.Debug("bad NovAtel log", "file", filepath.Base(path), "log", m.Msg, "error", err)
		}
	}
	slog.Debug("parsed NovAtel ASCII", "type", fileType, "file", filepath.Base(path), "log", want,
		"epochs", len(r.Epochs), "inspva", len(r.INSPVA), "insstdev", len(r.INSSTDEV),
		"failed", failed, "unknown_time", skipped, "bad_time", badTime)
	return r, nil
}

// novatelTimeStatus is m's header time status, or nil when it has none.
func novatelTimeStatus(m novatelascii.LongMessage) *string {
	if m.TimeStatus == "" {
		return nil
	}
	s := m.TimeStatus
	return &s
}

// decodeNovatelASCII decodes a RANGEA, INSPVAA or INSSTDEVA log onto the
// matching slice; other logs are ignored.
func decodeNovatelASCII(m novatelascii.LongMessage, epochs *[]observation.Epoch, inspva *[]datamodels.INSPVARow, insstdev *[]datamodels.INSSTDEVRow) error {
	switch m.Msg {
	case LogRANGEA:
		epoch, err := RANGEAEpoch(m)
		if err != nil {
			return err
		}
		*epochs = append(*epochs, epoch)
	case LogINSPVAA:
		r, err := parseNovatelINSPVA(m.Data, m.Time())
		if err != nil {
			return err
		}
		r.TimeStatus = novatelTimeStatus(m)
		*inspva = append(*inspva, r)
	case LogINSSTDEVA:
		r, err := parseNovatelINSSTDEV(m.Data, m.Time())
		if err != nil {
			return err
		}
		r.TimeStatus = novatelTimeStatus(m)
		*insstdev = append(*insstdev, r)
	}
	return nil
}

// RANGEAEpoch decodes a RANGEA log into an observation epoch. An epoch with
// no satellites is an error, and one whose time fails
// gnssiceberg.ValidateEpoch an ErrBadEpochTime error.
func RANGEAEpoch(m novatelascii.LongMessage) (observation.Epoch, error) {
	rangea, err := novatelascii.DeserializeRANGEA(m.Data)
	if err != nil {
		return observation.Epoch{}, err
	}
	epoch, err := rangea.SerializeGNSSEpoch(m.Time())
	if err != nil {
		return observation.Epoch{}, err
	}
	if len(epoch.Satellites) == 0 {
		return observation.Epoch{}, fmt.Errorf("RANGEA with no satellites")
	}
	if err := gnssiceberg.ValidateEpoch(epoch.Time); err != nil {
		return observation.Epoch{}, fmt.Errorf("%w: %v", ErrBadEpochTime, err)
	}
	return epoch, nil
}

// TimeStatusUnknown is the NovAtel time status UNKNOWN: no time fix.
const TimeStatusUnknown = 20

// timeStatuses are the NovAtel OEM7 GPS reference time statuses
// (https://docs.novatel.com/OEM7/Content/Messages/GPS_Reference_Time_Statu.htm).
var timeStatuses = map[int]string{
	20: "UNKNOWN", 60: "APPROXIMATE", 80: "COARSEADJUSTING", 100: "COARSE",
	120: "COARSESTEERING", 130: "FREEWHEELING", 140: "FINEADJUSTING", 160: "FINE",
	170: "FINEBACKUPSTEERING", 180: "FINESTEERING", 200: "SATTIME",
}

// insStatuses are the NovAtel OEM7 inertial solution statuses
// (https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSATT.htm#InertialSolutionStatus).
var insStatuses = map[int]string{
	0: "INS_INACTIVE", 1: "INS_ALIGNING", 2: "INS_HIGH_VARIANCE", 3: "INS_SOLUTION_GOOD",
	6: "INS_SOLUTION_FREE", 7: "INS_ALIGNMENT_COMPLETE", 8: "DETERMINING_ORIENTATION",
	9: "WAITING_INITIALPOS", 10: "WAITING_AZIMUTH", 11: "INITIALIZING_BIASES",
	12: "MOTION_DETECT", 14: "WAITING_ALIGNMENTORIENTATION",
}

// TimeStatusName is the name of NovAtel time status code, as the ASCII logs
// print it, e.g. FINESTEERING for 180; an unlisted code is its number.
func TimeStatusName(code int) string { return novatelStatusName(timeStatuses, code) }

// INSStatusName is the name of NovAtel inertial solution status code, as the
// ASCII logs print it, e.g. INS_SOLUTION_GOOD for 3; an unlisted code is its
// number.
func INSStatusName(code int) string { return novatelStatusName(insStatuses, code) }

func novatelStatusName(names map[int]string, code int) string {
	if s, ok := names[code]; ok {
		return s
	}
	return strconv.Itoa(code)
}

// ObsRows is one GNSSObsRow per epoch, all of log source src.
func ObsRows(src edid.LogSource, epochs []observation.Epoch) []datamodels.GNSSObsRow {
	rows := make([]datamodels.GNSSObsRow, len(epochs))
	for i, e := range epochs {
		rows[i] = datamodels.GNSSObsRow{LogSource: src, Epoch: e}
	}
	return rows
}
