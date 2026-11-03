// Package xsens decodes XSENS MTData2 messages from a byte stream.
//
// A message is: preamble 0xFA, bus id 0xFF, message id, length (or 0xFF
// followed by a 2-byte big-endian extended length), payload, checksum. The
// low byte of the sum of every byte after the preamble is zero. MTData2
// (id 0x36) payloads are a sequence of (2-byte data id, 1-byte length,
// value) items with big-endian float32 values.
//
// Reference: Sonardyne python_scripts XSENSDecoder.py.
package xsens

import (
	"encoding/binary"
	"math"
)

const (
	preamble   byte = 0xFA
	busID      byte = 0xFF
	midMTData2 byte = 0x36

	idEulerAngles   uint16 = 0x2030 // roll, pitch, yaw [deg]
	idAcceleration  uint16 = 0x4020 // x, y, z [m/s^2]
	idMagneticField uint16 = 0xC020 // x, y, z [arbitrary units]
)

// Vec3 is a three-component value; Valid is false when the message lacked it.
type Vec3 struct {
	X, Y, Z float64
	Valid   bool
}

// MTData2 holds the data items the AHRS table needs from one message.
type MTData2 struct {
	Euler         Vec3 // X=roll, Y=pitch, Z=yaw [deg]
	Acceleration  Vec3
	MagneticField Vec3
}

// Decoder accumulates stream bytes and extracts complete messages.
type Decoder struct {
	buf []byte
	pos int64 // stream offset of buf[0]
	// Bad counts candidate messages rejected by checksum.
	Bad int
}

// Write appends stream bytes.
func (d *Decoder) Write(p []byte) { d.buf = append(d.buf, p...) }

// Next returns the next complete MTData2 message and the stream offset of its
// preamble (counting every byte ever written). ok is false when more input is
// needed. Other message ids are skipped.
func (d *Decoder) Next() (msg MTData2, offset int64, ok bool) {
	consumed := 0
	defer func() {
		d.buf = d.buf[consumed:]
		d.pos += int64(consumed)
	}()
	for {
		b := d.buf[consumed:]
		i := 0
		for i+1 < len(b) && !(b[i] == preamble && b[i+1] == busID) {
			i++
		}
		consumed += i
		b = b[i:]
		if len(b) < 4 {
			return MTData2{}, 0, false
		}
		hdr, n := 4, int(b[3])
		if n == 0xFF {
			if len(b) < 6 {
				return MTData2{}, 0, false
			}
			hdr, n = 6, int(binary.BigEndian.Uint16(b[4:6]))
		}
		end := hdr + n + 1
		if len(b) < end {
			return MTData2{}, 0, false
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

func decodeMTData2(p []byte) MTData2 {
	var m MTData2
	for len(p) >= 3 {
		id, n := binary.BigEndian.Uint16(p[0:2]), int(p[2])
		if len(p) < 3+n {
			break
		}
		v := p[3 : 3+n]
		if n == 12 {
			switch id {
			case idEulerAngles:
				m.Euler = vec3(v)
			case idAcceleration:
				m.Acceleration = vec3(v)
			case idMagneticField:
				m.MagneticField = vec3(v)
			}
		}
		p = p[3+n:]
	}
	return m
}

func vec3(v []byte) Vec3 {
	f := func(i int) float64 {
		return float64(math.Float32frombits(binary.BigEndian.Uint32(v[4*i:])))
	}
	return Vec3{X: f(0), Y: f(1), Z: f(2), Valid: true}
}
