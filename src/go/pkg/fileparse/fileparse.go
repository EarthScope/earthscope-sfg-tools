// Package fileparse lists the Level 1 parsers: one per (raw file type, table).
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

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/ahrs00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/ctd"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/dfop00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/gnss00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/novatel000"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/novatel770"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/qcpin"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/seabird"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/tcvr00"
)

// ParseFunc parses one file into the rows of one table, from every log source
// in the file that feeds it, without the scope and EDID columns (see
// datamodels.ParsedSchema).
type ParseFunc func(path string) (arrow.Table, error)

// EDIDFunc returns the stream EDID of rows for scope whose Log_Source is
// logSource, e.g. INSPVAA, or RANGECMP5B_1 for NOV770's second antenna
// (datamodels.LogSource).
type EDIDFunc func(scope datamodels.Scope, logSource string) string

// Parser parses one raw file type into one Level 1 table.
type Parser struct {
	FileType  string              // e.g. NOV000
	Table     datamodels.TableDef // the table Parse produces
	Parsefunc ParseFunc
	EDID      EDIDFunc // stream_edid of each row, from its Log_Source
}

// Parse takes in the file path and scope, returning an arrow table of the
// parsed rows for the Parser's table definition. It calls
// p.Parsefunc(path), then adds the Network, Station and Campaign
// columns from scope and maps p.EDID of each row's Log_Source to the
// "stream_edid" column, dropping Log_Source if p.Table has none (see stamp).
// The caller releases the table.
func (p *Parser) Parse(path string, scope datamodels.Scope) (arrow.Table, error) {
	parsed, err := p.Parsefunc(path)
	if err != nil {
		return nil, err
	}
	defer parsed.Release()
	return p.stamp(parsed, scope)
}

// streamEDID is the EDIDFunc of fileType's parsers:
// network:station:campaign:file_type:log_source.
func streamEDID(fileType string) EDIDFunc {
	return func(scope datamodels.Scope, logSource string) string {
		return datamodels.StreamEDID(scope, fileType, logSource)
	}
}

// Parsers are the Level 1 parsers, one per (file type, table).
var Parsers = []Parser{
	{FileType: ahrs00.FileType, Table: datamodels.AttitudeAHRS, Parsefunc: ahrs00.AttitudeAHRS, EDID: streamEDID(ahrs00.FileType)},
	{FileType: novatel770.FileType, Table: datamodels.GNSSObs, Parsefunc: novatel770.GNSSObs, EDID: streamEDID(novatel770.FileType)},
	{FileType: novatel000.FileType, Table: datamodels.GNSSObs, Parsefunc: novatel000.GNSSObs, EDID: streamEDID(novatel000.FileType)},
	{FileType: novatel000.FileType, Table: datamodels.INSPVA, Parsefunc: novatel000.INSPVA, EDID: streamEDID(novatel000.FileType)},
	{FileType: novatel000.FileType, Table: datamodels.INSSTDEV, Parsefunc: novatel000.INSSTDEV, EDID: streamEDID(novatel000.FileType)},
	{FileType: dfop00.FileType, Table: datamodels.TWTT, Parsefunc: dfop00.TWTT, EDID: streamEDID(dfop00.FileType)},
	{FileType: dfop00.FileType, Table: datamodels.AttitudeAHRS, Parsefunc: dfop00.AttitudeAHRS, EDID: streamEDID(dfop00.FileType)},
	{FileType: dfop00.FileType, Table: datamodels.GNSSAntennaPositions, Parsefunc: dfop00.AntennaPositions, EDID: streamEDID(dfop00.FileType)},
	{FileType: gnss00.FileType, Table: datamodels.GNSSAntennaPositions, Parsefunc: gnss00.AntennaPositions, EDID: streamEDID(gnss00.FileType)},
	{FileType: gnss00.FileType, Table: datamodels.GNSSSatellitesInView, Parsefunc: gnss00.SatellitesInView, EDID: streamEDID(gnss00.FileType)},
	{FileType: gnss00.FileType, Table: datamodels.GNSSDOP, Parsefunc: gnss00.DOP, EDID: streamEDID(gnss00.FileType)},
	{FileType: gnss00.FileType, Table: datamodels.GNSSCourseSpeed, Parsefunc: gnss00.CourseSpeed, EDID: streamEDID(gnss00.FileType)},
	{FileType: tcvr00.FileType, Table: datamodels.TransceiverInterrogations, Parsefunc: tcvr00.Interrogations, EDID: streamEDID(tcvr00.FileType)},
	{FileType: tcvr00.FileType, Table: datamodels.TransceiverRanges, Parsefunc: tcvr00.Ranges, EDID: streamEDID(tcvr00.FileType)},
	{FileType: tcvr00.FileType, Table: datamodels.TransceiverSettings, Parsefunc: tcvr00.Settings, EDID: streamEDID(tcvr00.FileType)},
	{FileType: qcpin.FileType, Table: datamodels.TWTT, Parsefunc: qcpin.TWTT, EDID: streamEDID(qcpin.FileType)},
	{FileType: qcpin.FileType, Table: datamodels.AttitudeAHRS, Parsefunc: qcpin.AttitudeAHRS, EDID: streamEDID(qcpin.FileType)},
	{FileType: qcpin.FileType, Table: datamodels.GNSSAntennaPositions, Parsefunc: qcpin.AntennaPositions, EDID: streamEDID(qcpin.FileType)},
	{FileType: qcpin.FileType, Table: datamodels.GNSSObs, Parsefunc: qcpin.GNSSObs, EDID: streamEDID(qcpin.FileType)},
	{FileType: ctd.FileType, Table: datamodels.SoundSpeed, Parsefunc: ctd.SoundSpeed, EDID: streamEDID(ctd.FileType)},
	{FileType: seabird.FileType, Table: datamodels.SoundSpeed, Parsefunc: seabird.SoundSpeed, EDID: streamEDID(seabird.FileType)},
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
