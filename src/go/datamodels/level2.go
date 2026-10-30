package datamodels

import (
	"slices"
	"strings"
)

// Level 2 and Level 3 tables: derived from Level 1 by downstream processing
// (PRIDE, the GNATSS smoother, shot building), not parsed from raw files. A
// derived row can combine several source logs, so these tables carry the
// scope columns but no Log_Source. stream_edid
// names the derived stream (see DerivedEDID), so Catalog.Append replaces a
// rebuilt product the same way it replaces a re-ingested file.
//
// The levels follow plans/iceberg-gnatss-proposal.md: Level 2 holds the PPP
// and smoothed antenna tracks, and GNSS-A built with the Level 1 (SV3)
// antenna positions; Level 3 holds GNSS-A built with the smoothed track. The
// GNSS-A schemas are the same at both levels; the tables share a name and
// differ only by namespace.

// DerivedScope holds the columns every derived table starts with.
var DerivedScope = Level1Scope[:3]

// derivedStreamColumn is the last column of every derived table. Unlike
// Level 1's StreamEDIDColumn it is required: the tables are new.
var derivedStreamColumn = Column{Name: StreamEDIDColumn.Name, Type: str, Required: true,
	Doc: "Derived stream EDID, network:station:campaign:product:input"}

// derived wraps cols in the scope columns and the derived stream column.
func derived(cols ...Column) []Column {
	return append(append(append([]Column{}, DerivedScope...), cols...), derivedStreamColumn)
}

// Derived products, the fourth field of a derived stream EDID.
const (
	ProductPRIDE    = "PRIDE"    // PRIDE PPP-AR kinematic positions
	ProductSmoothed = "SMOOTHED" // Kalman-smoothed antenna track
	ProductGNSSA    = "GNSSA"    // GNSS-A shots
)

// DerivedEDID is the stream EDID of a derived product built from input, the
// stream it was built from with its scope dropped: e.g. PRIDE from
// NOV000:RANGEA is net:sta:camp:PRIDE:NOV000.RANGEA. Colons in input become
// dots so the EDID keeps five fields.
func DerivedEDID(scope Scope, product, input string) string {
	return StreamEDID(scope, product, strings.ReplaceAll(input, ":", "."))
}

// GNSSAntennaPositionsPPP mirrors level2.GNSS_ANTENNA_POSITIONS_PPP.
var GNSSAntennaPositionsPPP = TableDef{
	Name:       "gnss_antenna_positions_ppp",
	Level:      2,
	TimeColumn: "time",
	Doc: "Antenna positions from PRIDE PPP-AR kinematic solutions of Level 1 " +
		"GNSS observations; lat/lon/height converted to ECEF at build. Input " +
		"is the gnss_observations stream, e.g. NOV000.RANGEA.",
	Columns: derived(append(append([]Column{}, AntennaPositionColumns...),
		Column{Name: "q", Type: int32Type, Doc: "GPS fix quality; null for PPP rows", Ge: bound(0), Le: bound(9)},
		Column{Name: "number_of_satellites", Type: int32Type, Doc: "Number of satellites used in the solution", Required: true, Ge: bound(0)},
		Column{Name: "pdop", Type: float64Type, Doc: "Position dilution of precision", Required: true, Ge: bound(0)},
		Column{Name: "wrms", Type: float64Type, Doc: "Weighted RMS of position residuals [m]", Required: true, Ge: bound(0)},
	)...),
}

// GNSSAntennaPositionsSmoothed is Go-only: the workflows keep this track in
// memory between the smoother and the shot refinement.
var GNSSAntennaPositionsSmoothed = TableDef{
	Name:       "gnss_antenna_positions_smoothed",
	Level:      2,
	TimeColumn: "time",
	Doc: "Antenna track from the GNATSS Kalman filter and smoother over PPP " +
		"positions and INSPVA velocities, at the epochs of its inputs. Input " +
		"is the PPP stream, e.g. PRIDE.NOV000.RANGEA; filter settings are in " +
		"the snapshot summary.",
	Columns: derived(append(append([]Column{}, AntennaPositionColumns...),
		Column{Name: "rho_xy", Type: float64Type, Doc: "Position correlation X-Y", Ge: bound(-1), Le: bound(1)},
		Column{Name: "rho_xz", Type: float64Type, Doc: "Position correlation X-Z", Ge: bound(-1), Le: bound(1)},
		Column{Name: "rho_yz", Type: float64Type, Doc: "Position correlation Y-Z", Ge: bound(-1), Le: bound(1)},
	)...),
}

// garposColumns mirrors derived._GARPOS_COLUMNS.
var garposColumns = derived(
	Column{Name: "SET", Type: str, Doc: "Name of observation subset, e.g. S01", Required: true},
	Column{Name: "LN", Type: str, Doc: "Name of survey line, e.g. L01", Required: true},
	Column{Name: "MT", Type: str, Doc: "ID of mirror transponder (must match the site-parameter file)", Required: true},
	Column{Name: "TT", Type: float64Type, Doc: "Corrected travel time [s] (raw range - TAT - trigger delay)", Required: true, Ge: bound(0)},
	Column{Name: "ResiTT", Type: float64Type, Doc: "Travel-time residual [ms] (populated by GARPOS)", Required: true},
	Column{Name: "TakeOff", Type: float64Type, Doc: "Ray-path takeoff angle [deg] (populated by GARPOS)", Required: true},
	Column{Name: "gamma", Type: float64Type, Doc: "Sound-speed variation correction term [m/s]", Required: true},
	Column{Name: "flag", Type: boolType, Doc: "True = shot excluded from processing", Required: true},
	Column{Name: "ST", Type: timestamptz, Doc: "Transmission time of acoustic signal, UTC", Required: true},
	Column{Name: "ant_e0", Type: float64Type, Doc: "Antenna position east of array center at ST [m]", Required: true},
	Column{Name: "ant_n0", Type: float64Type, Doc: "Antenna position north of array center at ST [m]", Required: true},
	Column{Name: "ant_u0", Type: float64Type, Doc: "Antenna position up from array center at ST [m]", Required: true},
	Column{Name: "head0", Type: float64Type, Doc: "Heading at ST [deg]", Required: true, Ge: bound(0), Le: bound(360)},
	Column{Name: "pitch0", Type: float64Type, Doc: "Pitch at ST [deg]", Required: true, Ge: bound(-90), Le: bound(90)},
	Column{Name: "roll0", Type: float64Type, Doc: "Roll at ST [deg]", Required: true, Ge: bound(-180), Le: bound(180)},
	Column{Name: "RT", Type: timestamptz, Doc: "Reception time of acoustic signal, UTC", Required: true},
	Column{Name: "ant_e1", Type: float64Type, Doc: "Antenna position east of array center at RT [m]", Required: true},
	Column{Name: "ant_n1", Type: float64Type, Doc: "Antenna position north of array center at RT [m]", Required: true},
	Column{Name: "ant_u1", Type: float64Type, Doc: "Antenna position up from array center at RT [m]", Required: true},
	Column{Name: "head1", Type: float64Type, Doc: "Heading at RT [deg]", Required: true, Ge: bound(0), Le: bound(360)},
	Column{Name: "pitch1", Type: float64Type, Doc: "Pitch at RT [deg]", Required: true, Ge: bound(-90), Le: bound(90)},
	Column{Name: "roll1", Type: float64Type, Doc: "Roll at RT [deg]", Required: true, Ge: bound(-180), Le: bound(180)},
)

// antennaEpochColumns are the community standard's antenna position and
// sigma columns at one epoch (suffix 0 = transmit, 1 = receive).
func antennaEpochColumns(suffix, label string) []Column {
	var cols []Column
	for _, a := range []string{"X", "Y", "Z"} {
		cols = append(cols, Column{Name: "ant_" + a + suffix, Type: float64Type,
			Doc: "Antenna position " + a + ", ECEF, at " + label + " [m]"})
	}
	for _, a := range []string{"X", "Y", "Z"} {
		cols = append(cols, Column{Name: "ant_sig" + a + suffix, Type: float64Type, Ge: bound(0),
			Doc: "Antenna position std dev " + a + " at " + label + " [m]"})
	}
	return cols
}

func attitudeColumns(suffix, label string) []Column {
	return []Column{
		{Name: "heading" + suffix, Type: float64Type, Doc: "Heading at " + label + " [deg]", Ge: bound(0), Le: bound(360)},
		{Name: "pitch" + suffix, Type: float64Type, Doc: "Pitch at " + label + " [deg]", Ge: bound(-90), Le: bound(90)},
		{Name: "roll" + suffix, Type: float64Type, Doc: "Roll at " + label + " [deg]", Ge: bound(-180), Le: bound(180)},
	}
}

// communityColumns mirrors derived._COMMUNITY_COLUMNS. X/Y/Z_transmit and
// _receive are transducer positions; ant_* are the antenna's.
var communityColumns = derived(slices.Concat(
	[]Column{
		{Name: "MT_ID", Type: str, Doc: "ID of mirror transponder", Required: true},
		{Name: "TravelTime", Type: float64Type, Doc: "Corrected travel time [s] (raw range - TAT - trigger delay)", Required: true, Ge: bound(0)},
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
		{Name: "acc", Type: int32Type, Doc: "Cross-correlation accuracy [%]", Ge: bound(0), Le: bound(100)},
		{Name: "dbV", Type: float32Type, Doc: "Decibel-voltage signal strength"},
		{Name: "quality_flag", Type: str, Doc: "Shot quality flag"},
	},
	antennaEpochColumns("0", "transmit"),
	antennaEpochColumns("1", "receive"),
	attitudeColumns("0", "transmit"),
	attitudeColumns("1", "receive"),
)...)

// GNSS-A tables; the antenna positions come from Level 1
// gnss_antenna_positions (SV3 GNSS block) at Level 2, and from
// gnss_antenna_positions_smoothed at Level 3. Input is that positions stream.
var (
	GarposL2 = TableDef{Name: "garpos", Level: 2, TimeColumn: "ST", Columns: garposColumns,
		Doc: "GNSS-A in GARPOS format, antenna positions from Level 1 SV3 GNSS (local ENU about the array center)."}
	GarposL3 = TableDef{Name: "garpos", Level: 3, TimeColumn: "ST", Columns: garposColumns,
		Doc: "GNSS-A in GARPOS format, antenna positions from the Level 2 smoothed track (local ENU about the array center)."}
	CommunityStandardL2 = TableDef{Name: "community_standard", Level: 2, TimeColumn: "T_transmit", Columns: communityColumns,
		Doc: "GNSS-A community standard format, antenna positions from Level 1 SV3 GNSS (ECEF)."}
	CommunityStandardL3 = TableDef{Name: "community_standard", Level: 3, TimeColumn: "T_transmit", Columns: communityColumns,
		Doc: "GNSS-A community standard format, antenna positions from the Level 2 smoothed track (ECEF)."}
)

// Level2Tables lists every Level 2 table definition.
var Level2Tables = []TableDef{GNSSAntennaPositionsPPP, GNSSAntennaPositionsSmoothed, GarposL2, CommunityStandardL2}

// Level3Tables lists every Level 3 table definition.
var Level3Tables = []TableDef{GarposL3, CommunityStandardL3}
