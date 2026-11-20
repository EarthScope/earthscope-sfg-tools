package datamodels

import (
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// INSPVARow is one INSPVAA log. Time is the log header time (GPS time scale).
type INSPVARow struct {
	LogSource            edid.LogSource
	Time                 time.Time
	Lat, Lon, Alt        float64 // [deg], [deg], [m]
	North, East, Up      float64 // [m/s]
	Roll, Pitch, Azimuth float64 // [deg]
	INSStatus            *string // e.g. INS_SOLUTION_GOOD
	TimeStatus           *string // log header time status, e.g. FINESTEERING
}

// INSSTDEVRow is one INSSTDEVA log. Time is the log header time (GPS time scale).
type INSSTDEVRow struct {
	LogSource                     edid.LogSource
	Time                          time.Time
	LatSig, LonSig, AltSig        float64 // [m]
	NorthSig, EastSig, UpSig      float64 // [m/s]
	RollSig, PitchSig, AzimuthSig float64 // [deg]
	ExtSolStat, TimeSinceUpdate   *string
	TimeStatus                    *string // log header time status, e.g. FINESTEERING
}

// BuildINSPVA converts rows to inspva rows; times become UTC.
func BuildINSPVA(records []INSPVARow, scope edid.Scope) (arrow.Table, error) {
	b, err := newBuilder(INSPVA)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	edidCol := col[*array.StringBuilder](b, StreamEDIDName)
	tm := col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat", "lon", "alt", "north", "east", "up", "roll", "pitch", "heading"} {
		cols = append(cols, col[*array.Float64Builder](b, name))
	}
	insStatus := col[*array.StringBuilder](b, "ins_status")
	timeStatus := col[*array.StringBuilder](b, "time_status")
	for _, r := range records {
		edidCol.Append(edid.Stream(scope, r.LogSource))
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.Lat, r.Lon, r.Alt, r.North, r.East, r.Up, r.Roll, r.Pitch, r.Azimuth} {
			cols[i].Append(v)
		}
		appendOpt(insStatus, r.INSStatus)
		appendOpt(timeStatus, r.TimeStatus)
	}
	return b.Table()
}

// --- INSSTDEV rows ---

// BuildINSSTDEV converts rows to insstdev rows; times become UTC. The cov_* columns hold the logged attitude std devs (see INSSTDEV).
func BuildINSSTDEV(records []INSSTDEVRow, scope edid.Scope) (arrow.Table, error) {
	b, err := newBuilder(INSSTDEV)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	edidCol := col[*array.StringBuilder](b, StreamEDIDName)
	tm := col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat_sig", "lon_sig", "alt_sig", "north_sig", "east_sig", "up_sig", "cov_rr", "cov_pp", "cov_hh"} {
		cols = append(cols, col[*array.Float64Builder](b, name))
	}
	extSolStat := col[*array.StringBuilder](b, "ext_sol_stat")
	sinceUpdate := col[*array.StringBuilder](b, "time_since_update")
	timeStatus := col[*array.StringBuilder](b, "time_status")
	for _, r := range records {
		edidCol.Append(edid.Stream(scope, r.LogSource))
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.LatSig, r.LonSig, r.AltSig, r.NorthSig, r.EastSig, r.UpSig, r.RollSig, r.PitchSig, r.AzimuthSig} {
			cols[i].Append(v)
		}
		appendOpt(extSolStat, r.ExtSolStat)
		appendOpt(sinceUpdate, r.TimeSinceUpdate)
		appendOpt(timeStatus, r.TimeStatus)
	}
	return b.Table()
}
