// Package edid builds and splits stream EDIDs, the identity of every row in
// the SFG tables (their stream_edid column):
//
//	network.station.campaign.file_type.log_source   (Stream: parsed rows)
//	network.station.campaign.product.input          (Derived: built products)
//
// A Scope is the network, station and campaign a file belongs to. The
// log-source constants name streams shared across file types; the product
// constants name derived products. Table definitions (package datamodels)
// know only the column name, not how its values are made.
package edid

import (
	"fmt"
	"strings"
)

// Scope is the Network/Station/Campaign a file's rows belong to. File
// parsers take it and decide how it maps onto their table's columns.
type Scope struct {
	Network, Station, Campaign string
}

// Sep separates the fields of a stream EDID. It is a character
// url.QueryEscape keeps, so stream_edid partition paths read as the EDID.
// No network, station, campaign, file type or log source contains it
// (CheckScope); a derived EDID's last field may (Derived).
const Sep = "."

// LogSource names a stream within a scope: the file type and the log source
// name (INSPVAA, RANGECMP5B_1) of the rows. Neither contains Sep.
type LogSource struct{ FileType, Name string }

// AllLogSources is the zero LogSource, which names no stream: a selection
// uses it to mean every log source.
var AllLogSources = LogSource{}

// IsAll reports whether s is AllLogSources.
func (s LogSource) IsAll() bool { return s == AllLogSources }

// String is "<FileType>.<Name>".
func (s LogSource) String() string { return s.FileType + Sep + s.Name }

// ParseLogSource is the inverse of String; it requires exactly one Sep.
func ParseLogSource(s string) (LogSource, error) {
	parts := strings.Split(s, Sep)
	if len(parts) != 2 {
		return LogSource{}, fmt.Errorf("log source %q: want <file_type>%s<name>", s, Sep)
	}
	return LogSource{FileType: parts[0], Name: parts[1]}, nil
}

// Stream is the EDID of the stream of rows from src for scope:
// network.station.campaign.fileType.name,
// e.g. cascadia-gorda.SLT1.2026_A_1126.NOV770.RANGECMP5B_1.
func Stream(scope Scope, src LogSource) string {
	return streamEDID(scope, src.FileType, src.Name)
}

func streamEDID(scope Scope, fileType, logSource string) string {
	return strings.Join([]string{scope.Network, scope.Station, scope.Campaign, fileType, logSource}, Sep)
}

// Split splits a stream EDID into its five fields: network, station,
// campaign, file type or product, and log source or derived input (which
// may hold Sep itself). ok is false if it has fewer.
func Split(edid string) (fields []string, ok bool) {
	fields = strings.SplitN(edid, Sep, 5)
	return fields, len(fields) == 5
}

// CheckScope reports an error if a field of scope contains Sep, which
// would shift the fields of its stream EDIDs.
func CheckScope(scope Scope) error {
	for _, f := range []string{scope.Network, scope.Station, scope.Campaign} {
		if strings.Contains(f, Sep) {
			return fmt.Errorf("scope %q contains %q, the stream EDID separator", f, Sep)
		}
	}
	return nil
}

// Log sources shared across file types, named for what the rows hold rather
// than the file they came from, so the same kind of stream has the same
// log source whichever file type it is parsed from.
const (
	LogINTGRPLY   = "INTG-RPLY"  // interrogations paired with their replies: travel_times, community_standard
	LogAHRS       = "AHRS"       // attitude_ahrs
	LogGNSSINS    = "GNSSINS"    // antenna positions from an INS solution: gnss_antenna_positions
	LogGNSSPSEUDO = "GNSSPSEUDO" // single-antenna pseudorange fixes, no INS: gnss_antenna_positions
	LogINSPVAA    = "INSPVAA"    // NovAtel INS position, velocity and attitude: inspva
)

// AntennaLogName is the log source name (the EDID's last field) of rows from log type logType on one
// antenna (measurement source): the log type, with NovAtel's _<antenna>
// suffix for antennas after the primary, e.g. RANGECMP5B, RANGECMP5B_1.
func AntennaLogName(logType string, antenna uint8) string {
	if antenna == 0 {
		return logType
	}
	return fmt.Sprintf("%s_%d", logType, antenna)
}

// Derived products, the fourth field of a derived stream EDID.
const (
	ProductPRIDE    = "PRIDE"    // PRIDE PPP-AR kinematic positions
	ProductSmoothed = "SMOOTHED" // Kalman-smoothed antenna track
	ProductGNSSA    = "GNSSA"    // GNSS-A shots
)

// Derived is the stream EDID of a derived product built from input, the
// stream it was built from with its scope dropped: e.g. PRIDE from
// NOV000:RANGEA (or NOV000.RANGEA) is net.sta.camp.PRIDE.NOV000.RANGEA. The
// input is the last field, so it may hold Sep (see Split).
func Derived(scope Scope, product, input string) string {
	return streamEDID(scope, product, strings.ReplaceAll(input, ":", Sep))
}
