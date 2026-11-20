// Package gnss00 parses Sonardyne GNSS00.bin logs: the SV3's GNSS receiver
// NMEA output, one sentence per GPSA packet, the packet stamped with the
// SV3's UTC time.
//
// Each sentence type is a log source with its own table: GGA fixes become
// gnss_antenna_positions rows, GSV satellites in view gnss_satellites_in_view,
// GSA fix type and DOPs gnss_dop, and VTG course and speed gnss_course_speed.
// The receiver logs GGA only sparsely (about one every 15 minutes); GSV, GSA
// and VTG arrive about every second. GSA also supplies each GGA fix's PDOP.
// No GST is logged, so the position sigmas are null. Only GGA carries a time,
// so GSV, GSA and VTG rows take the SV3's packet time.
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

	"gitlab.com/earthscope/gnsstools/core/coordinates"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/gpsa"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
)

// pdopWindow is how far a GSA may lie from a GGA (by packet time) for its
// PDOP to be the fix's. GSA is logged about every 2 s.
const pdopWindow = 2 * time.Second

// sentence is one NMEA sentence and the time of the GPSA packet carrying it.
type sentence struct {
	at     time.Time // packet common time, UTC
	fields []string  // comma-separated fields, checksum removed; fields[0] is e.g. "$GPGGA"
}

// ParseAntennaPositions parses the GGA fixes of the GNSS00.bin file at path into
// gnss_antenna_positions rows, one per distinct
// fix time, in ECEF. GGA without a fix, or with a bad checksum or field, are
// skipped and counted.
func ParseAntennaPositions(path string, src edid.LogSource) ([]datamodels.GNSSAntennaPositionRow, error) {
	sentences, badSum, err := readGNSS00File(path)
	if err != nil {
		return nil, err
	}
	rs, noFix, bad := antennaPositions(sentences)
	slog.Debug("parsed GNSS00", "file", filepath.Base(path), "sentences", len(sentences),
		"fixes", len(rs), "no_fix", noFix, "bad_gga", bad, "bad_checksum", badSum)
	for i := range rs {
		rs[i].LogSource = datamodels.GNSS00GNSSPSEUDO
	}
	return rowutil.Filter(rs, src, func(r datamodels.GNSSAntennaPositionRow) edid.LogSource { return r.LogSource }), nil
}

// ParseSatellitesInView parses the GSV sentences of the GNSS00.bin file at path
// into gnss_satellites_in_view rows: one per
// satellite of each GSV group, at the packet time of the group's first
// sentence. Satellite entries that do not parse are skipped and counted.
func ParseSatellitesInView(path string, src edid.LogSource) ([]datamodels.GNSSSatelliteInViewRow, error) {
	sentences, badSum, err := readGNSS00File(path)
	if err != nil {
		return nil, err
	}
	rs, bad := satellitesInView(sentences)
	slog.Debug("parsed GNSS00 GSV", "file", filepath.Base(path), "rows", len(rs), "bad_gsv", bad, "bad_checksum", badSum)
	for i := range rs {
		rs[i].LogSource = datamodels.GNSS00GSV
	}
	return rowutil.Filter(rs, src, func(r datamodels.GNSSSatelliteInViewRow) edid.LogSource { return r.LogSource }), nil
}

// ParseDOP parses the GSA sentences of the GNSS00.bin file at path into gnss_dop
// rows, one per sentence. GSA reporting no DOP
// (no fix) or that do not parse are skipped and counted.
func ParseDOP(path string, src edid.LogSource) ([]datamodels.GNSSDOPRow, error) {
	sentences, badSum, err := readGNSS00File(path)
	if err != nil {
		return nil, err
	}
	rs, noFix, bad := dops(sentences)
	slog.Debug("parsed GNSS00 GSA", "file", filepath.Base(path), "rows", len(rs), "no_fix", noFix, "bad_gsa", bad, "bad_checksum", badSum)
	for i := range rs {
		rs[i].LogSource = datamodels.GNSS00GSA
	}
	return rowutil.Filter(rs, src, func(r datamodels.GNSSDOPRow) edid.LogSource { return r.LogSource }), nil
}

// ParseCourseSpeed parses the VTG sentences of the GNSS00.bin file at path into
// gnss_course_speed rows, one per sentence. VTG
// reporting neither course nor speed (no fix) or that do not parse are
// skipped and counted.
func ParseCourseSpeed(path string, src edid.LogSource) ([]datamodels.GNSSCourseSpeedRow, error) {
	sentences, badSum, err := readGNSS00File(path)
	if err != nil {
		return nil, err
	}
	rs, noFix, bad := courseSpeeds(sentences)
	slog.Debug("parsed GNSS00 VTG", "file", filepath.Base(path), "rows", len(rs), "no_fix", noFix, "bad_vtg", bad, "bad_checksum", badSum)
	for i := range rs {
		rs[i].LogSource = datamodels.GNSS00VTG
	}
	return rowutil.Filter(rs, src, func(r datamodels.GNSSCourseSpeedRow) edid.LogSource { return r.LogSource }), nil
}

// readGNSS00File reads the NMEA sentences of the GNSS00.bin file at path.
func readGNSS00File(path string) ([]sentence, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	return readSentences(f)
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

// --- GSV, GSA and VTG decoding ---

// talker is a sentence's talker, e.g. "GP" for $GPGSV.
func talker(fields []string) string {
	if s := fields[0]; len(s) == 6 {
		return s[1:3]
	}
	return ""
}

// optFloat is the number in field v, or nil when v is empty.
func optFloat(v string) (*float64, error) {
	if v == "" {
		return nil, nil
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil, fmt.Errorf("NMEA number %q", v)
	}
	return &x, nil
}

// satellitesInView is one row per satellite of each GSV group:
// $GPGSV,total,num,inview{,prn,elev,az,snr}. Rows take the packet time of
// their group's first sentence (num 1); a group whose first sentence is
// missing takes its own sentence's time. bad counts sentences and satellite
// entries that do not parse.
func satellitesInView(sentences []sentence) (rs []datamodels.GNSSSatelliteInViewRow, bad int) {
	var groupTime time.Time
	for i := range sentences {
		s := &sentences[i]
		f := s.fields
		if talkerType(f) != "GSV" {
			continue
		}
		if len(f) < 4 {
			bad++
			continue
		}
		num, err1 := strconv.Atoi(f[2])
		inView, err2 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil || num < 1 || inView < 0 {
			bad++
			continue
		}
		if num == 1 || groupTime.IsZero() {
			groupTime = s.at
		}
		at, tk := groupTime, talker(f)
		for j := 4; j < len(f); j += 4 {
			var sat [4]string // prn, elevation, azimuth, snr; a short last entry pads with ""
			copy(sat[:], f[j:])
			if sat[0] == "" {
				continue
			}
			prn, err := strconv.Atoi(sat[0])
			el, err1 := optFloat(sat[1])
			az, err2 := optFloat(sat[2])
			snr, err3 := optFloat(sat[3])
			if err != nil || prn < 1 || err1 != nil || err2 != nil || err3 != nil {
				bad++
				continue
			}
			rs = append(rs, datamodels.GNSSSatelliteInViewRow{
				Time: at, Talker: tk, SatsInView: int32(inView), PRN: int32(prn),
				Elevation: el, Azimuth: az, SNR: snr,
			})
		}
	}
	return rs, bad
}

// dops is one row per GSA sentence:
// $GPGSA,mode,fix,sv1..sv12,pdop,hdop,vdop. noFix counts GSA with no DOP;
// bad counts GSA that do not parse.
func dops(sentences []sentence) (rs []datamodels.GNSSDOPRow, noFix, bad int) {
	for i := range sentences {
		s := &sentences[i]
		f := s.fields
		if talkerType(f) != "GSA" {
			continue
		}
		if len(f) < 18 {
			bad++
			continue
		}
		if f[15] == "" && f[16] == "" && f[17] == "" {
			noFix++
			continue
		}
		fix, err := strconv.Atoi(f[2])
		pdop, err1 := optFloat(f[15])
		hdop, err2 := optFloat(f[16])
		vdop, err3 := optFloat(f[17])
		if err != nil || fix < 1 || fix > 3 || err1 != nil || err2 != nil || err3 != nil {
			bad++
			continue
		}
		var used []string
		for _, sv := range f[3:15] {
			if sv != "" {
				used = append(used, strings.TrimLeft(sv, "0"))
			}
		}
		rs = append(rs, datamodels.GNSSDOPRow{
			Time: s.at, Talker: talker(f), Mode: rowutil.OptString(f[1]), FixType: int32(fix),
			SatellitesUsed: strings.Join(used, ","), NumberUsed: int32(len(used)),
			PDOP: pdop, HDOP: hdop, VDOP: vdop,
		})
	}
	return rs, noFix, bad
}

// courseSpeeds is one row per VTG sentence:
// $GPVTG,course,T,course,M,speed,N,speed,K[,mode]. noFix counts VTG with
// neither course nor speed; bad counts VTG that do not parse.
func courseSpeeds(sentences []sentence) (rs []datamodels.GNSSCourseSpeedRow, noFix, bad int) {
	for i := range sentences {
		s := &sentences[i]
		f := s.fields
		if talkerType(f) != "VTG" {
			continue
		}
		if len(f) < 9 {
			bad++
			continue
		}
		if f[1] == "" && f[3] == "" && f[5] == "" && f[7] == "" {
			noFix++
			continue
		}
		cTrue, err1 := optFloat(f[1])
		cMag, err2 := optFloat(f[3])
		knots, err3 := optFloat(f[5])
		kmh, err4 := optFloat(f[7])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			bad++
			continue
		}
		var mode *string
		if len(f) > 9 {
			mode = rowutil.OptString(f[9])
		}
		rs = append(rs, datamodels.GNSSCourseSpeedRow{
			Time: s.at, Talker: talker(f), CourseTrue: cTrue, CourseMagnetic: cMag,
			SpeedKnots: knots, SpeedKmh: kmh, Mode: mode,
		})
	}
	return rs, noFix, bad
}
