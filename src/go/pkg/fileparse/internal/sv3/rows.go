package sv3

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"gitlab.com/earthscope/gnsstools/core/coordinates"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// TravelTimes pairs each range event with the most recent preceding
// interrogation; a zero range (no reply detected) yields no row.
func TravelTimes(events []Event) []datamodels.TWTTRow {
	var (
		out                       []datamodels.TWTTRow
		ping                      *Event
		noPing, noReply, badTimes int
	)
	for i := range events {
		e := &events[i]
		switch {
		case e.Event == "interrogation":
			ping = e
			continue
		case e.Event != "range" || e.Range == nil:
			continue
		case ping == nil:
			noPing++
			continue
		case e.Range.Range == 0:
			noReply++
			continue
		}
		sent, err1 := UnixMicros(ping.Time.Common)
		recv, err2 := UnixMicros(e.Time.Common)
		if err1 != nil || err2 != nil {
			badTimes++
			continue
		}
		snr, tat := float64(e.Range.Diag.SNR), e.Range.TAT/1000
		out = append(out, datamodels.TWTTRow{
			MTID:       e.Range.CN,
			TTransmit:  time.UnixMicro(sent).UTC(),
			TReceive:   time.UnixMicro(recv).UTC(),
			TravelTime: e.Range.Range,
			DBV:        float32(e.Range.Diag.DBV),
			Acc:        int32(math.Round(float64(e.Range.Diag.XC))),
			ASNR:       &snr,
			TAT:        &tat,
		})
	}
	slog.Debug("SV3 travel times", "no_interrogation", noPing, "no_reply", noReply, "bad_times", badTimes)
	return out
}

// Attitudes is one attitude_ahrs row per distinct AHRS block (consecutive
// events usually repeat the same block).
func Attitudes(events []Event) []datamodels.AttitudeAHRSRow {
	var (
		out  []datamodels.AttitudeAHRSRow
		last json.Number
		bad  int
	)
	for i := range events {
		a, err := events[i].AHRS()
		if err != nil {
			bad++
			continue
		}
		if a == nil || a.Time.Common == last {
			continue
		}
		t, err := UnixMicros(a.Time.Common)
		if err != nil {
			bad++
			continue
		}
		last = a.Time.Common
		r := datamodels.AttitudeAHRSRow{
			Time:           time.UnixMicro(t).UTC(),
			TimeInstrument: a.Time.Instrument,
			TimeStartCount: a.Time.StartCount,
			Acx:            a.Acx, Acy: a.Acy, Acz: a.Acz,
			H: a.H, HMag: a.HMag, P: a.P, R: a.R,
		}
		if a.Time.Status != "" {
			r.TimeStatus = &a.Time.Status
		}
		out = append(out, r)
	}
	if bad > 0 {
		slog.Debug("SV3 bad AHRS blocks", "count", bad)
	}
	return out
}

// AntennaPositions is one gnss_antenna_positions row per distinct GNSS block,
// keyed by its fix time (events repeat the current block); the first block
// seen for a time wins. Blocks without a usable position, time or fix
// quality are skipped and counted.
func AntennaPositions(events []Event) []datamodels.GNSSAntennaPositionRow {
	var (
		out  []datamodels.GNSSAntennaPositionRow
		seen = map[json.Number]bool{}
		bad  int
	)
	for i := range events {
		g, err := events[i].GNSS()
		if err != nil {
			bad++
			continue
		}
		if g == nil || seen[g.Time.Common] {
			continue
		}
		seen[g.Time.Common] = true
		r, err := antennaPosition(g)
		if err != nil {
			bad++
			continue
		}
		out = append(out, r)
	}
	if bad > 0 {
		slog.Debug("SV3 bad GNSS blocks", "count", bad)
	}
	return out
}

// antennaPosition converts g to ECEF: the WGS84 position from latitude,
// longitude and hae, and the ENU std devs rotated to ECEF std devs
// (correlations dropped). A negative std dev is Sonardyne's -1.0 "not
// available" sentinel and nulls all three sigmas.
func antennaPosition(g *GNSSBlock) (datamodels.GNSSAntennaPositionRow, error) {
	if g.Latitude == nil || g.Longitude == nil || g.HAE == nil {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GNSS block without a position")
	}
	lat, lon, hae := *g.Latitude, *g.Longitude, *g.HAE
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 || math.Abs(hae) > 1000 || g.Q < 0 || g.Q > 9 {
		return datamodels.GNSSAntennaPositionRow{}, fmt.Errorf("GNSS block out of range: lat %v lon %v hae %v q %d", lat, lon, hae, g.Q)
	}
	t, err := UnixMicros(g.Time.Common)
	if err != nil {
		return datamodels.GNSSAntennaPositionRow{}, err
	}
	geo := coordinates.GeodeticCoord{Latitude: lat * math.Pi / 180, Longitude: lon * math.Pi / 180, Height: hae}
	ecef := coordinates.GeodeticToECEF(geo)
	r := datamodels.GNSSAntennaPositionRow{
		Time: time.UnixMicro(t).UTC(),
		X:    ecef[0], Y: ecef[1], Z: ecef[2],
		Q: g.Q,
	}
	if g.SDX >= 0 && g.SDY >= 0 && g.SDZ >= 0 {
		sig := enuSigmasToECEF(geo, g.SDX, g.SDY, g.SDZ)
		r.SigX, r.SigY, r.SigZ = &sig[0], &sig[1], &sig[2]
	}
	return r, nil
}

// enuSigmasToECEF rotates diagonal ENU std devs at geo to ECEF std devs.
// R (ECEF to ENU) is orthonormal, so ECEF = Rᵀ·ENU and the variance of ECEF
// axis i is Σⱼ R[j][i]²·var(ENU j).
func enuSigmasToECEF(geo coordinates.GeodeticCoord, sigE, sigN, sigU float64) [3]float64 {
	rot := coordinates.ComputeECEFToENUTransform(geo) // row-major, rows E, N, U
	variances := [3]float64{sigE * sigE, sigN * sigN, sigU * sigU}
	var out [3]float64
	for i := range 3 {
		var v float64
		for j := range 3 {
			r := rot[j*3+i]
			v += r * r * variances[j]
		}
		out[i] = math.Sqrt(v)
	}
	return out
}
