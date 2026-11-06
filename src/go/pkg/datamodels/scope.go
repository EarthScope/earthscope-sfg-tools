package datamodels

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Scope is the Network/Station/Campaign a file's rows belong to. File
// parsers take it and decide how it maps onto their table's columns.
type Scope struct {
	Network, Station, Campaign string
}

// ScopeFromRawDir reads the scope from a .../<network>/<station>/<campaign>/raw
// directory path.
func ScopeFromRawDir(dir string) Scope {
	campaign := filepath.Dir(filepath.Clean(dir))
	station := filepath.Dir(campaign)
	return Scope{
		Network:  filepath.Base(filepath.Dir(station)),
		Station:  filepath.Base(station),
		Campaign: filepath.Base(campaign),
	}
}

// EDIDSep separates the fields of a stream EDID. It is a character
// url.QueryEscape keeps, so stream_edid partition paths read as the EDID.
// No network, station, campaign, file type or log source contains it
// (CheckScope); a derived EDID's last field may (DerivedEDID).
const EDIDSep = "."

// StreamEDID is the EDID of the stream of rows from logSource in a file of
// type fileType for scope: network.station.campaign.fileType.logSource,
// e.g. cascadia-gorda.SLT1.2026_A_1126.NOV770.RANGECMP5B_1.
func StreamEDID(scope Scope, fileType, logSource string) string {
	return strings.Join([]string{scope.Network, scope.Station, scope.Campaign, fileType, logSource}, EDIDSep)
}

// SplitEDID splits a stream EDID into its five fields: network, station,
// campaign, file type or product, and log source or derived input (which
// may hold EDIDSep itself). ok is false if it has fewer.
func SplitEDID(edid string) (fields []string, ok bool) {
	fields = strings.SplitN(edid, EDIDSep, 5)
	return fields, len(fields) == 5
}

// CheckScope reports an error if a field of scope contains EDIDSep, which
// would shift the fields of its stream EDIDs.
func CheckScope(scope Scope) error {
	for _, f := range []string{scope.Network, scope.Station, scope.Campaign} {
		if strings.Contains(f, EDIDSep) {
			return fmt.Errorf("scope %q contains %q, the stream EDID separator", f, EDIDSep)
		}
	}
	return nil
}

// Log sources shared across file types, named for what the rows hold rather
// than the file they came from, so the same kind of stream has the same
// Log_Source whichever file type it is parsed from.
const (
	LogINTGRPLY   = "INTG-RPLY"  // interrogations paired with their replies: travel_times
	LogAHRS       = "AHRS"       // attitude_ahrs
	LogGNSSINS    = "GNSSINS"    // antenna positions from an INS solution: gnss_antenna_positions
	LogGNSSPSEUDO = "GNSSPSEUDO" // single-antenna pseudorange fixes, no INS: gnss_antenna_positions
	LogINSPVAA    = "INSPVAA"    // NovAtel INS position, velocity and attitude: inspva
)

// LogSource is the Log_Source value of rows from log type logType on one
// antenna (measurement source): the log type, with NovAtel's _<antenna>
// suffix for antennas after the primary, e.g. RANGECMP5B, RANGECMP5B_1.
func LogSource(logType string, antenna uint8) string {
	if antenna == 0 {
		return logType
	}
	return fmt.Sprintf("%s_%d", logType, antenna)
}
