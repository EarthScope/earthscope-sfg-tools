// Package fileparse holds the Level 0 parsers: one per (raw file type,
// table), listed in Parsers.
//
// Each file type has its own file (ahrs00.go, dfop00.go, novatel000.go,
// nva.go, qcpin.go, ...) with one parse function per table it produces, called with
// the file path and scope, e.g. parseNOV000INSPVA(path, scope). It returns
// the table's rows from every log source in the file that feeds the table,
// with the table's full schema: each row's stream_edid is
// edid.Stream(scope, FileType, its log source). A file is read once per
// parser run on it; finding the files is the caller's job.
//
// The rest are shared by the parsers: the row records and Arrow builders
// for tables fed by several file types (rows.go), gpsa.go (Sonardyne GPSA
// packets), novatel.go (NovAtel logs) and sv3.go (SV3 events, shared by
// DFOP00 and QCPIN). Single-use decoders and builders live in their file
// type's file, e.g. XSENS in ahrs00.go.
package fileparse

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// ParseFunc parses one file of scope into the rows of one table, from every
// log source in the file that feeds it, each stamped with its stream_edid.
type ParseFunc func(path string, scope edid.Scope) (arrow.Table, error)

// Parser parses one raw file type into one Level 0 table.
type Parser struct {
	FileType  string           // e.g. NOV000
	Table     catalog.TableDef // the table Parse produces
	ParseFile ParseFunc
}

// Parse parses the file at path, of scope, into rows with the table's
// full schema. The caller releases the table.
func (p Parser) Parse(path string, scope edid.Scope) (arrow.Table, error) {
	return p.ParseFile(path, scope)
}

// StreamEDID is the stream EDID of p's rows for scope whose log source is
// logSource, e.g. INSPVAA, or RANGECMP5B_1 for NOV770's second antenna
// (edid.LogSource): network.station.campaign.file_type.log_source.
func (p Parser) StreamEDID(scope edid.Scope, logSource string) string {
	return edid.Stream(scope, p.FileType, logSource)
}

// Parsers are the Level 0 parsers, one per (file type, table).
var Parsers = []Parser{
	{FileType: FileTypeAHRS00, Table: datamodels.AttitudeAHRS, ParseFile: parseAHRS00AttitudeAHRS},
	{FileType: FileTypeNOV770, Table: datamodels.GNSSObs, ParseFile: parseNOV770GNSSObs},
	{FileType: FileTypeNOV000, Table: datamodels.GNSSObs, ParseFile: parseNOV000GNSSObs},
	{FileType: FileTypeNOV000, Table: datamodels.INSPVA, ParseFile: parseNOV000INSPVA},
	{FileType: FileTypeNOV000, Table: datamodels.INSSTDEV, ParseFile: parseNOV000INSSTDEV},
	{FileType: FileTypeNVA, Table: datamodels.GNSSObs, ParseFile: parseNVAGNSSObs},
	{FileType: FileTypeNVA, Table: datamodels.INSPVA, ParseFile: parseNVAINSPVA},
	{FileType: FileTypeNVA, Table: datamodels.INSSTDEV, ParseFile: parseNVAINSSTDEV},
	{FileType: FileTypeDFOP00, Table: datamodels.TWTT, ParseFile: parseDFOP00TWTT},
	{FileType: FileTypeDFOP00, Table: datamodels.INSPVA, ParseFile: parseDFOP00INSPVA},
	{FileType: FileTypeDFOP00, Table: datamodels.CommunityStandard, ParseFile: parseDFOP00CommunityStandard},
	{FileType: FileTypeGNSS00, Table: datamodels.GNSSAntennaPositions, ParseFile: parseGNSS00AntennaPositions},
	{FileType: FileTypeGNSS00, Table: datamodels.GNSSSatellitesInView, ParseFile: parseGNSS00SatellitesInView},
	{FileType: FileTypeGNSS00, Table: datamodels.GNSSDOP, ParseFile: parseGNSS00DOP},
	{FileType: FileTypeGNSS00, Table: datamodels.GNSSCourseSpeed, ParseFile: parseGNSS00CourseSpeed},
	{FileType: FileTypeTCVR00, Table: datamodels.TransceiverInterrogations, ParseFile: parseTCVR00Interrogations},
	{FileType: FileTypeTCVR00, Table: datamodels.TransceiverRanges, ParseFile: parseTCVR00Ranges},
	{FileType: FileTypeTCVR00, Table: datamodels.TransceiverSettings, ParseFile: parseTCVR00Settings},
	{FileType: FileTypeQCPIN, Table: datamodels.TWTT, ParseFile: parseQCPINTWTT},
	{FileType: FileTypeQCPIN, Table: datamodels.AttitudeAHRS, ParseFile: parseQCPINAttitudeAHRS},
	{FileType: FileTypeQCPIN, Table: datamodels.GNSSAntennaPositions, ParseFile: parseQCPINAntennaPositions},
	{FileType: FileTypeQCPIN, Table: datamodels.GNSSObs, ParseFile: parseQCPINGNSSObs},
	{FileType: FileTypeCTD, Table: datamodels.SoundSpeed, ParseFile: parseCTDSoundSpeed},
	{FileType: FileTypeSEABIRD, Table: datamodels.SoundSpeed, ParseFile: parseSeabirdSoundSpeed},
}

// Patterns are the base-name globs of each file type's raw files, as
// Sonardyne campaign directories name them. NVA has none: bare NovAtel ASCII
// logs follow no naming convention, so callers name their type.
var Patterns = map[string]string{
	FileTypeAHRS00:  "*AHRS00.bin",
	FileTypeNOV770:  "*NOV770.raw",
	FileTypeNOV000:  "*NOV000.bin",
	FileTypeDFOP00:  "*DFOP00.raw",
	FileTypeGNSS00:  "*GNSS00.bin",
	FileTypeTCVR00:  "*TCVR00.bin",
	FileTypeCTD:     "*CTD*_fit",
	FileTypeSEABIRD: "*.cnv",
	FileTypeQCPIN:   "*.pin",
}

// FileTypeOf is the file type whose pattern (see Patterns) matches path's
// base name, or "" if none does.
func FileTypeOf(path string) string {
	name := filepath.Base(path)
	for _, ft := range slices.Sorted(maps.Keys(Patterns)) {
		if ok, _ := filepath.Match(Patterns[ft], name); ok {
			return ft
		}
	}
	return ""
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

// ReadGNSSEpochs decodes the GNSS observation epochs of the raw file at path,
// of fileType (case-insensitive): NOV000, NVA or QCPIN (log source RANGEA),
// or NOV770 (RANGECMPB, RANGECMP5B, and _1 for the second antenna; see
// edid.LogSource). It returns them by log source, in file order, skipping
// the same logs as the file type's GNSSObs parser. Unlike the parser it
// builds no table, so callers such as RINEX writers get the epochs directly.
func ReadGNSSEpochs(path, fileType string) (map[string][]observation.Epoch, error) {
	var (
		epochs []observation.Epoch
		err    error
	)
	switch strings.ToUpper(fileType) {
	case FileTypeNOV770:
		return nov770Epochs(path)
	case FileTypeNOV000:
		epochs, err = nov000Epochs(path)
	case FileTypeNVA:
		var r novatelRecords
		r, err = scanNovatelASCII(path, FileTypeNVA, newNovatelASCIIScanner, LogRANGEA)
		epochs = r.epochs
	case FileTypeQCPIN:
		epochs, err = qcpinEpochs(path)
	default:
		return nil, fmt.Errorf("no GNSS observations in file type %q: want %s, %s, %s or %s",
			fileType, FileTypeNOV000, FileTypeNOV770, FileTypeNVA, FileTypeQCPIN)
	}
	if err != nil {
		return nil, err
	}
	return map[string][]observation.Epoch{LogRANGEA: epochs}, nil
}
