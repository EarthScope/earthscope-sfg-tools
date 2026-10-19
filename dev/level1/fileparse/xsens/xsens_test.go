package xsens

import (
	"encoding/binary"
	"math"
	"testing"
)

func item(id uint16, x, y, z float32) []byte {
	b := []byte{byte(id >> 8), byte(id), 12}
	for _, v := range []float32{x, y, z} {
		b = binary.BigEndian.AppendUint32(b, math.Float32bits(v))
	}
	return b
}

// message builds an XSENS message with a valid checksum.
func message(mid byte, payload []byte) []byte {
	m := []byte{preamble, busID, mid, byte(len(payload))}
	m = append(m, payload...)
	var sum byte
	for _, c := range m[1:] {
		sum += c
	}
	return append(m, -sum)
}

func TestDecoderAcrossWrites(t *testing.T) {
	ok := message(midMTData2, append(item(idEulerAngles, 1.5, -0.25, -170), item(idMagneticField, 0.2, -0.4, 0.7)...))
	other := message(0x30, nil)
	corrupt := append([]byte{}, ok...)
	corrupt[6] ^= 1
	stream := append(append(append([]byte{0x00, 0xFA}, corrupt...), other...), ok...)

	var d Decoder
	var got []MTData2
	var offs []int64
	for i := 0; i < len(stream); i += 7 { // feed in chunks that split messages
		d.Write(stream[i:min(i+7, len(stream))])
		for {
			m, off, ok := d.Next()
			if !ok {
				break
			}
			got, offs = append(got, m), append(offs, off)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if want := int64(2 + len(corrupt) + len(other)); offs[0] != want {
		t.Errorf("offset = %d, want %d", offs[0], want)
	}
	m := got[0]
	if !m.Euler.Valid || m.Euler.X != 1.5 || m.Euler.Y != -0.25 || m.Euler.Z != -170 {
		t.Errorf("euler = %+v", m.Euler)
	}
	if m.Acceleration.Valid || !m.MagneticField.Valid {
		t.Errorf("validity: acc %v mag %v", m.Acceleration.Valid, m.MagneticField.Valid)
	}
	if d.Bad == 0 {
		t.Error("corrupt message was not counted")
	}
}
