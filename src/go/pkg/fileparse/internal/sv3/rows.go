package sv3

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"gitlab.com/earthscope/gnsstools/core/coordinates"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/novatel"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/rows"
)

// A pair is a range event (a transponder reply) and the most recent
// interrogation (ping) before it, with both times in UNIX microseconds.
type pair struct {
	ping, reply *Event
	sent, recv  int64
}

// pairs pairs each range event with the most recent preceding
// interrogation. A zero range (no reply detected), a reply with no ping
// before it, and unreadable times are skipped and counted in the log.
func pairs(events []Event) []pair {
	var (
		out                       []pair
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
		out = append(out, pair{ping: ping, reply: e, sent: sent, recv: recv})
	}
	slog.Debug("SV3 ping/reply pairs", "pairs", len(out), "no_interrogation", noPing, "no_reply", noReply, "bad_times", badTimes)
	return out
}

// TravelTimes is one travel_times row per ping/reply pair (see pairs), with
// the raw range as logged.
func TravelTimes(events []Event) []rows.TWTTRow {
	var out []rows.TWTTRow
	for _, p := range pairs(events) {
		r := p.reply.Range
		snr, tat := float64(r.Diag.SNR), r.TAT/1000
		out = append(out, rows.TWTTRow{
			MTID:       r.CN,
			TTransmit:  time.UnixMicro(p.sent).UTC(),
			TReceive:   time.UnixMicro(p.recv).UTC(),
			TravelTime: r.Range,
			DBV:        float32(r.Diag.DBV),
			Acc:        int32(math.Round(float64(r.Diag.XC))),
			ASNR:       &snr,
			TAT:        &tat,
		})
	}
	return out
}

// TriggerDelay is the SV3 hardware trigger delay [s], part of every logged
// range (Python TRIGGER_DELAY_SV3).
const TriggerDelay = 0.13

// Shot pairing rules, as the Python shot builder's (SV3PairingRules): a
// reply more than maxRoundTrip after its ping, or whose logged time is not
// its ping's plus the range less TriggerDelay (within returnTolerance), is
// not that ping's reply.
const (
	maxRoundTrip    = 15 * time.Second
	returnTolerance = time.Microsecond
)

// Shots is one community_standard row per ping/reply pair (see pairs) that
// passes the shot pairing rules. The travel time is corrected: range - TAT -
// TriggerDelay. The antenna position and std devs at each end come from
// that event's GNSS block, in ECEF (as gnss_antenna_positions); a pair
// without a usable GNSS block at either end is no shot, as in the Python
// builder. The attitude comes from the NOV_INS block, and is null where it
// was not logged (2024 DFOP00 files), where the Python builder drops the
// shot. Transducer positions stay null: they need lever arms.
func Shots(events []Event) []rows.CommunityStandardRow {
	var (
		out                       []rows.CommunityStandardRow
		tooLong, mismatch, noGNSS int
	)
	for _, p := range pairs(events) {
		r := p.reply.Range
		rtt := time.Duration(p.recv-p.sent) * time.Microsecond
		if rtt < 0 || rtt > maxRoundTrip {
			tooLong++
			continue
		}
		want := time.Duration(math.Round((r.Range-TriggerDelay)*1e6)) * time.Microsecond
		if d := rtt - want; d > returnTolerance || d < -returnTolerance {
			mismatch++
			continue
		}
		ping, reply := antennaEpoch(p.ping), antennaEpoch(p.reply)
		if ping.X == nil || reply.X == nil {
			noGNSS++
			continue
		}
		snr, acc, dbv := float64(r.Diag.SNR), int32(math.Round(float64(r.Diag.XC))), float32(r.Diag.DBV)
		out = append(out, rows.CommunityStandardRow{
			MTID:       r.CN,
			TravelTime: r.Range - r.TAT/1000 - TriggerDelay,
			TTransmit:  time.UnixMicro(p.sent).UTC(),
			TReceive:   time.UnixMicro(p.recv).UTC(),
			ASNR:       &snr,
			Acc:        &acc,
			DBV:        &dbv,
			Transmit:   ping,
			Receive:    reply,
		})
	}
	slog.Debug("SV3 shots", "shots", len(out), "round_trip_too_long", tooLong, "return_time_mismatch", mismatch,
		"no_gnss_position", noGNSS)
	return out
}

// antennaEpoch is e's antenna state: the GNSS block's position and std devs
// in ECEF, and the NOV_INS attitude. Missing or bad blocks leave nils.
func antennaEpoch(e *Event) rows.AntennaEpoch {
	var out rows.AntennaEpoch
	if g, err := e.GNSS(); err == nil && g != nil {
		if pos, err := antennaPosition(g); err == nil {
			out.X, out.Y, out.Z = &pos.X, &pos.Y, &pos.Z
			out.SigX, out.SigY, out.SigZ = pos.SigX, pos.SigY, pos.SigZ
		}
	}
	if ins, err := e.INS(); err == nil && ins != nil {
		h, pt, r := ins.H, ins.P, ins.R
		out.Heading, out.Pitch, out.Roll = &h, &pt, &r
	}
	return out
}

// Attitudes is one attitude_ahrs row per distinct AHRS block (consecutive
// events usually repeat the same block).
func Attitudes(events []Event) []rows.AttitudeAHRSRow {
	var (
		out  []rows.AttitudeAHRSRow
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
		r := rows.AttitudeAHRSRow{
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

// INSPVA is one inspva row per distinct NOV_INS block, keyed by its GPS time
// (events repeat the current block). Blocks with no time fix (time status
// UNKNOWN) are skipped.
func INSPVA(events []Event) []rows.INSPVARow {
	var (
		out            []rows.INSPVARow
		last           float64
		bad, unknownTS int
	)
	for i := range events {
		b, err := events[i].INS()
		if err != nil {
			bad++
			continue
		}
		if b == nil || b.GPST == last {
			continue
		}
		last = b.GPST
		if b.TimeStatus == novatel.TimeStatusUnknown {
			unknownTS++
			continue
		}
		ins, ts := novatel.INSStatusName(b.SolutionType), novatel.TimeStatusName(b.TimeStatus)
		out = append(out, rows.INSPVARow{
			Time: gnsstime.GPSEpoch().Add(time.Duration(math.Round(b.GPST*1e6)) * time.Microsecond),
			Lat:  b.Latitude, Lon: b.Longitude, Alt: b.HAE,
			North: b.VX, East: b.VY, Up: b.VZ,
			Roll: b.R, Pitch: b.P, Azimuth: b.H,
			INSStatus: &ins, TimeStatus: &ts,
		})
	}
	if bad > 0 || unknownTS > 0 {
		slog.Debug("SV3 skipped NOV_INS blocks", "bad", bad, "unknown_time", unknownTS)
	}
	return out
}

// AntennaPositions is one gnss_antenna_positions row per distinct GNSS block,
// keyed by its fix time (events repeat the current block); the first block
// seen for a time wins. Blocks without a usable position, time or fix
// quality are skipped and counted.
func AntennaPositions(events []Event) []rows.GNSSAntennaPositionRow {
	var (
		out  []rows.GNSSAntennaPositionRow
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
func antennaPosition(g *GNSSBlock) (rows.GNSSAntennaPositionRow, error) {
	if g.Latitude == nil || g.Longitude == nil || g.HAE == nil {
		return rows.GNSSAntennaPositionRow{}, fmt.Errorf("GNSS block without a position")
	}
	lat, lon, hae := *g.Latitude, *g.Longitude, *g.HAE
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 || math.Abs(hae) > 1000 || g.Q < 0 || g.Q > 9 {
		return rows.GNSSAntennaPositionRow{}, fmt.Errorf("GNSS block out of range: lat %v lon %v hae %v q %d", lat, lon, hae, g.Q)
	}
	t, err := UnixMicros(g.Time.Common)
	if err != nil {
		return rows.GNSSAntennaPositionRow{}, err
	}
	geo := coordinates.GeodeticCoord{Latitude: lat * math.Pi / 180, Longitude: lon * math.Pi / 180, Height: hae}
	ecef := coordinates.GeodeticToECEF(geo)
	r := rows.GNSSAntennaPositionRow{
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
