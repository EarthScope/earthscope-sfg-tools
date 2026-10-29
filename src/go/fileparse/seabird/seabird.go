// Package seabird parses Sea-Bird SBE .cnv sound velocity profiles: a header
// of "*" and "#" lines ending at "*END*", then one whitespace-separated row
// of values per scan.
//
// Depth and sound speed are found by the header's "# name N = <short>:"
// lines (depth: dep*, e.g. depSM; speed: sv*, e.g. svCM); without them the
// columns are 0 and 5, as in soundspeed_operations.parse_seabird_lines in
// the Python package. The profile is stored as logged, not extrapolated.
package seabird

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

const (
	// FileType names the file type in stream EDIDs.
	FileType = "SEABIRD"
	// LogType is the one log type Sea-Bird files carry, and the Log_Source
	// value of their rows.
	LogType = "SEABIRD"
)

// Default columns when the header names none, as in the Python parser.
const (
	defaultDepthCol = 0
	defaultSpeedCol = 5
)

var (
	nameRe = regexp.MustCompile(`^#\s*name\s+(\d+)\s*=\s*([^:\s]+)`)
	// Cast time headers, in order of preference: the NMEA fix time, then
	// the instrument's start time.
	timeRes = []*regexp.Regexp{
		regexp.MustCompile(`^\*\s*NMEA UTC \(Time\)\s*=\s*(.+)$`),
		regexp.MustCompile(`^#\s*start_time\s*=\s*(.+?)\s*(\[.*)?$`),
	}
	badFlagRe = regexp.MustCompile(`^#\s*bad_flag\s*=\s*(\S+)`)
)

// header is what SoundSpeed reads from a .cnv header.
type header struct {
	depthCol, speedCol int
	castTime           time.Time
	badFlag            *float64
}

// SoundSpeed parses the Sea-Bird .cnv file at path into sound_speed rows (see
// datamodels.ParsedSchema), every row at the header's cast time. Data rows
// that are short, not numeric, flagged bad or above the surface are skipped
// and counted. The caller releases the table.
func SoundSpeed(path string) (arrow.Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)

	h, err := readHeader(sc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	var (
		rs  []datamodels.SoundSpeedRow
		bad int
	)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		if max(h.depthCol, h.speedCol) >= len(fields) {
			bad++
			continue
		}
		d, err1 := strconv.ParseFloat(fields[h.depthCol], 64)
		s, err2 := strconv.ParseFloat(fields[h.speedCol], 64)
		if err1 != nil || err2 != nil || d < 0 ||
			(h.badFlag != nil && (d == *h.badFlag || s == *h.badFlag)) {
			bad++
			continue
		}
		rs = append(rs, datamodels.SoundSpeedRow{Time: h.castTime, Depth: d, Speed: s})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	slog.Debug("parsed SEABIRD", "file", filepath.Base(path), "rows", len(rs), "bad_lines", bad,
		"cast_time", h.castTime.Format(time.RFC3339))
	return datamodels.BuildSoundSpeed(rs, LogType)
}

// readHeader reads sc up to and including the "*END*" line.
func readHeader(sc *bufio.Scanner) (header, error) {
	h := header{depthCol: -1, speedCol: -1}
	var times [2]string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "*END*") {
			if h.depthCol < 0 || h.speedCol < 0 {
				h.depthCol, h.speedCol = defaultDepthCol, defaultSpeedCol
			}
			for _, s := range times {
				if s == "" {
					continue
				}
				t, err := parseHeaderTime(s)
				if err != nil {
					return h, err
				}
				h.castTime = t
				return h, nil
			}
			return h, fmt.Errorf("no cast time (NMEA UTC (Time) or start_time) in the header")
		}
		if m := nameRe.FindStringSubmatch(line); m != nil {
			col, _ := strconv.Atoi(m[1])
			switch short := m[2]; {
			case h.depthCol < 0 && strings.HasPrefix(short, "dep"):
				h.depthCol = col
			case h.speedCol < 0 && strings.HasPrefix(short, "sv"):
				h.speedCol = col
			}
			continue
		}
		if m := badFlagRe.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				h.badFlag = &v
			}
			continue
		}
		for i, re := range timeRes {
			if m := re.FindStringSubmatch(line); m != nil && times[i] == "" {
				times[i] = strings.TrimSpace(m[1])
			}
		}
	}
	if err := sc.Err(); err != nil {
		return h, err
	}
	return h, fmt.Errorf("no *END* line ending the header")
}

// parseHeaderTime parses a Sea-Bird header time, e.g. "Sep 12 2025 10:15:00",
// as UTC.
func parseHeaderTime(s string) (time.Time, error) {
	for _, layout := range []string{"Jan 02 2006 15:04:05", "Jan 2 2006 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised header time %q", s)
}
