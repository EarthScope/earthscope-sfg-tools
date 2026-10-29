package novatel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

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
func ParseINSPVA(data string, t time.Time) (datamodels.INSPVARow, error) {
	f := strings.Split(data, ",")
	if len(f) < 12 {
		return datamodels.INSPVARow{}, fmt.Errorf("INSPVAA: %d fields, want 12", len(f))
	}
	r := datamodels.INSPVARow{Time: t, INSStatus: nonEmpty(strings.TrimSpace(f[11]))}
	err := parseFloats("INSPVAA", f, 2, &r.Lat, &r.Lon, &r.Alt, &r.North, &r.East, &r.Up, &r.Roll, &r.Pitch, &r.Azimuth)
	return r, err
}

// ParseINSSTDEV decodes an INSSTDEVA data section:
// lat_sig,lon_sig,height_sig,north_sig,east_sig,up_sig,roll_sig,pitch_sig,azimuth_sig,ext_sol_stat,time_since_update,...
func ParseINSSTDEV(data string, t time.Time) (datamodels.INSSTDEVRow, error) {
	f := strings.Split(data, ",")
	if len(f) < 9 {
		return datamodels.INSSTDEVRow{}, fmt.Errorf("INSSTDEVA: %d fields, want at least 9", len(f))
	}
	r := datamodels.INSSTDEVRow{Time: t}
	if err := parseFloats("INSSTDEVA", f, 0, &r.LatSig, &r.LonSig, &r.AltSig, &r.NorthSig, &r.EastSig, &r.UpSig,
		&r.RollSig, &r.PitchSig, &r.AzimuthSig); err != nil {
		return r, err
	}
	if len(f) > 10 {
		r.ExtSolStat = nonEmpty(strings.TrimSpace(f[9]))
		r.TimeSinceUpdate = nonEmpty(strings.TrimSpace(f[10]))
	}
	return r, nil
}

// nonEmpty is &s, or nil when s is empty.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
