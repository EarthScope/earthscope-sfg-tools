package convert

import (
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/files"
)

// SourceRaw reads raw files through the Level 0 parsers.
const SourceRaw = "raw"

// rawPlan is a checked raw source: the file types to read, the Level 0
// tables they feed (or --table picks) and the one log source, if any.
type rawPlan struct {
	types     []string
	tables    []catalog.TableRef
	logSource edid.LogSource // edid.AllLogSources for all
}

// checkRawSource checks spec's raw source values, as usage errors.
func checkRawSource(spec Spec) (*rawPlan, error) {
	p := &rawPlan{logSource: edid.AllLogSources}
	if spec.Scope.Campaign == "" {
		return nil, usagef("--source %s needs the full --scope NET/STA/CAMP: it stamps every row", SourceRaw)
	}
	if spec.MaxFiles < 0 {
		return nil, usagef("--max-files %d: want 0 (all) or more", spec.MaxFiles)
	}
	if spec.Workers < 0 {
		return nil, usagef("--workers %d: want 0 (one per CPU) or more", spec.Workers)
	}
	types := spec.SourceTypes
	if spec.LogSource != "" {
		ls, err := rawLogSource(spec.LogSource)
		if err != nil {
			return nil, err
		}
		p.logSource = ls
		switch {
		case len(types) == 0:
			types = []string{ls.FileType}
		case len(types) != 1 || !strings.EqualFold(types[0], ls.FileType):
			return nil, usagef("--log-source %s is a %s stream: --source-type must be %s or omitted", ls, ls.FileType, ls.FileType)
		}
	}
	switch {
	case spec.TypesFromNames && len(types) == 0:
		types = fileTypes()
	case len(types) == 0:
		return nil, usagef("--source %s needs --source-type", SourceRaw)
	case !spec.TypesFromNames:
		for _, uri := range spec.SourceURIs {
			if info, err := os.Stat(uri); !(err == nil && info.IsDir()) && len(types) != 1 {
				return nil, usagef("--source-uri %s is a file or glob: give exactly one --source-type", uri)
			}
		}
	}
	produced := map[string]bool{}
	for _, ft := range types {
		ps := fileparse.ForFileType(ft)
		if len(ps) == 0 {
			return nil, usagef("--source-type %q: no parsers (want one of %s)", ft, strings.Join(fileTypes(), ", "))
		}
		ft = ps[0].FileType
		if slices.Contains(p.types, ft) {
			return nil, usagef("--source-type %s given twice", ft)
		}
		p.types = append(p.types, ft)
		for _, ps := range ps {
			produced[ps.Ref.Def.Name] = true
		}
	}
	if len(spec.Tables) == 0 {
		for _, ref := range datamodels.GNSSA_L0.Refs() {
			if produced[ref.Def.Name] {
				p.tables = append(p.tables, ref)
			}
		}
		return p, nil
	}
	for _, name := range spec.Tables {
		// Parsers write gnssa_l0, so a bare name is its table there.
		def, ok := datamodels.GNSSA_L0.Table(strings.TrimPrefix(name, datamodels.GNSSA_L0.Name+"."))
		if !ok {
			if _, err := datamodels.SFG.Find(name); err != nil && !strings.Contains(name, ".") {
				return nil, usagef("--table: %w", err)
			}
		}
		if !ok || !produced[def.Name] {
			return nil, usagef("--table %s: not produced by --source-type %s", name, strings.Join(p.types, ","))
		}
		if !slices.ContainsFunc(p.tables, func(r catalog.TableRef) bool { return r.Def.Name == def.Name }) {
			ref, err := datamodels.SFG.Find(datamodels.GNSSA_L0.Name + "." + def.Name)
			if err != nil {
				return nil, err
			}
			p.tables = append(p.tables, ref)
		}
	}
	return p, nil
}

// icebergPlan is a checked Iceberg source: its catalog, the tables to read
// and the selection.
type icebergPlan struct {
	cfg  catalog.Config
	refs []catalog.TableRef
	sel  storage.Selection
}

// checkIcebergSource checks spec's Iceberg source values, as usage errors.
func checkIcebergSource(spec Spec) (*icebergPlan, error) {
	if len(spec.SourceURIs) != 1 {
		return nil, usagef("--source %s takes one --source-uri", SourceIceberg)
	}
	cfg, err := catalog.ParseConfig(spec.SourceCatalog, spec.SourceURIs[0])
	if err != nil {
		return nil, usagef("--source-catalog: %w", err)
	}
	cfg = cfg.WithDatabasePrefix(spec.SourceDatabasePrefix)
	if len(spec.Tables) == 0 {
		return nil, usagef("--table is required for --source %s", SourceIceberg)
	}
	if len(spec.SourceTypes) > 1 {
		return nil, usagef("--source-type: --source %s takes one file type or product", SourceIceberg)
	}
	p := &icebergPlan{cfg: cfg, sel: storage.Selection{Scope: spec.Scope, Columns: spec.Columns}}
	p.sel.LogSources = selectedLogSources(spec)
	if !spec.Start.IsZero() && !spec.End.IsZero() && !spec.Start.Before(spec.End) {
		return nil, usagef("--start %s is not before --end %s", spec.Start.Format(time.RFC3339), spec.End.Format(time.RFC3339))
	}
	p.sel.Window = timeWindow(spec.Start, spec.End)
	level0 := true
	for _, name := range spec.Tables {
		ref, err := datamodels.SFG.Find(name)
		if err != nil {
			return nil, usagef("--table: %w", err)
		}
		def := ref.Def
		level0 = level0 && ref.Database == datamodels.GNSSA_L0.Name
		if _, err := p.sel.Filter(def); err != nil {
			return nil, usagef("%s: %w", def.Name, err)
		}
		sc, err := def.IcebergSchema()
		if err != nil {
			return nil, err
		}
		for _, col := range spec.Columns {
			if _, ok := sc.FindFieldByName(col); !ok {
				return nil, usagef("--columns: %s has no column %q", def.Name, col)
			}
		}
		if !slices.ContainsFunc(p.refs, func(r catalog.TableRef) bool { return r.Database == ref.Database && r.Def.Name == def.Name }) {
			p.refs = append(p.refs, ref)
		}
	}
	p.sel.Tables = p.refs
	// Level 0 log sources are declared; below it a log source is a product's
	// input, which is not.
	if ls := p.sel.LogSources; level0 && len(ls) == 1 && ls[0].FileType != "" && ls[0].Name != "" {
		if err := CheckLogSource(ls[0]); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// selectedLogSources are the log sources --source-type and --log-source
// pick, for a source that reads tables: nil for every stream. A log source
// without its file type is rejected by Selection's checks.
func selectedLogSources(spec Spec) []edid.LogSource {
	if len(spec.SourceTypes) != 1 && spec.LogSource == "" {
		return nil
	}
	ft := ""
	if len(spec.SourceTypes) == 1 {
		ft = spec.SourceTypes[0]
	}
	return []edid.LogSource{{FileType: ft, Name: spec.LogSource}}
}

// filesPlan is a checked files source: the directory, its format, the
// tables to read (all the directory holds when --table is not given) and
// the selection.
type filesPlan struct {
	dir    string
	format files.Format
	refs   []catalog.TableRef
	sel    storage.Selection
}

// checkFilesSource checks spec's files source values, as usage errors.
func checkFilesSource(spec Spec) (*filesPlan, error) {
	if len(spec.SourceURIs) != 1 {
		return nil, usagef("--source %s takes one --source-uri, the directory of <database>/<table>/ files", SourceFiles)
	}
	format := files.Format(spec.TableFormat)
	if format != files.CSV && format != files.Parquet {
		return nil, usagef("--table-format %q: --source %s reads %s or %s", spec.TableFormat, SourceFiles, files.Parquet, files.CSV)
	}
	if len(spec.SourceTypes) > 1 {
		return nil, usagef("--source-type: --source %s takes one file type or product", SourceFiles)
	}
	p := &filesPlan{dir: spec.SourceURIs[0], format: format,
		sel: storage.Selection{Scope: spec.Scope, Columns: spec.Columns}}
	p.sel.LogSources = selectedLogSources(spec)
	if _, err := p.sel.StreamMatcher(); err != nil {
		return nil, usagef("%w", err)
	}
	for _, name := range spec.Tables {
		ref, err := datamodels.SFG.Find(name)
		if err != nil {
			return nil, usagef("--table: %w", err)
		}
		schema, err := ref.Def.ArrowSchema()
		if err != nil {
			return nil, err
		}
		for _, col := range spec.Columns {
			if !schema.HasField(col) {
				return nil, usagef("--columns: %s has no column %q", ref.Def.Name, col)
			}
		}
		if !slices.ContainsFunc(p.refs, func(r catalog.TableRef) bool { return r.Database == ref.Database && r.Def.Name == ref.Def.Name }) {
			p.refs = append(p.refs, ref)
		}
	}
	p.sel.Tables = p.refs
	return p, nil
}

// timeWindow is [start, end) as a Selection's Window, which is inclusive: nil
// when neither is set, and open at the end of a zero time.
func timeWindow(start, end time.Time) *catalog.Window {
	if start.IsZero() && end.IsZero() {
		return nil
	}
	w := &catalog.Window{Min: math.MinInt64, Max: math.MaxInt64}
	if !start.IsZero() {
		w.Min = arrow.Timestamp(start.UnixMicro())
	}
	if !end.IsZero() {
		w.Max = arrow.Timestamp(end.UnixMicro() - 1)
	}
	return w
}

// rawLogSource parses a raw source's --log-source, <FILE_TYPE>.<NAME>, and
// requires some Level 0 table to list it (TableDef.Sources). The errors are
// usage errors, found before any file is opened.
func rawLogSource(v string) (edid.LogSource, error) {
	ls, err := edid.ParseLogSource(v)
	if err != nil {
		return ls, usagef("--log-source: %w", err)
	}
	if ls.FileType == "" || ls.Name == "" {
		return ls, usagef("--log-source %q: want <file_type>%s<name> with both parts, e.g. DFOP00%sINSPVAA", v, edid.Sep, edid.Sep)
	}
	return ls, CheckLogSource(ls)
}

// CheckLogSource is a *UsageError unless some table of refs lists ls in its
// Sources (TableDef.Sources); with no refs, any Level 0 table may. A raw
// source, the Level 0 tables of an Iceberg source and sfg rinex check their
// log source with it before any file or table is read.
func CheckLogSource(ls edid.LogSource, refs ...catalog.TableRef) error {
	if len(refs) == 0 {
		refs = datamodels.GNSSA_L0.Refs()
	}
	for _, ref := range refs {
		if slices.Contains(ref.Def.Sources, ls) {
			return nil
		}
	}
	if len(refs) == 1 {
		return usagef("--log-source %s: table %s does not list this log source", ls, refs[0])
	}
	return usagef("--log-source %s: no Level 0 table lists this log source", ls)
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
