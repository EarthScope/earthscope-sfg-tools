// Package novatel holds the NovAtel decoding shared by the NOV000 and NOV770
// file parsers: an ASCII log scanner (plain or GPSA-wrapped) with workarounds
// for gnsstools scanner bugs, and INS log decoding into datamodels.INSPVARow
// and datamodels.INSSTDEVRow. Building the tables is package datamodels' job.
//
// Adapted from src/go/pkg/sfg_utils (parsing.go, ascii_workarounds.go).
package novatel

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/gpsa"
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

// receiverStatusReader applies decimalReceiverStatus to each line of r.
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
