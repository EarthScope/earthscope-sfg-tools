package schemas

import "path/filepath"

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
