package novatel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/tablebuild"
)

// INSPVA is one INSPVAA log. Time is the log header time (GPS time scale).
type INSPVA struct {
	Time                 time.Time
	Lat, Lon, Alt        float64 // [deg], [deg], [m]
	North, East, Up      float64 // [m/s]
	Roll, Pitch, Azimuth float64 // [deg]
}

// INSSTDEV is one INSSTDEVA log. Time is the log header time (GPS time scale).
type INSSTDEV struct {
	Time                          time.Time
	LatSig, LonSig, AltSig        float64 // [m]
	NorthSig, EastSig, UpSig      float64 // [m/s]
	RollSig, PitchSig, AzimuthSig float64 // [deg]
	ExtSolStat, TimeSinceUpdate   string
}

// parseFloats parses fields[from:from+len(dst)] into dst.
func parseFloats(log string, fields []string, from int, dst ...*float64) error {
	for i, d := range dst {
		v, err := strconv.ParseFloat(strings.TrimSpace(fields[from+i]), 64)
		if err != nil {
			return fmt.Errorf("%s field %d: %w", log, from+i, err)
		}
		*d = v
	}
	return nil
}

// ParseINSPVA decodes an INSPVAA data section:
// week,seconds,lat,lon,height,north,east,up,roll,pitch,azimuth,status
func ParseINSPVA(data string, t time.Time) (INSPVA, error) {
	f := strings.Split(data, ",")
	if len(f) < 12 {
		return INSPVA{}, fmt.Errorf("INSPVAA: %d fields, want 12", len(f))
	}
	r := INSPVA{Time: t}
	err := parseFloats("INSPVAA", f, 2, &r.Lat, &r.Lon, &r.Alt, &r.North, &r.East, &r.Up, &r.Roll, &r.Pitch, &r.Azimuth)
	return r, err
}

// ParseINSSTDEV decodes an INSSTDEVA data section:
// lat_sig,lon_sig,height_sig,north_sig,east_sig,up_sig,roll_sig,pitch_sig,azimuth_sig,ext_sol_stat,time_since_update,...
func ParseINSSTDEV(data string, t time.Time) (INSSTDEV, error) {
	f := strings.Split(data, ",")
	if len(f) < 9 {
		return INSSTDEV{}, fmt.Errorf("INSSTDEVA: %d fields, want at least 9", len(f))
	}
	r := INSSTDEV{Time: t}
	if err := parseFloats("INSSTDEVA", f, 0, &r.LatSig, &r.LonSig, &r.AltSig, &r.NorthSig, &r.EastSig, &r.UpSig,
		&r.RollSig, &r.PitchSig, &r.AzimuthSig); err != nil {
		return r, err
	}
	if len(f) > 10 {
		r.ExtSolStat = strings.TrimSpace(f[9])
		r.TimeSinceUpdate = strings.TrimSpace(f[10])
	}
	return r, nil
}

func utcMicros(gps time.Time) arrow.Timestamp {
	return arrow.Timestamp(gnsstime.GPSTimeToUTC(gps).UnixMicro())
}

// INSPVATable converts records to INSPVA rows; times become UTC.
func INSPVATable(records []INSPVA, scope schemas.Scope, logSource string) (arrow.Table, error) {
	b, err := tablebuild.New(schemas.INSPVA)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	sc := b.Scope(scope, logSource)
	tm := tablebuild.Col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat", "lon", "alt", "north", "east", "up", "roll", "pitch", "heading"} {
		cols = append(cols, tablebuild.Col[*array.Float64Builder](b, name))
	}
	for _, r := range records {
		sc.Append()
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.Lat, r.Lon, r.Alt, r.North, r.East, r.Up, r.Roll, r.Pitch, r.Azimuth} {
			cols[i].Append(v)
		}
	}
	return b.Table()
}

// INSSTDEVTable converts records to INSSTDEV rows; times become UTC. The
// cov_* columns hold the logged attitude std devs (see schemas.INSSTDEV).
func INSSTDEVTable(records []INSSTDEV, scope schemas.Scope, logSource string) (arrow.Table, error) {
	b, err := tablebuild.New(schemas.INSSTDEV)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	sc := b.Scope(scope, logSource)
	tm := tablebuild.Col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat_sig", "lon_sig", "alt_sig", "north_sig", "east_sig", "up_sig", "cov_rr", "cov_pp", "cov_hh"} {
		cols = append(cols, tablebuild.Col[*array.Float64Builder](b, name))
	}
	extSolStat := tablebuild.Col[*array.StringBuilder](b, "ext_sol_stat")
	sinceUpdate := tablebuild.Col[*array.StringBuilder](b, "time_since_update")
	optional := func(col *array.StringBuilder, v string) {
		if v == "" {
			col.AppendNull()
		} else {
			col.Append(v)
		}
	}
	for _, r := range records {
		sc.Append()
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.LatSig, r.LonSig, r.AltSig, r.NorthSig, r.EastSig, r.UpSig, r.RollSig, r.PitchSig, r.AzimuthSig} {
			cols[i].Append(v)
		}
		optional(extSolStat, r.ExtSolStat)
		optional(sinceUpdate, r.TimeSinceUpdate)
	}
	return b.Table()
}
