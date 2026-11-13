package datamodels

import "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"

// The SFG catalog's shape (see catalog.CatalogDef): one database per
// level. gnssa_l0 holds parsed records, gnssa_l1 and gnssa_l2 products built
// from them. A table definition carries no level, so one can be listed in
// several databases (community_standard is in all three).

// The SFG databases, one per level.
var (
	GNSSA_L0 = catalog.DatabaseDef{
		Name: "gnssa_l0",
		Doc:  "Level 0: records parsed from raw files, one table per record type, and GNSS-A shots parsed from DFOP00.",
		Tables: []catalog.TableDef{
			TWTT,
			INSPVA,
			INSSTDEV,
			GNSSObs,
			AttitudeAHRS,
			SoundSpeed,
			GNSSAntennaPositions,
			GNSSSatellitesInView,
			GNSSDOP,
			GNSSCourseSpeed,
			TransceiverInterrogations,
			TransceiverRanges,
			TransceiverSettings,
			CommunityStandard,
		},
	}
	GNSSA_L1 = catalog.DatabaseDef{
		Name:   "gnssa_l1",
		Doc:    "Level 1: PPP and smoothed antenna tracks, and GNSS-A built with Level 0 antenna positions.",
		Tables: []catalog.TableDef{GNSSAntennaPositionsPPP},
	}
	GNSSA_L2 = catalog.DatabaseDef{
		Name:   "gnssa_l2",
		Doc:    "Level 2: GNSS-A built with the Level 1 smoothed antenna track.",
		Tables: []catalog.TableDef{Garpos, CommunityStandard},
	}
)

// SFG is the seafloor geodesy catalog: every database and table.
var SFG = catalog.CatalogDef{
	Name:      "sfg",
	Doc:       "Seafloor geodesy (GNSS-A) data, Level 0 to 2.",
	Databases: []catalog.DatabaseDef{GNSSA_L0, GNSSA_L1, GNSSA_L2},
}
