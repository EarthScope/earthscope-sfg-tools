package datamodels

import (
	"slices"
	"strings"
)

// Level 1 and Level 2 tables: derived from Level 0 by downstream processing
// (PRIDE, the GNATSS smoother, shot building), not parsed from raw files. A
// derived row can combine several source logs. stream_edid names the
// derived stream (see DerivedEDID), so Table.Append replaces a rebuilt
// product the same way it replaces a re-ingested file.
//
// The levels follow plans/iceberg-gnatss-proposal.md: Level 1 holds the PPP
// and smoothed antenna tracks, and GNSS-A built with the Level 0 (SV3)
// antenna positions; Level 2 holds GNSS-A built with the smoothed track. The
// GNSS-A schemas are the same at both levels; the tables share a name and
// differ only by namespace.

// Derived products, the fourth field of a derived stream EDID.
const (
	ProductPRIDE    = "PRIDE"    // PRIDE PPP-AR kinematic positions
	ProductSmoothed = "SMOOTHED" // Kalman-smoothed antenna track
	ProductGNSSA    = "GNSSA"    // GNSS-A shots
)

// DerivedEDID is the stream EDID of a derived product built from input, the
// stream it was built from with its scope dropped: e.g. PRIDE from
// NOV000:RANGEA (or NOV000.RANGEA) is net.sta.camp.PRIDE.NOV000.RANGEA. The
// input is the last field, so it may hold EDIDSep (see SplitEDID).
func DerivedEDID(scope Scope, product, input string) string {
	return StreamEDID(scope, product, strings.ReplaceAll(input, ":", EDIDSep))
}

// GNSSAntennaPositionsPPP mirrors level1.GNSS_ANTENNA_POSITIONS_PPP.
var GNSSAntennaPositionsPPP = TableDef{
	Name:       "gnss_antenna_positions_ppp",
	Level:      1,
	TimeColumn: "time",
	Doc: "Antenna positions from PRIDE PPP-AR kinematic solutions of Level 0 " +
		"GNSS observations; lat/lon/height converted to ECEF at build. Input " +
		"is the gnss_observations stream, e.g. NOV000.RANGEA.",
	Schema: newSchema(append(append([]field{
		{Name: StreamEDIDName, Type: str, Required: true, Doc: "Derived stream EDID, network.station.campaign.product.input"},
	}, AntennaPositionColumns...),
		field{Name: "q", Type: int32Type, Doc: "GPS fix quality; null for PPP rows"},
		field{Name: "number_of_satellites", Type: int32Type, Doc: "Number of satellites used in the solution", Required: true},
		field{Name: "pdop", Type: float64Type, Doc: "Position dilution of precision", Required: true},
		field{Name: "wrms", Type: float64Type, Doc: "Weighted RMS of position residuals [m]", Required: true},
	)...),
	Checks: checks(AntennaPositionChecks, map[string]Check{
		"q":                    {Ge: bound(0), Le: bound(9)},
		"number_of_satellites": {Ge: bound(0)},
		"pdop":                 {Ge: bound(0)},
		"wrms":                 {Ge: bound(0)},
	}),
}

// GNSSAntennaPositionsSmoothed is Go-only: the workflows keep this track in
// memory between the smoother and the shot refinement.
var GNSSAntennaPositionsSmoothed = TableDef{
	Name:       "gnss_antenna_positions_smoothed",
	Level:      1,
	TimeColumn: "time",
	Doc: "Antenna track from the GNATSS Kalman filter and smoother over PPP " +
		"positions and INSPVA velocities, at the epochs of its inputs. Input " +
		"is the PPP stream, e.g. PRIDE.NOV000.RANGEA; filter settings are in " +
		"the snapshot summary.",
	Schema: newSchema(append(append([]field{
		{Name: StreamEDIDName, Type: str, Required: true, Doc: "Derived stream EDID, network.station.campaign.product.input"},
	}, AntennaPositionColumns...),
		field{Name: "rho_xy", Type: float64Type, Doc: "Position correlation X-Y"},
		field{Name: "rho_xz", Type: float64Type, Doc: "Position correlation X-Z"},
		field{Name: "rho_yz", Type: float64Type, Doc: "Position correlation Y-Z"},
	)...),
	Checks: checks(AntennaPositionChecks, map[string]Check{
		"rho_xy": {Ge: bound(-1), Le: bound(1)},
		"rho_xz": {Ge: bound(-1), Le: bound(1)},
		"rho_yz": {Ge: bound(-1), Le: bound(1)},
	}),
}

// garposSchema mirrors derived._GARPOS_COLUMNS.
var garposSchema = newSchema(
	field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Derived stream EDID, network.station.campaign.product.input"},
	field{Name: "SET", Type: str, Doc: "Name of observation subset, e.g. S01", Required: true},
	field{Name: "LN", Type: str, Doc: "Name of survey line, e.g. L01", Required: true},
	field{Name: "MT", Type: str, Doc: "ID of mirror transponder (must match the site-parameter file)", Required: true},
	field{Name: "TT", Type: float64Type, Doc: "Corrected travel time [s] (raw range - TAT - trigger delay)", Required: true},
	field{Name: "ResiTT", Type: float64Type, Doc: "Travel-time residual [ms] (populated by GARPOS)", Required: true},
	field{Name: "TakeOff", Type: float64Type, Doc: "Ray-path takeoff angle [deg] (populated by GARPOS)", Required: true},
	field{Name: "gamma", Type: float64Type, Doc: "Sound-speed variation correction term [m/s]", Required: true},
	field{Name: "flag", Type: boolType, Doc: "True = shot excluded from processing", Required: true},
	field{Name: "ST", Type: timestamptz, Doc: "Transmission time of acoustic signal, UTC", Required: true},
	field{Name: "ant_e0", Type: float64Type, Doc: "Antenna position east of array center at ST [m]", Required: true},
	field{Name: "ant_n0", Type: float64Type, Doc: "Antenna position north of array center at ST [m]", Required: true},
	field{Name: "ant_u0", Type: float64Type, Doc: "Antenna position up from array center at ST [m]", Required: true},
	field{Name: "head0", Type: float64Type, Doc: "Heading at ST [deg]", Required: true},
	field{Name: "pitch0", Type: float64Type, Doc: "Pitch at ST [deg]", Required: true},
	field{Name: "roll0", Type: float64Type, Doc: "Roll at ST [deg]", Required: true},
	field{Name: "RT", Type: timestamptz, Doc: "Reception time of acoustic signal, UTC", Required: true},
	field{Name: "ant_e1", Type: float64Type, Doc: "Antenna position east of array center at RT [m]", Required: true},
	field{Name: "ant_n1", Type: float64Type, Doc: "Antenna position north of array center at RT [m]", Required: true},
	field{Name: "ant_u1", Type: float64Type, Doc: "Antenna position up from array center at RT [m]", Required: true},
	field{Name: "head1", Type: float64Type, Doc: "Heading at RT [deg]", Required: true},
	field{Name: "pitch1", Type: float64Type, Doc: "Pitch at RT [deg]", Required: true},
	field{Name: "roll1", Type: float64Type, Doc: "Roll at RT [deg]", Required: true},
)

// garposChecks are the checks of garposSchema.
var garposChecks = map[string]Check{
	"TT":     {Ge: bound(0)},
	"head0":  {Ge: bound(0), Le: bound(360)},
	"pitch0": {Ge: bound(-90), Le: bound(90)},
	"roll0":  {Ge: bound(-180), Le: bound(180)},
	"head1":  {Ge: bound(0), Le: bound(360)},
	"pitch1": {Ge: bound(-90), Le: bound(90)},
	"roll1":  {Ge: bound(-180), Le: bound(180)},
}

// antennaEpochColumns are the community standard's antenna position and
// sigma columns at one epoch (suffix 0 = transmit, 1 = receive).
func antennaEpochColumns(suffix, label string) []field {
	var cols []field
	for _, a := range []string{"X", "Y", "Z"} {
		cols = append(cols, field{Name: "ant_" + a + suffix, Type: float64Type,
			Doc: "Antenna position " + a + ", ECEF, at " + label + " [m]"})
	}
	for _, a := range []string{"X", "Y", "Z"} {
		cols = append(cols, field{Name: "ant_sig" + a + suffix, Type: float64Type,
			Doc: "Antenna position std dev " + a + " at " + label + " [m]"})
	}
	return cols
}

// antennaEpochChecks are the checks of antennaEpochColumns(suffix, ...).
func antennaEpochChecks(suffix string) map[string]Check {
	out := map[string]Check{}
	for _, a := range []string{"X", "Y", "Z"} {
		out["ant_sig"+a+suffix] = Check{Ge: bound(0)}
	}
	return out
}

// attitudeColumns are the community standard's attitude columns at one
// epoch (suffix 0 = transmit, 1 = receive).
func attitudeColumns(suffix, label string) []field {
	return []field{
		{Name: "heading" + suffix, Type: float64Type, Doc: "Heading at " + label + " [deg]"},
		{Name: "pitch" + suffix, Type: float64Type, Doc: "Pitch at " + label + " [deg]"},
		{Name: "roll" + suffix, Type: float64Type, Doc: "Roll at " + label + " [deg]"},
	}
}

// attitudeChecks are the checks of attitudeColumns(suffix, ...).
func attitudeChecks(suffix string) map[string]Check {
	return map[string]Check{
		"heading" + suffix: {Ge: bound(0), Le: bound(360)},
		"pitch" + suffix:   {Ge: bound(-90), Le: bound(90)},
		"roll" + suffix:    {Ge: bound(-180), Le: bound(180)},
	}
}

// communitySchema mirrors derived._COMMUNITY_COLUMNS. X/Y/Z_transmit and
// _receive are transducer positions; ant_* are the antenna's.
var communitySchema = newSchema(slices.Concat(
	[]field{
		{Name: StreamEDIDName, Type: str, Required: true, Doc: "Derived stream EDID, network.station.campaign.product.input"},
		{Name: "MT_ID", Type: str, Doc: "ID of mirror transponder", Required: true},
		{Name: "TravelTime", Type: float64Type, Doc: "Corrected travel time [s] (raw range - TAT - trigger delay)", Required: true},
		{Name: "T_transmit", Type: timestamptz, Doc: "Transmission time, UTC", Required: true},
		{Name: "X_transmit", Type: float64Type, Doc: "Transducer position X at transmit, ECEF [m]", Required: true},
		{Name: "Y_transmit", Type: float64Type, Doc: "Transducer position Y at transmit, ECEF [m]", Required: true},
		{Name: "Z_transmit", Type: float64Type, Doc: "Transducer position Z at transmit, ECEF [m]", Required: true},
		{Name: "T_receive", Type: timestamptz, Doc: "Reception time, UTC", Required: true},
		{Name: "X_receive", Type: float64Type, Doc: "Transducer position X at receive, ECEF [m]", Required: true},
		{Name: "Y_receive", Type: float64Type, Doc: "Transducer position Y at receive, ECEF [m]", Required: true},
		{Name: "Z_receive", Type: float64Type, Doc: "Transducer position Z at receive, ECEF [m]", Required: true},
		{Name: "TDC_ID", Type: str, Doc: "Transducer ID"},
		{Name: "aSNR", Type: float64Type, Doc: "Signal-to-noise ratio [dB]"},
		{Name: "acc", Type: int32Type, Doc: "Cross-correlation accuracy [%]"},
		{Name: "dbV", Type: float32Type, Doc: "Decibel-voltage signal strength"},
		{Name: "quality_flag", Type: str, Doc: "Shot quality flag"},
	},
	antennaEpochColumns("0", "transmit"),
	antennaEpochColumns("1", "receive"),
	attitudeColumns("0", "transmit"),
	attitudeColumns("1", "receive"),
)...)

// communityChecks are the checks of communitySchema.
var communityChecks = checks(
	map[string]Check{
		"TravelTime": {Ge: bound(0)},
		"acc":        {Ge: bound(0), Le: bound(100)},
	},
	antennaEpochChecks("0"), antennaEpochChecks("1"),
	attitudeChecks("0"), attitudeChecks("1"),
)

// GNSS-A tables; the antenna positions come from Level 0
// gnss_antenna_positions (SV3 GNSS block) at Level 1, and from
// gnss_antenna_positions_smoothed at Level 2. Input is that positions stream.
var (
	GarposL1 = TableDef{Name: "garpos", Level: 1, TimeColumn: "ST", Schema: garposSchema, Checks: garposChecks,
		Doc: "GNSS-A in GARPOS format, antenna positions from Level 0 SV3 GNSS (local ENU about the array center)."}
	GarposL2 = TableDef{Name: "garpos", Level: 2, TimeColumn: "ST", Schema: garposSchema, Checks: garposChecks,
		Doc: "GNSS-A in GARPOS format, antenna positions from the Level 1 smoothed track (local ENU about the array center)."}
	CommunityStandardL1 = TableDef{Name: "community_standard", Level: 1, TimeColumn: "T_transmit", Schema: communitySchema, Checks: communityChecks,
		Doc: "GNSS-A community standard format, antenna positions from Level 0 SV3 GNSS (ECEF)."}
	CommunityStandardL2 = TableDef{Name: "community_standard", Level: 2, TimeColumn: "T_transmit", Schema: communitySchema, Checks: communityChecks,
		Doc: "GNSS-A community standard format, antenna positions from the Level 1 smoothed track (ECEF)."}
)

// Level1Tables lists every Level 1 table definition.
var Level1Tables = []TableDef{GNSSAntennaPositionsPPP, GNSSAntennaPositionsSmoothed, GarposL1, CommunityStandardL1}

// Level2Tables lists every Level 2 table definition.
var Level2Tables = []TableDef{GarposL2, CommunityStandardL2}
