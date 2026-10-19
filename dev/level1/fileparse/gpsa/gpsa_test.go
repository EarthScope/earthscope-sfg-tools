package gpsa

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// frame builds a stuffed DLE/STX ... DLE/ETX packet around body + checksum.
func frame(body []byte) []byte {
	var crc byte
	for _, b := range body {
		crc ^= b
	}
	out := []byte{dle, stx}
	for _, b := range append(append([]byte(nil), body...), crc) {
		if b == dle {
			out = append(out, dle)
		}
		out = append(out, b)
	}
	return append(out, dle, etx)
}

func header(id uint16, inst, common uint64) []byte {
	h := make([]byte, HeaderLen)
	binary.BigEndian.PutUint16(h[0:2], id)
	binary.LittleEndian.PutUint64(h[2:10], inst)
	binary.LittleEndian.PutUint64(h[10:18], common)
	return h
}

func TestReader(t *testing.T) {
	data := []byte{0x10, 0x02, 0x03, 0xFA} // includes bytes that need stuffing
	good := frame(append(header(0x83DE, 1_582_568_483, 1_780_431_522_199_201), data...))
	bad := frame(header(1, 2, 3))
	bad[len(bad)-3] ^= 0xFF // corrupt the checksum

	g := NewReader(bytes.NewReader(append(append(bad, good...), dle, stx, 0x01)))
	p, err := g.Next()
	if err != nil {
		t.Fatal(err)
	}
	if p.MsgID != 0x83DE&0x3FF || p.InstrumentTime != 1_582_568_483 ||
		p.CommonTime != 1_780_431_522_199_201 || !bytes.Equal(p.Data, data) {
		t.Fatalf("unexpected packet %+v", p)
	}
	if g.Corrupt != 1 {
		t.Errorf("Corrupt = %d, want 1", g.Corrupt)
	}
	if _, err := g.Next(); err != io.EOF {
		t.Errorf("truncated tail: err = %v, want EOF", err)
	}
}
