package gnss00

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

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

// optString is v, or nil when v is empty.
func optString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
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
			Time: s.at, Talker: talker(f), Mode: optString(f[1]), FixType: int32(fix),
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
			mode = optString(f[9])
		}
		rs = append(rs, datamodels.GNSSCourseSpeedRow{
			Time: s.at, Talker: talker(f), CourseTrue: cTrue, CourseMagnetic: cMag,
			SpeedKnots: knots, SpeedKmh: kmh, Mode: mode,
		})
	}
	return rs, noFix, bad
}
