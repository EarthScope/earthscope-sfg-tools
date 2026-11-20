package datamodels

import "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"

// Table refs: every (database, table) of SFG, declared once. Callers name a
// table with one of these rather than rebuilding it with Database.Ref.
// L0, L1 and L2 are the database's level; CommunityStandard is listed at
// Level 0 and Level 2.
var (
	L0TWTT                      = GNSSA_L0.Ref(TWTT)
	L0INSPVA                    = GNSSA_L0.Ref(INSPVA)
	L0INSSTDEV                  = GNSSA_L0.Ref(INSSTDEV)
	L0GNSSObs                   = GNSSA_L0.Ref(GNSSObs)
	L0AttitudeAHRS              = GNSSA_L0.Ref(AttitudeAHRS)
	L0SoundSpeed                = GNSSA_L0.Ref(SoundSpeed)
	L0GNSSAntennaPositions      = GNSSA_L0.Ref(GNSSAntennaPositions)
	L0GNSSSatellitesInView      = GNSSA_L0.Ref(GNSSSatellitesInView)
	L0GNSSDOP                   = GNSSA_L0.Ref(GNSSDOP)
	L0GNSSCourseSpeed           = GNSSA_L0.Ref(GNSSCourseSpeed)
	L0TransceiverInterrogations = GNSSA_L0.Ref(TransceiverInterrogations)
	L0TransceiverRanges         = GNSSA_L0.Ref(TransceiverRanges)
	L0TransceiverSettings       = GNSSA_L0.Ref(TransceiverSettings)
	L0CommunityStandard         = GNSSA_L0.Ref(CommunityStandard)

	L1GNSSAntennaPositionsPPP = GNSSA_L1.Ref(GNSSAntennaPositionsPPP)

	L2Garpos            = GNSSA_L2.Ref(Garpos)
	L2CommunityStandard = GNSSA_L2.Ref(CommunityStandard)
)

// Refs lists every ref var above, in database order. CheckDefinitions
// checks that it matches SFG.Refs().
var Refs = []catalog.TableRef{
	L0TWTT, L0INSPVA, L0INSSTDEV, L0GNSSObs, L0AttitudeAHRS, L0SoundSpeed,
	L0GNSSAntennaPositions, L0GNSSSatellitesInView, L0GNSSDOP, L0GNSSCourseSpeed,
	L0TransceiverInterrogations, L0TransceiverRanges, L0TransceiverSettings,
	L0CommunityStandard,
	L1GNSSAntennaPositionsPPP,
	L2Garpos, L2CommunityStandard,
}
