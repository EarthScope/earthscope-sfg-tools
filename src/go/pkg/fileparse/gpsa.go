// gpsa: reads Sonardyne GPSA binary logs (AHRS00.bin, NOV000.bin, ...).
//
// Every packet is DLE/STX ... DLE/ETX framed with DLE byte-stuffing and a
// trailing 1-byte XOR checksum. The unstuffed contents start with an 18-byte
// header: a 2-byte big-endian message id (low 10 bits used), then the
// instrument and common times as little-endian uint64 microseconds. The
// common time is UNIX UTC; the instrument time counts from power-on.

package fileparse

import (
	"bufio"
	"encoding/binary"
	"io"
)

const (
	dle byte = 0x10
	stx byte = 0x02
	etx byte = 0x03

	// gpsaHeaderLen is the size of the header preceding each packet's data.
	gpsaHeaderLen = 18
)

// gpsaPacket is one checksum-valid GPSA packet.
type gpsaPacket struct {
	MsgID          uint16
	InstrumentTime uint64 // microseconds since instrument power-on
	CommonTime     uint64 // microseconds since the UNIX epoch, UTC
	Data           []byte // payload after the header, checksum stripped
}

// gpsaReader yields the valid packets of a GPSA stream.
type gpsaReader struct {
	r *bufio.Reader
	// Corrupt counts packets dropped for a bad checksum, an ETX without an
	// STX, or a body too short to hold the header.
	Corrupt int
}

// newGPSAReader returns a gpsaReader over r.
func newGPSAReader(r io.Reader) *gpsaReader {
	return &gpsaReader{r: bufio.NewReaderSize(r, 1<<20)}
}

// Next returns the next valid packet, or io.EOF at the end of the stream.
// A packet cut off by the end of the stream is dropped.
func (g *gpsaReader) Next() (gpsaPacket, error) {
	var (
		gotDLE, gotSTX bool
		buf            []byte
		crc            byte
	)
	for {
		b, err := g.r.ReadByte()
		if err != nil {
			return gpsaPacket{}, err
		}
		switch {
		case b == dle && !gotDLE:
			gotDLE = true
		case b == stx && gotDLE:
			gotDLE, gotSTX = false, true
			buf, crc = buf[:0], 0
		case b == etx && gotDLE:
			gotDLE = false
			if !gotSTX || crc != 0 || len(buf) < gpsaHeaderLen+1 {
				g.Corrupt++
				gotSTX = false
				continue
			}
			return gpsaPacket{
				MsgID:          binary.BigEndian.Uint16(buf[0:2]) & 0x3FF,
				InstrumentTime: binary.LittleEndian.Uint64(buf[2:10]),
				CommonTime:     binary.LittleEndian.Uint64(buf[10:18]),
				Data:           append([]byte(nil), buf[gpsaHeaderLen:len(buf)-1]...),
			}, nil
		default:
			// After a DLE, anything but STX/ETX is a stuffed byte (normally DLE).
			gotDLE = false
			if gotSTX {
				buf = append(buf, b)
				crc ^= b
			}
		}
	}
}
