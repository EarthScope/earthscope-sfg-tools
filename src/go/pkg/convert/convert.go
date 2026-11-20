// Package convert moves rows from one source to one sink: the library
// behind `sfg convert`. Every part of a conversion is stated explicitly;
// nothing is inferred from paths, file names or extensions.
//
// A conversion connects a storage.Reader to a storage.Writer (see Run).
// CheckFlags and New reject a conversion before any work starts; Conversion.Run
// carries it out and returns its Result.
package convert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/files"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/iceberg"
	rawio "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/raw"
)

// Source and sink kinds.
const (
	SourceIceberg = "iceberg"
	SourceFiles   = "files"
	SinkFiles     = "files"
	SinkIceberg   = "iceberg"
)

// Spec is one conversion as given on the command line.
type Spec struct {
	Source        string   // SourceIceberg, SourceRaw or SourceFiles
	SourceURIs    []string // the warehouse: a directory (catalog.Local) or overriding the catalog's; raw files: files, globs or directories; or the directory of <database>/<table>/ files
	SourceCatalog string   // "local", "glue" or a catalog of the iceberg-go configuration (see catalog.ParseConfig)
	// SourceDatabasePrefix and SinkDatabasePrefix are the catalog's
	// Config.DatabasePrefix on each side, e.g. "dev_".
	SourceDatabasePrefix string
	Sink                 string // SinkFiles or SinkIceberg
	SinkURI              string // a local directory; or the sink warehouse: a directory (catalog.Local) or overriding the catalog's
	SinkCatalog          string // "local", "glue" or a catalog of the iceberg-go configuration (see catalog.ParseConfig)
	SinkDatabasePrefix   string
	TableFormat          string // files.Parquet, files.CSV or files.Arrow (a files source reads only the first two)

	Scope       edid.Scope
	Tables      []string // bare or namespace-qualified table names
	SourceTypes []string // raw file types; for Iceberg, one file type, or product for Level 1/2 tables
	LogSource   string   // log source, or product input for Level 1/2 tables
	Start, End  time.Time
	Columns     []string
	MaxFiles    int // raw: at most this many files of each type; 0 is all

	// TypesFromNames (raw) tells each file's type by its name instead of
	// taking the files as SourceTypes: SourceTypes then only limits the
	// types read, all by default, and named files of no type are skipped.
	TypesFromNames bool

	Workers int // raw: files parsed at once; 0 is runtime.NumCPU()
}

// A UsageError is a conversion stated wrongly: a missing, inapplicable or
// invalid flag. It is found before any work starts.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, args ...any) error {
	return &UsageError{fmt.Errorf(format, args...)}
}

// endpoint is the flags a source or sink kind takes, by flag name. A sink's
// rejects are flags its source would take but it can't honour.
type endpoint struct{ required, optional, rejects []string }

// alwaysRequired are the flags every conversion needs; alwaysAccepted
// are flags any conversion may take.
var (
	alwaysRequired = []string{"source", "source-uri", "sink", "sink-uri", "scope"}
	alwaysAccepted = []string{"result"}
)

var sources = map[string]endpoint{
	SourceIceberg: {
		required: []string{"source-catalog", "table"},
		optional: []string{"source-database-prefix", "source-type", "log-source", "start", "end", "columns"},
	},
	SourceFiles: {
		required: []string{"table-format"},
		optional: []string{"table", "source-type", "log-source", "columns"},
	},
	SourceRaw: {
		required: []string{"source-type"},
		optional: []string{"table", "log-source", "max-files", "workers"},
	},
}

var sinks = map[string]endpoint{
	SinkFiles:   {required: []string{"table-format"}},
	SinkIceberg: {required: []string{"sink-catalog"}, optional: []string{"sink-database-prefix"}, rejects: []string{"columns"}},
}

func kinds(m map[string]endpoint) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return strings.Join(ks, ", ")
}

// CheckFlags checks the names of the flags given (without dashes) against
// what the source and sink kinds take: every required flag present, and
// none that the pair doesn't take.
func CheckFlags(source, sink string, given []string) error {
	src, ok := sources[source]
	if !ok {
		return usagef("--source %q: want one of %s", source, kinds(sources))
	}
	snk, ok := sinks[sink]
	if !ok {
		return usagef("--sink %q: want one of %s", sink, kinds(sinks))
	}
	var missing, extra []string
	for _, name := range slices.Concat(alwaysRequired, src.required, snk.required) {
		if !slices.Contains(given, name) && !slices.Contains(missing, "--"+name) {
			missing = append(missing, "--"+name)
		}
	}
	takes := slices.Concat(alwaysRequired, alwaysAccepted, src.required, src.optional, snk.required, snk.optional)
	for _, name := range given {
		if !slices.Contains(takes, name) || slices.Contains(snk.rejects, name) {
			extra = append(extra, "--"+name)
		}
	}
	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("--source %s --sink %s needs %s", source, sink, strings.Join(missing, ", ")))
	}
	if len(extra) > 0 {
		errs = append(errs, fmt.Errorf("%s doesn't apply to --source %s --sink %s", strings.Join(extra, ", "), source, sink))
	}
	if err := errors.Join(errs...); err != nil {
		return &UsageError{err}
	}
	return nil
}

// ParseScope parses NET, NET/STA or NET/STA/CAMP.
func ParseScope(s string) (edid.Scope, error) {
	parts := strings.Split(s, "/")
	if len(parts) > 3 || slices.Contains(parts, "") {
		return edid.Scope{}, usagef("--scope %q: want NET, NET/STA or NET/STA/CAMP", s)
	}
	parts = append(parts, "", "")
	sc := edid.Scope{Network: parts[0], Station: parts[1], Campaign: parts[2]}
	if err := edid.CheckScope(sc); err != nil {
		return edid.Scope{}, usagef("--scope: %w", err)
	}
	return sc, nil
}

// Conversion is a checked Spec, ready to run: the Reader and Writer it
// connects (an Iceberg one is opened by Run, so that nothing connects before
// the flags are checked) and the Selection it reads.
type Conversion struct {
	spec   Spec
	tables []catalog.TableRef // the tables the source reads
	sel    storage.Selection

	reader storage.Reader
	writer storage.Writer
	// openReader and openWriter make the Reader or Writer when it is nil.
	openReader func(context.Context) (storage.Reader, error)
	openWriter func(context.Context) (storage.Writer, error)
}

// New checks spec's values and builds its reader and writer. An Iceberg
// source or sink without a URI takes its catalog's configured warehouse
// (see catalog.Config.DefaultWarehouse). Every error is a *UsageError.
func New(spec Spec) (*Conversion, error) {
	if _, ok := sources[spec.Source]; !ok {
		return nil, usagef("--source %q: want one of %s", spec.Source, kinds(sources))
	}
	if _, ok := sinks[spec.Sink]; !ok {
		return nil, usagef("--sink %q: want one of %s", spec.Sink, kinds(sinks))
	}
	if spec.Source == SourceIceberg && len(spec.SourceURIs) == 0 {
		if wh := defaultWarehouse(spec.SourceCatalog); wh != "" {
			spec.SourceURIs = []string{wh}
		}
	}
	if spec.Sink == SinkIceberg && spec.SinkURI == "" {
		spec.SinkURI = defaultWarehouse(spec.SinkCatalog)
	}
	if len(spec.SourceURIs) == 0 || slices.Contains(spec.SourceURIs, "") {
		return nil, usagef("--source-uri is required: %s", uriHint(spec.Source, spec.SourceCatalog, "source"))
	}
	if spec.SinkURI == "" {
		return nil, usagef("--sink-uri is required: %s", uriHint(spec.Sink, spec.SinkCatalog, "sink"))
	}
	c := &Conversion{spec: spec}
	if spec.Scope.Network == "" {
		return nil, usagef("--scope is required")
	}

	switch spec.Source {
	case SourceIceberg:
		plan, err := checkIcebergSource(spec)
		if err != nil {
			return nil, err
		}
		c.tables, c.sel = plan.refs, plan.sel
		def := datamodels.SFG.Only(plan.refs...)
		c.openReader = func(ctx context.Context) (storage.Reader, error) {
			r, err := iceberg.New(ctx, plan.cfg, def)
			if err != nil {
				return nil, err
			}
			return r, nil
		}
	case SourceRaw:
		plan, err := checkRawSource(spec)
		if err != nil {
			return nil, err
		}
		c.tables = plan.tables
		opts := []rawio.Option{rawio.Types(plan.types...), rawio.MaxFiles(spec.MaxFiles), rawio.Workers(spec.Workers)}
		if !spec.TypesFromNames && len(plan.types) == 1 {
			opts = append(opts, rawio.AsGiven())
		}
		if c.reader, err = rawio.NewReader(spec.SourceURIs, opts...); err != nil {
			return nil, usagef("--source-uri: %w", err)
		}
		c.sel = storage.Selection{Scope: spec.Scope, Tables: plan.tables}
		if !plan.logSource.IsAll() {
			c.sel.LogSources = []edid.LogSource{plan.logSource}
		}
	case SourceFiles:
		plan, err := checkFilesSource(spec)
		if err != nil {
			return nil, err
		}
		rdr, err := files.NewReader(plan.dir, plan.format)
		if err != nil {
			return nil, usagef("--source-uri: %w", err)
		}
		c.tables, c.sel, c.reader = plan.refs, plan.sel, rdr
		if len(c.tables) == 0 {
			c.tables = rdr.Tables()
		}
		if len(c.tables) == 0 {
			return nil, usagef("--source-uri %s holds no %s files under <database>/<table>/", plan.dir, plan.format)
		}
	}

	switch spec.Sink {
	case SinkFiles:
		if !slices.Contains([]files.Format{files.Parquet, files.CSV, files.Arrow}, files.Format(spec.TableFormat)) {
			return nil, usagef("--table-format %q: want %s, %s or %s", spec.TableFormat, files.Parquet, files.CSV, files.Arrow)
		}
		w, err := files.NewWriter(spec.SinkURI, files.Format(spec.TableFormat))
		if err != nil {
			return nil, usagef("--sink-uri: %w", err)
		}
		c.writer = w
	case SinkIceberg:
		sinkCfg, err := catalog.ParseConfig(spec.SinkCatalog, spec.SinkURI)
		if err != nil {
			return nil, usagef("--sink-catalog: %w", err)
		}
		opts := []iceberg.Option{iceberg.Create(), iceberg.DatabasePrefix(spec.SinkDatabasePrefix)}
		if spec.Source == SourceIceberg {
			if spec.SourceCatalog == spec.SinkCatalog && spec.SourceDatabasePrefix == spec.SinkDatabasePrefix &&
				sameWarehouse(spec.SourceURIs[0], spec.SinkURI) {
				return nil, usagef("--source-uri and --sink-uri are the same %s warehouse (and database prefix): a table can't be copied onto itself", spec.SinkCatalog)
			}
			opts = append(opts, iceberg.Properties(map[string]string{
				PropCopySource:    spec.SourceCatalog + ":" + spec.SourceURIs[0],
				PropCopySelection: selectionString(spec),
			}))
		}
		def := datamodels.SFG.Only(c.tables...)
		c.openWriter = func(ctx context.Context) (storage.Writer, error) {
			w, err := iceberg.New(ctx, sinkCfg, def, opts...)
			if err != nil {
				return nil, err
			}
			return w, nil
		}
	}
	return c, nil
}

// Snapshot summary properties of rows copied from another catalog.
const (
	PropCopySource    = "sfg.copy-source"    // <catalog>:<warehouse>
	PropCopySelection = "sfg.copy-selection" // the --scope, --source-type, --log-source, --start and --end used
)

// defaultWarehouse is the warehouse the iceberg-go configuration gives the
// --catalog name, or "". An invalid name is reported when the endpoint's
// Config is parsed.
func defaultWarehouse(name string) string {
	cfg, _ := catalog.ParseConfig(name, "")
	return cfg.DefaultWarehouse()
}

// sameWarehouse reports whether two warehouse URIs name the same place.
// uriHint says why an endpoint of kind needs its URI.
func uriHint(kind, cat, side string) string {
	switch {
	case kind == SourceRaw:
		return "the raw files, globs or directories"
	case kind == SourceFiles:
		return "the directory of <database>/<table>/ files"
	case kind == SinkFiles:
		return "the output directory"
	case cat == string(catalog.Local):
		return "the local warehouse directory"
	case cat == string(catalog.Glue):
		return "the S3 warehouse, e.g. s3://bucket/prefix"
	default:
		return fmt.Sprintf("--%s-catalog %q has no warehouse in the iceberg-go configuration", side, cat)
	}
}

func sameWarehouse(a, b string) bool {
	clean := func(u string) string {
		if strings.Contains(u, "://") {
			return strings.TrimSuffix(u, "/")
		}
		if abs, err := filepath.Abs(u); err == nil {
			return abs
		}
		return filepath.Clean(u)
	}
	return clean(a) == clean(b)
}

// selectionString is spec's row selection as flags, for a snapshot summary.
func selectionString(spec Spec) string {
	parts := []string{"--scope " + scopeString(spec.Scope)}
	if len(spec.SourceTypes) > 0 {
		parts = append(parts, "--source-type "+strings.Join(spec.SourceTypes, ","))
	}
	if spec.LogSource != "" {
		parts = append(parts, "--log-source "+spec.LogSource)
	}
	if !spec.Start.IsZero() {
		parts = append(parts, "--start "+timeString(spec.Start))
	}
	if !spec.End.IsZero() {
		parts = append(parts, "--end "+timeString(spec.End))
	}
	return strings.Join(parts, " ")
}

// Run carries out the conversion with convert.Run, and adds what was asked
// for to its Result. A table or batch that fails is recorded in the result
// and the run moves on; the returned error is a fatal one, after which the
// result holds what was done before it. A usage error found by Run (the
// writer does not accept a table) returns a nil Result.
func (c *Conversion) Run(ctx context.Context) (*Result, error) {
	r := c.reader
	if r == nil {
		var err error
		if r, err = c.openReader(ctx); err != nil {
			res := c.result()
			res.addError("source", strings.Join(c.spec.SourceURIs, " "), err)
			return res, err
		}
	}
	if cl, ok := r.(io.Closer); ok {
		defer cl.Close()
	}
	w := c.writer
	if w == nil {
		var err error
		if w, err = c.openWriter(ctx); err != nil {
			res := c.result()
			res.addError("sink", c.spec.SinkURI, err)
			return res, err
		}
	}
	res, err := Run(ctx, r, w, c.sel)
	if res == nil {
		return nil, err // a usage error
	}
	meta := c.result()
	res.Source, res.Sink, res.Scope, res.Selection = meta.Source, meta.Sink, meta.Scope, meta.Selection
	for i, e := range res.Errors {
		switch {
		case e.Stage == "source" && e.Subject == "":
			res.Errors[i].Subject = strings.Join(c.spec.SourceURIs, " ")
		case e.Stage == "sink" && e.Subject == "":
			res.Errors[i].Subject = c.spec.SinkURI
		}
	}
	return res, err
}
