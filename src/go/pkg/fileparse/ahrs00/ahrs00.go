// Package ahrs00 parses Sonardyne AHRS00.bin logs (XSENS MTData2 wrapped in
// GPSA packets) into the Level 0 attitude_ahrs table.
package ahrs00

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/gpsa"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
)

// ahrs00Stats summarises a parse.
type ahrs00Stats struct {
	Packets        int // valid GPSA packets
	CorruptPackets int // GPSA packets dropped (checksum/framing)
	Messages       int // MTData2 messages decoded
	BadMessages    int // XSENS candidates dropped by checksum
}

type packetStart struct {
	offset int64 // stream offset of the packet's first data byte
	pkt    gpsa.Packet
}

// wrap360 maps an angle in degrees onto [0, 360).
func wrap360(deg float64) float64 {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}
	return deg
}

// magneticHeading converts magnetometer x/y to a heading, using the formula
// the GPSA system uses for its AHRS observation (Sonardyne README:
// Excel DEGREES(ATAN2(magX, -magY)), i.e. atan2(-magY, magX)).
func magneticHeading(x, y float64) float64 {
	return wrap360(math.Atan2(-y, x) * 180 / math.Pi)
}

func ptr(v float64) *float64 { return &v }

// parseAHRS00 reads an AHRS00.bin stream. Each XSENS message takes the time of
// the GPSA packet holding its first byte.
func parseAHRS00(r io.Reader) ([]datamodels.AttitudeAHRSRow, ahrs00Stats, error) {
	var (
		out     []datamodels.AttitudeAHRSRow
		stats   ahrs00Stats
		dec     xsensDecoder
		starts  []packetStart // packets whose bytes are still buffered
		written int64
	)
	g := gpsa.NewReader(r)
	emit := func() {
		for {
			m, off, ok := dec.Next()
			if !ok {
				return
			}
			for len(starts) > 1 && starts[1].offset <= off {
				starts = starts[1:]
			}
			out = append(out, toAttitude(starts[0].pkt, m))
		}
	}
	for {
		pkt, err := g.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, stats, err
		}
		stats.Packets++
		starts = append(starts, packetStart{written, pkt})
		dec.Write(pkt.Data)
		written += int64(len(pkt.Data))
		emit()
	}
	stats.CorruptPackets = g.Corrupt
	stats.Messages = len(out)
	stats.BadMessages = dec.Bad
	return out, stats, nil
}

// toAttitude is the attitude_ahrs row of XSENS message m, timed by its GPSA
// packet p. Measurements the message did not carry are nil.
func toAttitude(p gpsa.Packet, m xsensMTData2) datamodels.AttitudeAHRSRow {
	a := datamodels.AttitudeAHRSRow{
		Time:           time.UnixMicro(int64(p.CommonTime)).UTC(),
		TimeInstrument: float64(p.InstrumentTime) / 1e6,
	}
	if m.Euler.Valid {
		a.R, a.P, a.H = ptr(m.Euler.X), ptr(m.Euler.Y), ptr(wrap360(m.Euler.Z))
	}
	if m.Acceleration.Valid {
		a.Acx, a.Acy, a.Acz = ptr(m.Acceleration.X), ptr(m.Acceleration.Y), ptr(m.Acceleration.Z)
	}
	if m.MagneticField.Valid {
		a.HMag = ptr(magneticHeading(m.MagneticField.X, m.MagneticField.Y))
	}
	return a
}

func readAHRS00File(path string) ([]datamodels.AttitudeAHRSRow, ahrs00Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ahrs00Stats{}, err
	}
	defer f.Close()
	rs, stats, err := parseAHRS00(f)
	if err != nil {
		return nil, stats, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rs, stats, nil
}

// ParseAttitude parses the AHRS00.bin file at path into attitude_ahrs rows
// .
func ParseAttitude(path string, src edid.LogSource) ([]datamodels.AttitudeAHRSRow, error) {
	rs, stats, err := readAHRS00File(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed AHRS00", "file", filepath.Base(path), "rows", len(rs),
		"packets", stats.Packets, "corrupt_packets", stats.CorruptPackets, "bad_messages", stats.BadMessages)
	for i := range rs {
		rs[i].LogSource = datamodels.AHRS00AHRS
	}
	return rowutil.Filter(rs, src, func(r datamodels.AttitudeAHRSRow) edid.LogSource { return r.LogSource }), nil
}

// --- XSENS MTData2 decoding ---
//
// Decodes XSENS MTData2 messages from a byte stream.
//
// A message is: preamble 0xFA, bus id 0xFF, message id, length (or 0xFF
// followed by a 2-byte big-endian extended length), payload, checksum. The
// low byte of the sum of every byte after the preamble is zero. MTData2
// (id 0x36) payloads are a sequence of (2-byte data id, 1-byte length,
// value) items with big-endian float32 values.
//
// Reference: Sonardyne python_scripts XSENSDecoder.py.

const (
	xsensPreamble byte = 0xFA
	xsensBusID    byte = 0xFF
	midMTData2    byte = 0x36

	idEulerAngles   uint16 = 0x2030 // roll, pitch, yaw [deg]
	idAcceleration  uint16 = 0x4020 // x, y, z [m/s^2]
	idMagneticField uint16 = 0xC020 // x, y, z [arbitrary units]
)

// xsensVec3 is a three-component value; Valid is false when the message lacked it.
type xsensVec3 struct {
	X, Y, Z float64
	Valid   bool
}

// xsensMTData2 holds the data items the AHRS table needs from one message.
type xsensMTData2 struct {
	Euler         xsensVec3 // X=roll, Y=pitch, Z=yaw [deg]
	Acceleration  xsensVec3
	MagneticField xsensVec3
}

// xsensDecoder accumulates stream bytes and extracts complete messages.
type xsensDecoder struct {
	buf []byte
	pos int64 // stream offset of buf[0]
	// Bad counts candidate messages rejected by checksum.
	Bad int
}

// Write appends stream bytes.
func (d *xsensDecoder) Write(p []byte) { d.buf = append(d.buf, p...) }

// Next returns the next complete MTData2 message and the stream offset of its
// preamble (counting every byte ever written). ok is false when more input is
// needed. Other message ids are skipped.
func (d *xsensDecoder) Next() (msg xsensMTData2, offset int64, ok bool) {
	consumed := 0
	defer func() {
		d.buf = d.buf[consumed:]
		d.pos += int64(consumed)
	}()
	for {
		b := d.buf[consumed:]
		i := 0
		for i+1 < len(b) && !(b[i] == xsensPreamble && b[i+1] == xsensBusID) {
			i++
		}
		consumed += i
		b = b[i:]
		if len(b) < 4 {
			return xsensMTData2{}, 0, false
		}
		hdr, n := 4, int(b[3])
		if n == 0xFF {
			if len(b) < 6 {
				return xsensMTData2{}, 0, false
			}
			hdr, n = 6, int(binary.BigEndian.Uint16(b[4:6]))
		}
		end := hdr + n + 1
		if len(b) < end {
			return xsensMTData2{}, 0, false
		}
		var sum byte
		for _, c := range b[1:end] {
			sum += c
		}
		if sum != 0 {
			// A 0xFA 0xFF inside other data; resync one byte on.
			d.Bad++
			consumed++
			continue
		}
		start := consumed
		consumed += end
		if b[2] != midMTData2 {
			continue
		}
		return decodeMTData2(b[hdr : hdr+n]), d.pos + int64(start), true
	}
}

func decodeMTData2(p []byte) xsensMTData2 {
	var m xsensMTData2
	for len(p) >= 3 {
		id, n := binary.BigEndian.Uint16(p[0:2]), int(p[2])
		if len(p) < 3+n {
			break
		}
		v := p[3 : 3+n]
		if n == 12 {
			switch id {
			case idEulerAngles:
				m.Euler = decodeXsensVec3(v)
			case idAcceleration:
				m.Acceleration = decodeXsensVec3(v)
			case idMagneticField:
				m.MagneticField = decodeXsensVec3(v)
			}
		}
		p = p[3+n:]
	}
	return m
}

func decodeXsensVec3(v []byte) xsensVec3 {
	f := func(i int) float64 {
		return float64(math.Float32frombits(binary.BigEndian.Uint32(v[4*i:])))
	}
	return xsensVec3{X: f(0), Y: f(1), Z: f(2), Valid: true}
}
