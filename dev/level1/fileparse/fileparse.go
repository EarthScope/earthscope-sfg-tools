// Package fileparse lists the Level 1 parsers: one per (raw file type, log
// type, table).
//
// Each subpackage parses one file type and has one function per table it
// produces, called with the file path and log type, e.g.
// novatel000.INSPVA(path, novatel000.LogINSPVAA). It returns the table's rows
// without the scope and EDID columns (see datamodels.ParsedSchema).
// Parser.Parse calls it and then adds those columns from the scope, so the
// returned table has the table's full schema. A file is read once per parser
// run on it; finding the files is the caller's job.
package fileparse

import (
	"strings"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/ahrs00"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/dfop00"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/novatel000"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/novatel770"
)

// ParseFunc parses the logType logs of one file into the rows of one table,
// without the scope and EDID columns (see datamodels.ParsedSchema).
type ParseFunc func(path, logType string) (arrow.Table, error)

// EDIDFunc returns the stream EDID of rows for scope whose Log_Source is
// logType: the log type, or for NOV770's other antennas the log type plus
// its _<antenna> suffix (datamodels.LogSource).
type EDIDFunc func(scope datamodels.Scope, logType string) string

// Parser parses one log type of one raw file type into one Level 1 table.
type Parser struct {
	FileType  string              // e.g. NOV000
	LogType   string              // e.g. INSPVAA; passed to Parsefunc
	Table     datamodels.TableDef // the table Parse produces
	Parsefunc ParseFunc
	EDID      EDIDFunc // stream_edid of each row, from its Log_Source
}

// Parse takes in the file path and scope, returning an arrow table of the
// parsed rows for the Parser's table definition. It calls
// p.Parsefunc(path, p.LogType), then adds the Network, Station and Campaign
// columns from scope and maps p.EDID of each row's Log_Source to the
// "stream_edid" column, dropping Log_Source if p.Table has none (see stamp).
// The caller releases the table.
func (p *Parser) Parse(path string, scope datamodels.Scope) (arrow.Table, error) {
	parsed, err := p.Parsefunc(path, p.LogType)
	if err != nil {
		return nil, err
	}
	defer parsed.Release()
	return p.stamp(parsed, scope)
}

// streamEDID is the EDIDFunc of fileType's parsers:
// network:station:campaign:file_type:log_source.
func streamEDID(fileType string) EDIDFunc {
	return func(scope datamodels.Scope, logType string) string {
		return datamodels.StreamEDID(scope, fileType, logType)
	}
}

// Parsers are the Level 1 parsers, one per (file type, log type, table).
var Parsers = []Parser{
	{FileType: ahrs00.FileType, LogType: ahrs00.LogType, Table: datamodels.AttitudeAHRS, Parsefunc: ahrs00.AttitudeAHRS, EDID: streamEDID(ahrs00.FileType)},
	{FileType: novatel770.FileType, LogType: novatel770.LogRANGECMPB, Table: datamodels.GNSSObs, Parsefunc: novatel770.GNSSObs, EDID: streamEDID(novatel770.FileType)},
	{FileType: novatel770.FileType, LogType: novatel770.LogRANGECMP5B, Table: datamodels.GNSSObs, Parsefunc: novatel770.GNSSObs, EDID: streamEDID(novatel770.FileType)},
	{FileType: novatel000.FileType, LogType: novatel000.LogRANGEA, Table: datamodels.GNSSObs, Parsefunc: novatel000.GNSSObs, EDID: streamEDID(novatel000.FileType)},
	{FileType: novatel000.FileType, LogType: novatel000.LogINSPVAA, Table: datamodels.INSPVA, Parsefunc: novatel000.INSPVA, EDID: streamEDID(novatel000.FileType)},
	{FileType: novatel000.FileType, LogType: novatel000.LogINSSTDEVA, Table: datamodels.INSSTDEV, Parsefunc: novatel000.INSSTDEV, EDID: streamEDID(novatel000.FileType)},
	{FileType: dfop00.FileType, LogType: dfop00.LogType, Table: datamodels.TWTT, Parsefunc: dfop00.TWTT, EDID: streamEDID(dfop00.FileType)},
	{FileType: dfop00.FileType, LogType: dfop00.LogType, Table: datamodels.AttitudeAHRS, Parsefunc: dfop00.AttitudeAHRS, EDID: streamEDID(dfop00.FileType)},
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
