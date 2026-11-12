package tcvr00

import "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"

// testScope is the scope the tests parse files of.
var testScope = datamodels.Scope{Network: "net", Station: "STA", Campaign: "2026_A_0001"}

// testEDID is the stream EDID of rows of logSource in this file type for testScope.
func testEDID(logSource string) string { return datamodels.StreamEDID(testScope, FileType, logSource) }
