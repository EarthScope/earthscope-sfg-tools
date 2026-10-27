// Package gnss00 parses Sonardyne GNSS00.bin logs: the SV3's GNSS receiver
// NMEA output, one sentence per GPSA packet, the packet stamped with the
// SV3's UTC time.
//
// GGA fixes become gnss_antenna_positions rows. The receiver logs them only
// sparsely (about one every 15 minutes); most sentences are GSV (satellites
// in view), VTG (velocity) and GSA (DOPs), which no Level 1 table holds. GSA
// supplies each fix's PDOP. No GST is logged, so the position sigmas are null.
package gnss00

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"gitlab.com/earthscope/gnsstools/core/coordinates"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/gpsa"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
)

const (
	// FileType names the file type in stream EDIDs.
	FileType = "GNSS00"
	// LogGGA is the NMEA sentence gnss_antenna_positions rows come from, and
	// their Log_Source value.
	LogGGA = "GGA"
)

// pdopWindow is how far a GSA may lie from a GGA (by packet time) for its
// PDOP to be the fix's. GSA is logged about every 2 s.
const pdopWindow = 2 * time.Second

// sentence is one NMEA sentence and the time of the GPSA packet carrying it.
type sentence struct {
	at     time.Time // packet common time, UTC
	fields []string  // comma-separated fields, checksum removed; fields[0] is e.g. "$GPGGA"
}

// AntennaPositions parses the GGA fixes of the GNSS00.bin file at path into
// gnss_antenna_positions rows (see datamodels.ParsedSchema), one per distinct
// fix time, in ECEF. GGA without a fix, or with a bad checksum or field, are
// skipped and counted. logType must be LogGGA. The caller releases the table.
func AntennaPositions(path, logType string) (arrow.Table, error) {
	if err := logtype.Check(FileType, logType, LogGGA); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sentences, badSum, err := readSentences(f)
	if err != nil {
		return nil, err
	}
	rs, noFix, bad := antennaPositions(sentences)
	slog.Debug("parsed GNSS00", "file", filepath.Base(path), "sentences", len(sentences),
		"fixes", len(rs), "no_fix", noFix, "bad_gga", bad, "bad_checksum", badSum)
	return datamodels.BuildGNSSAntennaPositions(rs, LogGGA)
}

// readSentences reads the NMEA sentences of a GPSA stream, dropping those
// with a bad or missing checksum.
func readSentences(r io.Reader) (out []sentence, badSum int, err error) {
	g := gpsa.NewReader(r)
	for {
		p, err := g.Next()
		if errors.Is(err, io.EOF) {
			return out, badSum, nil
		}
		if err != nil {
			return nil, 0, err
		}
		at := time.UnixMicro(int64(p.CommonTime)).UTC()
		sc := bufio.NewScanner(strings.NewReader(string(p.Data)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "$") {
				continue
			}
			body, ok := checksum(line)
			if !ok {
				badSum++
				continue
			}
			out = append(out, sentence{at: at, fields: strings.Split(body, ",")})
		}
	}
}

// checksum verifies an NMEA sentence "$<body>*hh" and returns "$<body>".
func checksum(line string) (string, bool) {
	body, sum, ok := strings.Cut(line, "*")
	if !ok || len(sum) < 2 {
		return "", false
	}
	want, err := strconv.ParseUint(sum[:2], 16, 8)
	if err != nil {
		return "", false
	}
	var x byte
	for i := 1; i < len(body); i++ {
		x ^= body[i]
	}
	return body, x == byte(want)
}

// talkerType is a sentence's type without its talker, e.g. "GGA" for $GPGGA
// or $GNGGA.
func talkerType(fields []string) string {
	if s := fields[0]; len(s) == 6 {
		return s[3:]
	}
	return ""
}

// antennaPositions is one row per GGA fix time. noFix counts GGA reporting
// quality 0 or no position; bad counts GGA that do not parse.
func antennaPositions(sentences []sentence) (rs []datamodels.GNSSAntennaPositionRow, noFix, bad int) {
	var (
		lastGSA *sentence
		seen    = map[time.Time]bool{}
	)
	for i := range sentences {
		s := &sentences[i]
		switch talkerType(s.fields) {
		case "GSA":
			lastGSA = s
		case "GGA":
			r, err := gga(s)
			switch {
			case errors.Is(err, errNoFix):
				noFix++
			case err != nil:
				bad++
			case !seen[r.Time]:
				seen[r.Time] = true
				if lastGSA != nil && s.at.Sub(lastGSA.at).Abs() <= pdopWindow {
					r.PDOP = pdop(lastGSA)
				}
				rs = append(rs, r)
			}
		}
	}
	return rs, noFix, bad
}

var errNoFix = errors.New("GGA without a fix")

// gga converts a GGA sentence:
// $GPGGA,hhmmss.ss,ddmm.mm,N,dddmm.mm,W,q,sats,hdop,alt,M,sep,M,age,station.
// The height above the ellipsoid is alt + sep. The date, which GGA does not
// carry, is the packet's, moved a day either way if that brings the fix
// closer to the packet time (fixes either side of midnight).
func gga(s *sentence) (datamodels.GNSSAntennaPositionRow, error) {
	f := s.fields
	if len(f) < 12 {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GGA has %d fields", len(f))
	}
	q, err := strconv.Atoi(f[6])
	if err != nil || q < 0 || q > 9 {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GGA quality %q", f[6])
	}
	if q == 0 || f[2] == "" || f[4] == "" {
		return datamodels.GNSSAntennaPositionRow{}, errNoFix
	}
	t, err1 := fixTime(f[1], s.at)
	lat, err2 := degrees(f[2], f[3], "N", "S", 2)
	lon, err3 := degrees(f[4], f[5], "E", "W", 3)
	alt, err4 := strconv.ParseFloat(f[9], 64)
	sep, err5 := strconv.ParseFloat(f[11], 64)
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		return datamodels.GNSSAntennaPositionRow{}, err
	}
	hae := alt + sep
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 || math.Abs(hae) > 1000 {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GGA out of range: lat %v lon %v hae %v", lat, lon, hae)
	}
	ecef := coordinates.GeodeticToECEF(coordinates.GeodeticCoord{
		Latitude: lat * math.Pi / 180, Longitude: lon * math.Pi / 180, Height: hae,
	})
	r := datamodels.GNSSAntennaPositionRow{
		Time: t,
		X:    ecef[0], Y: ecef[1], Z: ecef[2],
		Q: int32(q),
	}
	if n, err := strconv.Atoi(f[7]); err == nil && n >= 0 {
		n32 := int32(n)
		r.NumberOfSatellites = &n32
	}
	return r, nil
}

// fixTime is GGA's hhmmss.ss time of day on the date that puts it nearest
// packet, the packet's UTC time.
func fixTime(hms string, packet time.Time) (time.Time, error) {
	if len(hms) < 6 {
		return time.Time{}, fmt.Errorf("GGA time %q", hms)
	}
	h, err1 := strconv.Atoi(hms[0:2])
	m, err2 := strconv.Atoi(hms[2:4])
	sec, err3 := strconv.ParseFloat(hms[4:], 64)
	if err := errors.Join(err1, err2, err3); err != nil || h > 23 || m > 59 || sec >= 61 {
		return time.Time{}, fmt.Errorf("GGA time %q", hms)
	}
	day := time.Date(packet.Year(), packet.Month(), packet.Day(), 0, 0, 0, 0, time.UTC)
	tod := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(math.Round(sec*1e6))*time.Microsecond
	best := day.Add(tod)
	for _, d := range []int{-1, 1} {
		if c := day.AddDate(0, 0, d).Add(tod); c.Sub(packet).Abs() < best.Sub(packet).Abs() {
			best = c
		}
	}
	return best, nil
}

// degrees converts NMEA (d)ddmm.mmmm and a hemisphere to signed decimal
// degrees; degDigits is 2 for latitude, 3 for longitude.
func degrees(v, hemi, pos, neg string, degDigits int) (float64, error) {
	if len(v) < degDigits+2 {
		return 0, fmt.Errorf("NMEA angle %q", v)
	}
	d, err1 := strconv.ParseFloat(v[:degDigits], 64)
	m, err2 := strconv.ParseFloat(v[degDigits:], 64)
	if err := errors.Join(err1, err2); err != nil || m >= 60 {
		return 0, fmt.Errorf("NMEA angle %q", v)
	}
	deg := d + m/60
	switch hemi {
	case pos:
		return deg, nil
	case neg:
		return -deg, nil
	}
	return 0, fmt.Errorf("NMEA hemisphere %q", hemi)
}

// pdop is a GSA sentence's PDOP:
// $GPGSA,mode,fix,sv1..sv12,pdop,hdop,vdop. Nil when absent or not a number.
func pdop(s *sentence) *float64 {
	if len(s.fields) < 18 {
		return nil
	}
	v, err := strconv.ParseFloat(s.fields[15], 64)
	if err != nil || v < 0 {
		return nil
	}
	return &v
}
