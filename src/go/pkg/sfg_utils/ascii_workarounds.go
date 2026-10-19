package sfg_utils

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
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

// nextMessage is scanner.NextMessage with a panic on a corrupted line turned
// into an error. The scanner has already consumed that line, so the next call
// resumes at the following one.
func nextMessage(scanner novatelascii.Scanner) (msg novatelascii.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("corrupt NovAtel ASCII message: %v", r)
		}
	}()
	return scanner.NextMessage()
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
