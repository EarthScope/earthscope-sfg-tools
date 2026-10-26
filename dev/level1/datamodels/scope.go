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

// StreamEDID is the EDID of the stream of rows from logSource in a file of
// type fileType for scope: network:station:campaign:fileType:logSource,
// e.g. cascadia-gorda:SLT1:2026_A_1126:NOV770:RANGECMP5B_1.
func StreamEDID(scope Scope, fileType, logSource string) string {
	return strings.Join([]string{scope.Network, scope.Station, scope.Campaign, fileType, logSource}, ":")
}

// LogSource is the Log_Source value of rows from log type logType on one
// antenna (measurement source): the log type, with NovAtel's _<antenna>
// suffix for antennas after the primary, e.g. RANGECMP5B, RANGECMP5B_1.
func LogSource(logType string, antenna uint8) string {
	if antenna == 0 {
		return logType
	}
	return fmt.Sprintf("%s_%d", logType, antenna)
}
