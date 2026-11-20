// Package iceberg is the Reader and Writer of an Iceberg catalog: one
// *Iceberg is both, over the tables of one catalog definition.
//
// Reading yields each selected table's rows as one Batch per stream per hour
// of time. Writing replaces, for each Batch, its streams' rows over the
// Batch's time span (catalog.Table.Replace), so writing the same rows again
// leaves one copy of them.
//
// Layering: storage/iceberg imports catalog and storage, not convert or cmd.
package iceberg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
)

// KindIceberg is the Kind of the Outputs the Writer reports.
const KindIceberg = "iceberg"

// Option changes how an Iceberg connects or writes.
type Option func(*options)

type options struct {
	create    bool
	region    string
	catalogID string
	prefix    string
	prefixSet bool
	props     map[string]string
}

// Create makes New build the catalog (catalog.Build): missing databases and
// tables are created and older tables evolved to their definitions. Without
// it New only loads what exists (catalog.Load) and creates nothing.
func Create() Option { return func(o *options) { o.create = true } }

// Region sets the AWS region of a Glue catalog. It is an error for any other
// catalog.
func Region(r string) Option { return func(o *options) { o.region = r } }

// CatalogID sets the AWS account that owns a Glue catalog. It is an error for
// any other catalog.
func CatalogID(id string) Option { return func(o *options) { o.catalogID = id } }

// DatabasePrefix sets the prefix of the catalog's database names (Config's
// DatabasePrefix), e.g. "dev_" for dev_gnssa_l0.
func DatabasePrefix(p string) Option {
	return func(o *options) { o.prefix, o.prefixSet = p, true }
}

// Properties adds props to the summary of every snapshot the Writer commits,
// e.g. where a copy came from.
func Properties(props map[string]string) Option {
	return func(o *options) {
		if o.props == nil {
			o.props = map[string]string{}
		}
		for k, v := range props {
			o.props[k] = v
		}
	}
}

// Iceberg is a connection to a catalog, both a storage.Reader and a
// storage.Writer of the tables of the definition it was opened with. It is
// not safe for concurrent use.
type Iceberg struct {
	cfg   catalog.Config
	def   catalog.CatalogDef
	cat   *catalog.Catalog
	props map[string]string

	closeOnce sync.Once
	closeErr  error
}

var (
	_ storage.Reader = (*Iceberg)(nil)
	_ storage.Writer = (*Iceberg)(nil)
)

// New connects to the catalog cfg names. It loads the tables of def the
// catalog holds, creating nothing, or with Create builds them (see Create).
// The connection is opened here, so errors in cfg or the connection are
// New's. Close it.
//
// With Create and a cfg.Warehouse, every table of def the catalog holds
// must lie under that warehouse (checkLocations); one that does not is an
// error naming it, checked before Build creates or evolves anything. It
// stops a development warehouse given without its DatabasePrefix from
// evolving and writing production's tables of the same names.
func New(ctx context.Context, cfg catalog.Config, def catalog.CatalogDef, opts ...Option) (*Iceberg, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.region != "" {
		cfg.Region = o.region
	}
	if o.catalogID != "" {
		cfg.CatalogID = o.catalogID
	}
	if o.prefixSet {
		cfg.DatabasePrefix = o.prefix
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	guard := o.create && cfg.Warehouse != ""
	if guard {
		// Check the tables that exist before Build evolves any of them.
		if err := checkExisting(ctx, cfg, def); err != nil {
			return nil, err
		}
	}
	var (
		cat *catalog.Catalog
		err error
	)
	if o.create {
		cat, err = catalog.Build(ctx, cfg, def)
	} else {
		cat, err = catalog.Load(ctx, cfg, def)
	}
	if err != nil {
		return nil, err
	}
	if guard {
		if err := checkLocations(cat, cfg); err != nil {
			cat.Close()
			return nil, err
		}
	}
	return &Iceberg{cfg: cfg, def: def, cat: cat, props: o.props}, nil
}

// checkExisting loads (creating and evolving nothing) the tables of def that
// cfg's catalog already holds and checks their locations (checkLocations).
// A Local warehouse with no catalog yet holds no tables.
func checkExisting(ctx context.Context, cfg catalog.Config, def catalog.CatalogDef) error {
	cat, err := catalog.Load(ctx, cfg, def)
	if err != nil {
		if cfg.Kind == catalog.Local {
			if _, serr := os.Stat(filepath.Join(cfg.Warehouse, "catalog.db")); errors.Is(serr, fs.ErrNotExist) {
				return nil
			}
		}
		return err
	}
	defer cat.Close()
	return checkLocations(cat, cfg)
}

// checkLocations is an error unless every table cat holds lies under
// cfg.Warehouse ("" checks nothing). Building with a warehouse writes there,
// so a table found elsewhere belongs to another catalog sharing the same
// database names: e.g. a development warehouse given without its
// DatabasePrefix finds production's gnssa_l0. Trailing slashes are ignored,
// and a Local warehouse matches both its absolute path and its file:// URI.
func checkLocations(cat *catalog.Catalog, cfg catalog.Config) error {
	if cfg.Warehouse == "" {
		return nil
	}
	whs, err := normLocation(cfg.Warehouse)
	if err != nil {
		return err
	}
	for _, ref := range cat.Def().Refs() {
		tbl, err := cat.TableOf(ref)
		if errors.Is(err, catalog.ErrNotInCatalog) {
			continue
		}
		if err != nil {
			return err
		}
		loc := tbl.Iceberg().Location()
		ats, err := normLocation(loc)
		if err != nil {
			return err
		}
		if !under(ats, whs) {
			return fmt.Errorf("table %s is at %s, outside the warehouse %s: it belongs to another catalog with the same database names; "+
				"give the --database-prefix of this warehouse's databases (e.g. dev_), or the warehouse the table lives in",
				cfg.Namespace(ref.Database)+"."+ref.Def.Name, loc, cfg.Warehouse)
		}
	}
	return nil
}

// under reports whether some form of a location is within some form of a
// warehouse.
func under(locs, warehouses []string) bool {
	for _, at := range locs {
		for _, wh := range warehouses {
			if at == wh || strings.HasPrefix(at, strings.TrimRight(wh, "/")+"/") {
				return true
			}
		}
	}
	return false
}

// normLocation is the forms of a warehouse or table location to compare:
// without trailing slashes, and for a local path (with or without file://)
// its clean absolute path and, if it differs, that path with symlinks
// resolved (macOS reaches its temporary directories through /var).
func normLocation(loc string) ([]string, error) {
	if rest, ok := strings.CutPrefix(loc, "file://"); ok {
		loc = rest
	} else if strings.Contains(loc, "://") {
		return []string{strings.TrimRight(loc, "/")}, nil
	}
	abs, err := filepath.Abs(loc)
	if err != nil {
		return nil, err
	}
	out := []string{abs}
	if real, err := filepath.EvalSymlinks(abs); err == nil && real != abs {
		out = append(out, real)
	}
	return out, nil
}

// NewLocal connects to the local catalog in warehouse directory dir.
func NewLocal(ctx context.Context, dir string, def catalog.CatalogDef, opts ...Option) (*Iceberg, error) {
	return New(ctx, catalog.LocalConfig(dir), def, opts...)
}

// NewGlue connects to the AWS Glue catalog placing tables under the S3
// prefix warehouse.
func NewGlue(ctx context.Context, warehouse string, def catalog.CatalogDef, opts ...Option) (*Iceberg, error) {
	return New(ctx, catalog.GlueConfig(warehouse), def, opts...)
}

// Catalog is the underlying connection, for callers that need more than
// reading and writing (snapshot inspection). It is closed by Close.
func (i *Iceberg) Catalog() *catalog.Catalog { return i.cat }

// Close closes the catalog's connections. It may be called more than once.
func (i *Iceberg) Close() error {
	i.closeOnce.Do(func() { i.closeErr = i.cat.Close() })
	return i.closeErr
}

func (i *Iceberg) has(ref catalog.TableRef) bool {
	return slices.ContainsFunc(i.def.Refs(), func(r catalog.TableRef) bool {
		return r.Database == ref.Database && r.Def.Name == ref.Def.Name
	})
}

// Tables are the tables of the definition, in declaration order.
func (i *Iceberg) Tables() []catalog.TableRef { return i.def.Refs() }

// Accepts reports whether the definition holds ref.
func (i *Iceberg) Accepts(ref catalog.TableRef) bool { return i.has(ref) }

// Write replaces b's streams over b's time span in b's table, as one
// snapshot recording b.Source (path and SHA-256) when set. A Batch with no
// rows writes nothing.
func (i *Iceberg) Write(ctx context.Context, b storage.Batch) ([]storage.Output, error) {
	if b.Table == nil {
		return nil, fmt.Errorf("%s: batch has no table", b.Ref)
	}
	if !i.has(b.Ref) {
		return nil, fmt.Errorf("%s: not in the catalog definition", b.Ref)
	}
	if b.Table.NumRows() == 0 {
		return nil, nil
	}
	tbl, err := i.cat.TableOf(b.Ref)
	if err != nil {
		return nil, err
	}
	var opts []catalog.AppendOption
	if len(i.props) > 0 {
		opts = append(opts, catalog.WithProperties(i.props))
	}
	if b.Source != nil {
		opts = append(opts, catalog.WithSources(*b.Source))
	}
	n, err := tbl.Replace(ctx, b.Table, opts...)
	if err != nil {
		return nil, err
	}
	return []storage.Output{{
		Ref: b.Ref, Kind: KindIceberg, Rows: n,
		Path: i.cfg.Namespace(b.Ref.Database) + "." + b.Ref.Def.Name,
	}}, nil
}

// copyChunk is the time span of one Batch: [t, t+copyChunk) of one stream.
const copyChunk = time.Hour

// Read yields the rows sel selects from each table of sel.Tables that the
// definition holds (all of them when sel.Tables is empty), in time order: one
// Batch per stream per hour of time, so the Batches of one stream never
// overlap in time and a Writer replacing by span can take them in any order.
// A table with no selected rows yields nothing. A table that cannot be read,
// or a stream-hour that fails to scan, yields a Batch with Err (Ref set) and
// the read goes on with the rest. yield takes ownership of each Batch's
// Table (see storage.Batch).
func (i *Iceberg) Read(ctx context.Context, sel storage.Selection, yield func(storage.Batch) error) error {
	for _, ref := range i.selected(sel) {
		if err := i.readTable(ctx, ref, sel, yield); err != nil {
			return err
		}
	}
	return nil
}

// selected is sel.Tables within the definition, or all of it.
func (i *Iceberg) selected(sel storage.Selection) []catalog.TableRef {
	if len(sel.Tables) == 0 {
		return i.Tables()
	}
	var out []catalog.TableRef
	for _, ref := range sel.Tables {
		if i.has(ref) {
			out = append(out, ref)
		}
	}
	return out
}

func (i *Iceberg) readTable(ctx context.Context, ref catalog.TableRef, sel storage.Selection, yield func(storage.Batch) error) error {
	fail := func(err error) error { return yield(storage.Batch{Ref: ref, Err: err}) }
	tbl, err := i.cat.TableOf(ref)
	if err != nil {
		return fail(err)
	}
	def := tbl.Def()
	// The streams and their time spans, from two columns.
	spanSel := sel
	spanSel.Columns = []string{catalog.StreamEDIDName, def.TimeColumn}
	spans, err := streamSpans(ctx, tbl, spanSel, def)
	if err != nil {
		return fail(err)
	}
	for _, st := range spans {
		parts, ok := edid.Split(st.edid)
		if !ok {
			if err := fail(fmt.Errorf("stream_edid %q: want network.station.campaign.type.source", st.edid)); err != nil {
				return err
			}
			continue
		}
		one := sel
		one.Scope = edid.Scope{Network: parts[0], Station: parts[1], Campaign: parts[2]}
		one.LogSources = []edid.LogSource{{FileType: parts[3], Name: parts[4]}}
		for t := st.start.Truncate(copyChunk); !t.After(st.end); t = t.Add(copyChunk) {
			if err := ctx.Err(); err != nil {
				return err
			}
			lo, hi := t.UnixMicro(), t.Add(copyChunk).UnixMicro()-1
			if w := sel.Window; w != nil {
				lo, hi = max(lo, int64(w.Min)), min(hi, int64(w.Max))
			}
			one.Window = &catalog.Window{Min: arrow.Timestamp(lo), Max: arrow.Timestamp(hi)}
			rows, err := scanTable(ctx, tbl, one, def)
			if err != nil {
				if err := fail(fmt.Errorf("%s from %s: %w", st.edid, time.UnixMicro(lo).UTC().Format(time.RFC3339), err)); err != nil {
					return err
				}
				continue
			}
			if rows.NumRows() == 0 {
				rows.Release()
				continue
			}
			if err := yield(storage.Batch{Ref: ref, Table: rows}); err != nil {
				return err // yield owns rows, whatever it returns
			}
		}
	}
	return nil
}

// span is one stream's first and last time.
type span struct {
	edid       string
	start, end time.Time
}

// streamSpans is the streams sel selects from tbl with their time spans,
// ordered by stream_edid; sel.Columns are stream_edid and the time column.
func streamSpans(ctx context.Context, tbl *catalog.Table, sel storage.Selection, def catalog.TableDef) ([]span, error) {
	filter, err := sel.Filter(def)
	if err != nil {
		return nil, err
	}
	schema, batches, err := tbl.Scan(ctx, filter, sel.Columns)
	if err != nil {
		return nil, err
	}
	ei, ti := schema.FieldIndices(catalog.StreamEDIDName), schema.FieldIndices(def.TimeColumn)
	if len(ei) != 1 || len(ti) != 1 {
		return nil, fmt.Errorf("%s: want one %s and one %s column", def.Name, catalog.StreamEDIDName, def.TimeColumn)
	}
	tt, ok := schema.Field(ti[0]).Type.(*arrow.TimestampType)
	if !ok {
		return nil, fmt.Errorf("%s: %s is not a timestamp column", def.Name, def.TimeColumn)
	}
	toTime, err := tt.GetToTimeFunc()
	if err != nil {
		return nil, err
	}
	byEDID := map[string]*span{}
	for rec, err := range batches {
		if err != nil {
			return nil, err
		}
		edids, ok1 := rec.Column(ei[0]).(*array.String)
		times, ok2 := rec.Column(ti[0]).(*array.Timestamp)
		if !ok1 || !ok2 {
			rec.Release()
			return nil, fmt.Errorf("%s: unexpected column types", def.Name)
		}
		for r := range int(rec.NumRows()) {
			if !edids.IsValid(r) || !times.IsValid(r) {
				continue
			}
			e, ts := edids.Value(r), toTime(times.Value(r)).UTC()
			s, ok := byEDID[e]
			if !ok {
				s = &span{edid: e, start: ts, end: ts}
				byEDID[e] = s
			}
			if ts.Before(s.start) {
				s.start = ts
			}
			if ts.After(s.end) {
				s.end = ts
			}
		}
		rec.Release()
	}
	out := make([]span, 0, len(byEDID))
	for _, s := range byEDID {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b span) int {
		switch {
		case a.edid < b.edid:
			return -1
		case a.edid > b.edid:
			return 1
		}
		return 0
	})
	return out, nil
}

// scanTable reads the rows sel selects from tbl into one table.
func scanTable(ctx context.Context, tbl *catalog.Table, sel storage.Selection, def catalog.TableDef) (arrow.Table, error) {
	filter, err := sel.Filter(def)
	if err != nil {
		return nil, err
	}
	schema, batches, err := tbl.Scan(ctx, filter, sel.Columns)
	if err != nil {
		return nil, err
	}
	var recs []arrow.RecordBatch
	defer func() {
		for _, r := range recs {
			r.Release()
		}
	}()
	for rec, err := range batches {
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return array.NewTableFromRecords(schema, recs), nil
}
