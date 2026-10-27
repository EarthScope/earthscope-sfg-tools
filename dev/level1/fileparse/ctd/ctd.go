// Package ctd parses CTD sound speed profiles: headerless two-column text
// files (e.g. CTD_NBR1_2023_fit, NTH1_2025_CTD_fit) of depth and sound speed,
// separated by whitespace or commas, with depth logged negative-down.
//
// Level 1 stores the profile as logged: depth is flipped to positive-down,
// but the profile is not extrapolated or jittered (compare
// soundspeed_operations.ctd_to_svp_v1/_v2 in the Python package).
package ctd

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

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
)

const (
	// FileType names the file type in stream EDIDs.
	FileType = "CTD"
	// LogType is the one log type CTD files carry, and the Log_Source value
	// of their rows.
	LogType = "CTD"
)

// yearRe finds the profile year in a file name: a 4-digit 19xx/20xx token
// between underscores or at either end, e.g. the 2023 in CTD_NBR1_2023_fit.
var yearRe = regexp.MustCompile(`(?:^|_)((?:19|20)\d\d)(?:_|$)`)

// ProfileTime is the time given to every row of the profile at path:
// 1 January 00:00 UTC of the year in its file name. The files carry no cast
// time, and the year is all the name gives; a fixed time keeps re-ingesting
// a file a replacement of its rows rather than an addition.
func ProfileTime(path string) (time.Time, error) {
	m := yearRe.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return time.Time{}, fmt.Errorf("%s: no profile year in the file name", filepath.Base(path))
	}
	year, _ := strconv.Atoi(m[1])
	return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), nil
}

// SoundSpeed parses the CTD profile at path into sound_speed rows (see
// datamodels.ParsedSchema). logType must be LogType. Lines that are not two
// numbers, or whose depth is above the surface, are skipped and counted.
// The caller releases the table.
func SoundSpeed(path, logType string) (arrow.Table, error) {
	if err := logtype.Check(FileType, logType, LogType); err != nil {
		return nil, err
	}
	at, err := ProfileTime(path)
	if err != nil {
		return nil, err
	}
	rs, bad, err := readProfile(path, at)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed CTD", "file", filepath.Base(path), "rows", len(rs), "bad_lines", bad,
		"time", at.Format(time.DateOnly))
	return datamodels.BuildSoundSpeed(rs, LogType)
}

func readProfile(path string, at time.Time) (rs []datamodels.SoundSpeedRow, bad int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.FieldsFunc(sc.Text(), func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\r'
		})
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			bad++
			continue
		}
		d, err1 := strconv.ParseFloat(fields[0], 64)
		s, err2 := strconv.ParseFloat(fields[1], 64)
		depth := -d
		if err1 != nil || err2 != nil || depth < 0 {
			bad++
			continue
		}
		if depth == 0 {
			depth = 0 // the surface line flips to -0
		}
		rs = append(rs, datamodels.SoundSpeedRow{Time: at, Depth: depth, Speed: s})
	}
	return rs, bad, sc.Err()
}
