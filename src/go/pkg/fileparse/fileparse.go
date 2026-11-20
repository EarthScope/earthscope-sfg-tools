// Package fileparse is the registry of the Level 0 parsers, one per (raw file
// type, table), and the detection of a raw file's type from its name.
//
// The parsing itself is in one subpackage per format (ahrs00, ctd, dfop00,
// gnss00, nov000, nov770, nva, qcpin, tcvr00). Each turns one file into one
// row type, with functions of the form
//
//	func(path string, src edid.LogSource) ([]datamodels.<X>Row, error)
//
// that return the rows of src (every log source in the file that feeds the row
// type when src is edid.AllLogSources), each carrying its LogSource, and never
// touch Arrow. Entry pairs each function with its datamodels builder, which
// stamps stream_edid = edid.Stream(scope, row.LogSource). A file is read once
// per parser run on it; finding the files is the caller's job.
//
// The decoders shared by several formats are in novatel (NovAtel logs),
// internal/gpsa (Sonardyne GPSA packets), internal/sv3 (SV3 events, shared by
// DFOP00 and QCPIN) and internal/rowutil.
package fileparse

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/ahrs00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/ctd"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/dfop00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/gnss00"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/nov000"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/nov770"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/nva"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/qcpin"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/tcvr00"
)

// The file types, as they appear in stream EDIDs (edid.LogSource.FileType).
// The format packages take theirs from the datamodels log-source vars.
const (
	FileTypeAHRS00  = "AHRS00"
	FileTypeNOV770  = "NOV770"
	FileTypeNOV000  = "NOV000"
	FileTypeNVA     = "NVA"
	FileTypeDFOP00  = "DFOP00"
	FileTypeGNSS00  = "GNSS00"
	FileTypeTCVR00  = "TCVR00"
	FileTypeQCPIN   = "QCPIN"
	FileTypeCTD     = "CTD"
	FileTypeSEABIRD = "SEABIRD"
)

// ErrUnknownLogSource is wrapped by Parser.Parse when asked for a log source
// its table does not list for the parser's file type.
var ErrUnknownLogSource = errors.New("log source not produced by this parser")

// Parser parses one raw file type into one Level 0 table. Build one with
// Entry; Parsers lists them.
type Parser struct {
	FileType string           // e.g. NOV000
	Ref      catalog.TableRef // the gnssa_l0 table Parse produces
	parse    func(path string, src edid.LogSource, scope edid.Scope) (arrow.Table, error)
}

// Entry pairs a format package's parse function with the datamodels builder
// of its row type R, into the Parser of fileType for ref (a gnssa_l0 ref).
func Entry[R any](fileType string, ref catalog.TableRef,
	parse func(path string, src edid.LogSource) ([]R, error),
	build func(rows []R, scope edid.Scope) (arrow.Table, error)) Parser {
	return Parser{FileType: fileType, Ref: ref, parse: func(path string, src edid.LogSource, scope edid.Scope) (arrow.Table, error) {
		rows, err := parse(path, src)
		if err != nil {
			return nil, err
		}
		return build(rows, scope)
	}}
}

// Sources are the log sources p produces: its table's Sources of p's file
// type.
func (p Parser) Sources() []edid.LogSource {
	var out []edid.LogSource
	for _, s := range p.Ref.Def.Sources {
		if s.FileType == p.FileType {
			out = append(out, s)
		}
	}
	return out
}

// Parse parses the file at path, of scope, into rows with the table's full
// schema: those of src, or of every log source of the file that feeds the
// table when src is edid.AllLogSources. A src that is not in Sources
// returns a wrapped ErrUnknownLogSource before the file is opened. The rows'
// stream EDIDs must all be of Sources (NOV770 builds its antenna-suffixed
// log sources at run time, and only the declared ones are accepted);
// otherwise Parse fails, naming the undeclared stream, rather than write
// rows no declaration accounts for. The caller releases the table.
func (p Parser) Parse(path string, src edid.LogSource, scope edid.Scope) (arrow.Table, error) {
	if !src.IsAll() && !slices.Contains(p.Sources(), src) {
		return nil, fmt.Errorf("%w: %s feeds %s from %v", ErrUnknownLogSource, src, p.Ref, p.Sources())
	}
	tbl, err := p.parse(path, src, scope)
	if err != nil {
		return nil, err
	}
	if err := p.checkStreams(tbl, scope); err != nil {
		tbl.Release()
		return nil, err
	}
	return tbl, nil
}

// checkStreams fails if a row of tbl has a stream_edid that is not the
// stream of one of p's Sources for scope.
func (p Parser) checkStreams(tbl arrow.Table, scope edid.Scope) error {
	idx := tbl.Schema().FieldIndices(catalog.StreamEDIDName)
	if len(idx) != 1 {
		return fmt.Errorf("%s rows have %d %s columns, want 1", p.Ref, len(idx), catalog.StreamEDIDName)
	}
	declared := map[string]bool{}
	for _, s := range p.Sources() {
		declared[edid.Stream(scope, s)] = true
	}
	last := ""
	for _, chunk := range tbl.Column(idx[0]).Data().Chunks() {
		col, ok := chunk.(*array.String)
		if !ok {
			return fmt.Errorf("%s: %s is not a string column", p.Ref, catalog.StreamEDIDName)
		}
		for i := range col.Len() {
			if col.IsNull(i) {
				return fmt.Errorf("%s: null %s", p.Ref, catalog.StreamEDIDName)
			}
			if v := col.Value(i); v != last {
				if !declared[v] {
					return fmt.Errorf("%s: %s parsed rows of undeclared log source %s: add it to the table's Sources in datamodels", p.Ref, p.FileType, v)
				}
				last = v
			}
		}
	}
	return nil
}

// Parsers are the Level 0 parsers, one per (file type, table), all of
// gnssa_l0. CheckRegistry checks them against the tables' Sources.
var Parsers = []Parser{
	Entry(FileTypeAHRS00, datamodels.L0AttitudeAHRS, ahrs00.ParseAttitude, datamodels.BuildAttitudeAHRS),
	Entry(FileTypeNOV770, datamodels.L0GNSSObs, nov770.ParseGNSSObs, datamodels.BuildGNSSObs),
	Entry(FileTypeNOV000, datamodels.L0GNSSObs, nov000.ParseGNSSObs, datamodels.BuildGNSSObs),
	Entry(FileTypeNOV000, datamodels.L0INSPVA, nov000.ParseINSPVA, datamodels.BuildINSPVA),
	Entry(FileTypeNOV000, datamodels.L0INSSTDEV, nov000.ParseINSSTDEV, datamodels.BuildINSSTDEV),
	Entry(FileTypeNVA, datamodels.L0GNSSObs, nva.ParseGNSSObs, datamodels.BuildGNSSObs),
	Entry(FileTypeNVA, datamodels.L0INSPVA, nva.ParseINSPVA, datamodels.BuildINSPVA),
	Entry(FileTypeNVA, datamodels.L0INSSTDEV, nva.ParseINSSTDEV, datamodels.BuildINSSTDEV),
	Entry(FileTypeDFOP00, datamodels.L0TWTT, dfop00.ParseTWTT, datamodels.BuildTWTT),
	Entry(FileTypeDFOP00, datamodels.L0INSPVA, dfop00.ParseINSPVA, datamodels.BuildINSPVA),
	Entry(FileTypeDFOP00, datamodels.L0CommunityStandard, dfop00.ParseCommunityStandard, datamodels.BuildCommunityStandard),
	Entry(FileTypeGNSS00, datamodels.L0GNSSAntennaPositions, gnss00.ParseAntennaPositions, datamodels.BuildGNSSAntennaPositions),
	Entry(FileTypeGNSS00, datamodels.L0GNSSSatellitesInView, gnss00.ParseSatellitesInView, datamodels.BuildGNSSSatellitesInView),
	Entry(FileTypeGNSS00, datamodels.L0GNSSDOP, gnss00.ParseDOP, datamodels.BuildGNSSDOP),
	Entry(FileTypeGNSS00, datamodels.L0GNSSCourseSpeed, gnss00.ParseCourseSpeed, datamodels.BuildGNSSCourseSpeed),
	Entry(FileTypeTCVR00, datamodels.L0TransceiverInterrogations, tcvr00.ParseInterrogations, datamodels.BuildTransceiverInterrogations),
	Entry(FileTypeTCVR00, datamodels.L0TransceiverRanges, tcvr00.ParseRanges, datamodels.BuildTransceiverRanges),
	Entry(FileTypeTCVR00, datamodels.L0TransceiverSettings, tcvr00.ParseSettings, datamodels.BuildTransceiverSettings),
	Entry(FileTypeQCPIN, datamodels.L0TWTT, qcpin.ParseTWTT, datamodels.BuildTWTT),
	Entry(FileTypeQCPIN, datamodels.L0AttitudeAHRS, qcpin.ParseAttitude, datamodels.BuildAttitudeAHRS),
	Entry(FileTypeQCPIN, datamodels.L0GNSSAntennaPositions, qcpin.ParseAntennaPositions, datamodels.BuildGNSSAntennaPositions),
	Entry(FileTypeQCPIN, datamodels.L0GNSSObs, qcpin.ParseGNSSObs, datamodels.BuildGNSSObs),
	Entry(FileTypeCTD, datamodels.L0SoundSpeed, ctd.ParseCTD, datamodels.BuildSoundSpeed),
	Entry(FileTypeSEABIRD, datamodels.L0SoundSpeed, ctd.ParseSeabird, datamodels.BuildSoundSpeed),
}

// CheckRegistry checks Parsers against the declarations: every parser is of
// a gnssa_l0 table and no (file type, table) has two; every (file type,
// table) that a gnssa_l0 table lists in Sources has exactly one parser; and
// every parser has a source to produce. (datamodels cannot import fileparse,
// so this is not in datamodels.CheckDefinitions; dev/gen_readme runs both.)
func CheckRegistry() error {
	var errs []string
	have := map[string]int{} // "<file type> <ref>" -> parsers
	for _, p := range Parsers {
		key := p.FileType + " " + p.Ref.String()
		have[key]++
		if p.Ref.Database != datamodels.GNSSA_L0.Name {
			errs = append(errs, fmt.Sprintf("parser %s: table %s is not in %s", p.FileType, p.Ref, datamodels.GNSSA_L0.Name))
		}
		if len(p.Sources()) == 0 {
			errs = append(errs, fmt.Sprintf("parser %s for %s: the table lists no %s log source", p.FileType, p.Ref, p.FileType))
		}
	}
	for key, n := range have {
		if n > 1 {
			errs = append(errs, fmt.Sprintf("%d parsers for %s", n, key))
		}
	}
	for _, ref := range datamodels.GNSSA_L0.Refs() {
		seen := map[string]bool{}
		for _, s := range ref.Def.Sources {
			key := s.FileType + " " + ref.String()
			if !seen[key] {
				seen[key] = true
				if have[key] == 0 {
					errs = append(errs, fmt.Sprintf("%s lists %s but no parser feeds it", ref, s))
				}
			}
		}
	}
	if len(errs) > 0 {
		slices.Sort(errs)
		return fmt.Errorf("parser registry:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
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
