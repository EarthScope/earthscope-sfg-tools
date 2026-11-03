package convert

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/compute"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
)

// SourceRaw reads raw files through the Level 1 parsers.
const SourceRaw = "raw"

// Patterns are the filename globs of each file type in a campaign raw
// directory; QC PIN files are unpacked under the campaign's qc/. A raw
// source given a directory reads only the files matching its types.
var Patterns = map[string]string{
	"AHRS00":  "*AHRS00.bin",
	"NOV770":  "*NOV770.raw",
	"NOV000":  "*NOV000.bin",
	"DFOP00":  "*DFOP00.raw",
	"GNSS00":  "*GNSS00.bin",
	"TCVR00":  "*TCVR00.bin",
	"CTD":     "*CTD*_fit",
	"SEABIRD": "*.cnv",
	"QCPIN":   "../qc/*/*.pin",
}

// rawSource parses the files of each type at its URIs into Level 1 tables, one
// table at a time: a file is parsed once per table its type feeds.
type rawSource struct {
	uris      []string
	types     []string
	defs      []datamodels.TableDef // the tables to read, in Level1Tables order
	scope     datamodels.Scope
	logSource string
	maxFiles  int

	inputs []Input
	failed map[string][]string // path -> "table: error" of each failed parse
}

// newRawSource checks spec's raw source values.
func newRawSource(spec Spec) (*rawSource, error) {
	s := &rawSource{uris: spec.SourceURIs, scope: spec.Scope, logSource: spec.LogSource, maxFiles: spec.MaxFiles,
		failed: map[string][]string{}}
	if spec.Scope.Campaign == "" {
		return nil, usagef("--source %s needs the full --scope NET/STA/CAMP: it stamps every row", SourceRaw)
	}
	if len(spec.SourceTypes) == 0 {
		return nil, usagef("--source %s needs --source-type", SourceRaw)
	}
	for _, uri := range spec.SourceURIs {
		if info, err := os.Stat(uri); !(err == nil && info.IsDir()) && len(spec.SourceTypes) != 1 {
			return nil, usagef("--source-uri %s is a file or glob: give exactly one --source-type", uri)
		}
	}
	if spec.MaxFiles < 0 {
		return nil, usagef("--max-files %d: want 0 (all) or more", spec.MaxFiles)
	}
	produced := map[string]bool{}
	for _, ft := range spec.SourceTypes {
		ps := fileparse.ForFileType(ft)
		if len(ps) == 0 {
			return nil, usagef("--source-type %q: no parsers (want one of %s)", ft, strings.Join(fileTypes(), ", "))
		}
		ft = ps[0].FileType
		if slices.Contains(s.types, ft) {
			return nil, usagef("--source-type %s given twice", ft)
		}
		s.types = append(s.types, ft)
		for _, p := range ps {
			produced[p.Table.Name] = true
		}
	}
	if len(spec.Tables) == 0 {
		for _, def := range datamodels.Level1Tables {
			if produced[def.Name] {
				s.defs = append(s.defs, def)
			}
		}
		return s, nil
	}
	for _, name := range spec.Tables {
		def, err := datamodels.TableByName(name)
		if err != nil {
			return nil, usagef("--table: %w", err)
		}
		if def.Level != 1 || !produced[def.Name] {
			return nil, usagef("--table %s: not produced by --source-type %s", name, strings.Join(s.types, ","))
		}
		if !slices.ContainsFunc(s.defs, func(d datamodels.TableDef) bool { return d.Name == def.Name }) {
			s.defs = append(s.defs, def)
		}
	}
	return s, nil
}

func fileTypes() []string {
	var out []string
	for _, p := range fileparse.Parsers {
		if !slices.Contains(out, p.FileType) {
			out = append(out, p.FileType)
		}
	}
	return out
}

// files lists the files of each type at s.uris: in a directory, the files
// matching each type's pattern; a file or glob (one type only) is itself.
// A file named more than once is listed once.
func (s *rawSource) files() (map[string][]string, error) {
	out := map[string][]string{}
	total := 0
	for _, ft := range s.types {
		var paths []string
		for _, uri := range s.uris {
			info, err := os.Stat(uri)
			isGlob := strings.ContainsAny(uri, "*?[")
			switch {
			case err == nil && info.IsDir():
				matches, _ := filepath.Glob(filepath.Join(uri, Patterns[ft]))
				paths = append(paths, matches...)
			case isGlob:
				matches, err := filepath.Glob(uri)
				if err != nil {
					return nil, usagef("--source-uri %s: %w", uri, err)
				}
				paths = append(paths, matches...)
			case err != nil:
				return nil, err
			default:
				paths = append(paths, uri)
			}
		}
		// Skip the "._" AppleDouble files macOS writes beside files on
		// non-Mac volumes, and directories.
		paths = slices.DeleteFunc(paths, func(p string) bool {
			fi, err := os.Stat(p)
			return strings.HasPrefix(filepath.Base(p), "._") || (err == nil && fi.IsDir())
		})
		slices.Sort(paths)
		paths = slices.Compact(paths)
		if s.maxFiles > 0 && len(paths) > s.maxFiles {
			paths = paths[:s.maxFiles]
		}
		out[ft] = paths
		total += len(paths)
	}
	if total == 0 {
		return nil, fmt.Errorf("no %s files at %s", strings.Join(s.types, ", "), strings.Join(s.uris, " "))
	}
	return out, nil
}

func (s *rawSource) Read(ctx context.Context, write func(Rows)) error {
	files, err := s.files()
	if err != nil {
		return err
	}
	for _, ft := range s.types {
		for _, path := range files[ft] {
			s.inputs = append(s.inputs, Input{Path: path, Type: ft, Status: "ok"})
		}
	}
	for _, def := range s.defs {
		schema, err := def.ArrowSchema()
		if err != nil {
			write(Rows{Def: def, Err: err})
			continue
		}
		var parsers []fileparse.Parser
		for _, ft := range s.types {
			for _, p := range fileparse.ForFileType(ft) {
				if p.Table.Name == def.Name && len(files[ft]) > 0 {
					parsers = append(parsers, p)
				}
			}
		}
		if len(parsers) == 0 {
			continue // no files of the types that feed it
		}
		write(Rows{Def: def, Schema: schema, Groups: s.groups(ctx, parsers, files)})
	}
	// A file that failed for any table is a failed input.
	for i, in := range s.inputs {
		if errs := s.failed[in.Path]; len(errs) > 0 {
			s.inputs[i].Status, s.inputs[i].Error = "failed", strings.Join(errs, "; ")
		}
	}
	return nil
}

// groups parses each file with its type's parser and yields its rows as
// one group, filtered to s.logSource when given. A file that fails to
// parse is recorded and skipped.
func (s *rawSource) groups(ctx context.Context, parsers []fileparse.Parser, files map[string][]string) iter.Seq2[Group, error] {
	return func(yield func(Group, error) bool) {
		for _, p := range parsers {
			keep := ""
			if s.logSource != "" {
				keep = p.EDID(s.scope, s.logSource)
			}
			for _, path := range files[p.FileType] {
				if err := ctx.Err(); err != nil {
					yield(Group{}, err)
					return
				}
				tbl, err := p.Parse(path, s.scope)
				if err == nil && keep != "" {
					tbl, err = filterTable(tbl, keep)
				}
				if err != nil {
					s.failed[path] = append(s.failed[path], p.Table.Name+": "+err.Error())
					continue
				}
				if !yield(Group{Rows: tbl, Source: path}, nil) {
					return
				}
			}
		}
	}
}

// filterTable is the rows of tbl whose stream_edid is edid. It releases
// tbl.
func filterTable(tbl arrow.Table, edid string) (arrow.Table, error) {
	defer tbl.Release()
	rdr := array.NewTableReader(tbl, -1)
	defer rdr.Release()
	var recs []arrow.RecordBatch
	defer func() {
		for _, r := range recs {
			r.Release()
		}
	}()
	for rdr.Next() {
		rec, err := filterStream(rdr.RecordBatch(), edid)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return array.NewTableFromRecords(tbl.Schema(), recs), nil
}

// filterStream is the rows of rec whose stream_edid is edid.
func filterStream(rec arrow.RecordBatch, edid string) (arrow.RecordBatch, error) {
	idx := rec.Schema().FieldIndices(datamodels.StreamEDIDColumn.Name)
	if len(idx) != 1 {
		return nil, fmt.Errorf("rows have no %s column", datamodels.StreamEDIDColumn.Name)
	}
	edids := rec.Column(idx[0]).(*array.String)
	b := array.NewBooleanBuilder(memory.DefaultAllocator)
	defer b.Release()
	for i := range edids.Len() {
		b.Append(edids.IsValid(i) && edids.Value(i) == edid)
	}
	mask := b.NewBooleanArray()
	defer mask.Release()
	return compute.FilterRecordBatch(context.Background(), rec, mask, compute.DefaultFilterOptions())
}

// Inputs is every file Read chose, with its status.
func (s *rawSource) Inputs() []Input { return s.inputs }
