// Package fileparse lists the raw file types with Level 1 parsers. Each
// subpackage parses one file type: ParseFile(path, def, scope) returns the
// rows of table def as an Arrow table, or an error when the file type cannot
// produce def.
package fileparse

import (
	"strings"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/ahrs00"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/dfop00"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/novatel000"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/novatel770"
)

// ParseFunc parses one file into the rows of def for scope.
type ParseFunc func(path string, def schemas.TableDef, scope schemas.Scope) (arrow.Table, error)

// FileType is one raw file type and the Level 1 tables it produces.
type FileType struct {
	Name    string // e.g. NOV770
	Pattern string // filename glob within a campaign raw directory
	Parse   ParseFunc
	Tables  []schemas.TableDef
}

// FileTypes are the file types with Level 1 parsers.
var FileTypes = []FileType{
	{Name: "AHRS00", Pattern: "*AHRS00.bin", Parse: ahrs00.ParseFile, Tables: ahrs00.Tables},
	{Name: "NOV770", Pattern: "*NOV770.raw", Parse: novatel770.ParseFile, Tables: novatel770.Tables},
	{Name: "NOV000", Pattern: "*NOV000.bin", Parse: novatel000.ParseFile, Tables: novatel000.Tables},
	{Name: "DFOP00", Pattern: "*DFOP00.raw", Parse: dfop00.ParseFile, Tables: dfop00.Tables},
}

// Lookup returns the file type named name (case-insensitive).
func Lookup(name string) (FileType, bool) {
	for _, ft := range FileTypes {
		if strings.EqualFold(ft.Name, name) {
			return ft, true
		}
	}
	return FileType{}, false
}

// Table returns the table of ft named name.
func (ft FileType) Table(name string) (schemas.TableDef, bool) {
	for _, t := range ft.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return schemas.TableDef{}, false
}
