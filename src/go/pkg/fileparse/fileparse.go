// Package fileparse lists the Level 0 parsers: one per (raw file type, table).
//
// Each subpackage parses one file type and has one function per table it
// produces, called with the file path, e.g. novatel000.INSPVA(path). It
// returns the table's rows from every log source in the file that feeds the
// table, without the scope and EDID columns (see datamodels.ParsedSchema).
// Parser.Parse calls it and then adds those columns from the scope, so the
// returned table has the table's full schema. A file is read once per parser
// run on it; finding the files is the caller's job.
package fileparse

import (
	"strings"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/ahrs00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/ctd"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/dfop00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/gnss00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel000"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel770"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/qcpin"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/seabird"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/tcvr00"
)

// ParseFunc parses one file into the rows of one table, from every log source
// in the file that feeds it, with Log_Source and without stream_edid (see
// datamodels.ParsedSchema).
type ParseFunc func(path string) (arrow.Table, error)

// Parser parses one raw file type into one Level 0 table.
type Parser struct {
	FileType  string              // e.g. NOV000
	Table     datamodels.TableDef // the table Parse produces
	ParseFile ParseFunc
}

// Parse takes in the file path and scope, returning an arrow table of the
// parsed rows for the Parser's table definition. It calls
// p.ParseFile(path), then maps each row's Log_Source to its p.StreamEDID in
// the "stream_edid" column, dropping Log_Source (see stamp).
// The caller releases the table.
func (p Parser) Parse(path string, scope datamodels.Scope) (arrow.Table, error) {
	parsed, err := p.ParseFile(path)
	if err != nil {
		return nil, err
	}
	defer parsed.Release()
	return p.stamp(parsed, scope)
}

// StreamEDID is the stream EDID of p's rows for scope whose Log_Source is
// logSource, e.g. INSPVAA, or RANGECMP5B_1 for NOV770's second antenna
// (datamodels.LogSource): network.station.campaign.file_type.log_source.
func (p Parser) StreamEDID(scope datamodels.Scope, logSource string) string {
	return datamodels.StreamEDID(scope, p.FileType, logSource)
}

// Parsers are the Level 0 parsers, one per (file type, table).
var Parsers = []Parser{
	{FileType: ahrs00.FileType, Table: datamodels.AttitudeAHRS, ParseFile: ahrs00.AttitudeAHRS},
	{FileType: novatel770.FileType, Table: datamodels.GNSSObs, ParseFile: novatel770.GNSSObs},
	{FileType: novatel000.FileType, Table: datamodels.GNSSObs, ParseFile: novatel000.GNSSObs},
	{FileType: novatel000.FileType, Table: datamodels.INSPVA, ParseFile: novatel000.INSPVA},
	{FileType: novatel000.FileType, Table: datamodels.INSSTDEV, ParseFile: novatel000.INSSTDEV},
	{FileType: dfop00.FileType, Table: datamodels.TWTT, ParseFile: dfop00.TWTT},
	{FileType: dfop00.FileType, Table: datamodels.INSPVA, ParseFile: dfop00.INSPVA},
	{FileType: gnss00.FileType, Table: datamodels.GNSSAntennaPositions, ParseFile: gnss00.AntennaPositions},
	{FileType: gnss00.FileType, Table: datamodels.GNSSSatellitesInView, ParseFile: gnss00.SatellitesInView},
	{FileType: gnss00.FileType, Table: datamodels.GNSSDOP, ParseFile: gnss00.DOP},
	{FileType: gnss00.FileType, Table: datamodels.GNSSCourseSpeed, ParseFile: gnss00.CourseSpeed},
	{FileType: tcvr00.FileType, Table: datamodels.TransceiverInterrogations, ParseFile: tcvr00.Interrogations},
	{FileType: tcvr00.FileType, Table: datamodels.TransceiverRanges, ParseFile: tcvr00.Ranges},
	{FileType: tcvr00.FileType, Table: datamodels.TransceiverSettings, ParseFile: tcvr00.Settings},
	{FileType: qcpin.FileType, Table: datamodels.TWTT, ParseFile: qcpin.TWTT},
	{FileType: qcpin.FileType, Table: datamodels.AttitudeAHRS, ParseFile: qcpin.AttitudeAHRS},
	{FileType: qcpin.FileType, Table: datamodels.GNSSAntennaPositions, ParseFile: qcpin.AntennaPositions},
	{FileType: qcpin.FileType, Table: datamodels.GNSSObs, ParseFile: qcpin.GNSSObs},
	{FileType: ctd.FileType, Table: datamodels.SoundSpeed, ParseFile: ctd.SoundSpeed},
	{FileType: seabird.FileType, Table: datamodels.SoundSpeed, ParseFile: seabird.SoundSpeed},
}

// ForFileType returns the parsers of fileType (case-insensitive), in
// Parsers order; none if it has no parsers.
func ForFileType(fileType string) []Parser {
	var ps []Parser
	for _, p := range Parsers {
		if strings.EqualFold(p.FileType, fileType) {
			ps = append(ps, p)
		}
	}
	return ps
}
