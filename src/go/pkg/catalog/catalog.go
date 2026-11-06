package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	_ "github.com/apache/iceberg-go/catalog/glue"
	_ "github.com/apache/iceberg-go/catalog/hadoop"
	_ "github.com/apache/iceberg-go/catalog/rest"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	"github.com/apache/iceberg-go/config"
	_ "github.com/apache/iceberg-go/io/gocloud" // s3://, gs:// and abfs[s]:// table IO
	"github.com/apache/iceberg-go/table"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Local is the Config.Name of a local warehouse: a directory holding an
// SQLite catalog.db and the table data. Any other name is a catalog of the
// iceberg-go configuration file.
const Local = "local"

// Config names the Iceberg catalog, as iceberg-go configures one: Name is a
// catalog of .iceberg-go.yaml (in $GOICEBERG_HOME, else the home
// directory), or "" for its default-catalog, and gives the catalog type
// (rest, glue, sql, hadoop), URI, credential and warehouse. Table data
// can be on any storage iceberg-go's FileIO reads: s3://, gs://, abfs[s]://
// or file://. Cloud credentials come from each cloud's default chain (env,
// profile, workload identity), or from Props.
//
// The catalog Local is a warehouse directory instead: Warehouse is the
// directory, created if needed, with an SQLite catalog in catalog.db.
type Config struct {
	Name      string
	Warehouse string             // overrides the configured warehouse; Local: the directory
	Props     iceberg.Properties // catalog and FileIO properties, overriding the configuration
}

// String describes the catalog for messages: name:warehouse.
func (cfg Config) String() string {
	name := cfg.Name
	if name == "" {
		name = "default"
	}
	if cfg.Warehouse == "" {
		return name
	}
	return name + ":" + cfg.Warehouse
}

// Catalog is an Iceberg catalog holding the tables (see Config).
type Catalog struct {
	cat    icecat.Catalog
	tables map[string]*table.Table
}

// Open opens the catalog cfg names, creating a Local warehouse if needed.
func Open(ctx context.Context, cfg Config) (*Catalog, error) {
	name, props := cfg.Name, maps.Clone(cfg.Props)
	if props == nil {
		props = iceberg.Properties{}
	}
	if cfg.Name == Local {
		local, err := localProps(cfg.Warehouse)
		if err != nil {
			return nil, err
		}
		maps.Copy(props, local)
		// The sql catalog keys its rows by catalog name; existing local
		// warehouses and pyiceberg's load_catalog("default", ...) use "default".
		name = "default"
	} else {
		if props["type"] == "" {
			if err := Check(name); err != nil {
				return nil, err
			}
		}
		if cfg.Warehouse != "" {
			props["warehouse"] = cfg.Warehouse
		}
	}
	cat, err := icecat.Load(ctx, name, props)
	if err != nil {
		return nil, fmt.Errorf("opening catalog %s: %w", cfg, err)
	}
	return &Catalog{cat: cat, tables: make(map[string]*table.Table)}, nil
}

// Check reports whether name is a catalog Open can find: Local, a catalog of
// the iceberg-go configuration, or "" for its default-catalog.
func Check(name string) error {
	if name == Local || name == "" {
		return nil
	}
	if _, ok := config.EnvConfig.Catalogs[name]; !ok {
		return fmt.Errorf("no catalog %q: want %s or a catalog of the iceberg-go configuration (.iceberg-go.yaml in $GOICEBERG_HOME or the home directory)", name, Local)
	}
	return nil
}

// Warehouse is the warehouse the iceberg-go configuration gives catalog
// name ("" for its default-catalog), or "" when it gives none. Local has
// no configured warehouse: its warehouse is the directory.
func Warehouse(name string) string {
	if name == Local {
		return ""
	}
	if name == "" {
		name = config.EnvConfig.DefaultCatalog
	}
	return config.EnvConfig.Catalogs[name].Warehouse
}

// localProps are the sql catalog properties of a Local warehouse directory,
// which is created if needed.
func localProps(dir string) (iceberg.Properties, error) {
	if dir == "" {
		return nil, errors.New("a local catalog needs a warehouse directory")
	}
	warehouse, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(warehouse, 0o755); err != nil {
		return nil, err
	}
	return iceberg.Properties{
		"type":            "sql",
		sqlcat.DriverKey:  sqliteshim.ShimName,
		sqlcat.DialectKey: string(sqlcat.SQLite),
		"uri":             filepath.Join(warehouse, "catalog.db"),
		"warehouse":       "file://" + warehouse,
	}, nil
}

// Close closes the catalog's connections.
func (c *Catalog) Close() error {
	if closer, ok := c.cat.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Table loads def's table, creating its namespace and table if needed.
func (c *Catalog) Table(ctx context.Context, def datamodels.TableDef) (*table.Table, error) {
	key := def.Namespace() + "." + def.Name
	if tbl, ok := c.tables[key]; ok {
		return tbl, nil
	}
	// A new namespace gets no location: the catalog places its tables at
	// <warehouse>/<namespace>.db/<table>, or where the server decides (rest).
	ns := table.Identifier{def.Namespace()}
	err := c.cat.CreateNamespace(ctx, ns, nil)
	if err != nil && !errors.Is(err, icecat.ErrNamespaceAlreadyExists) {
		return nil, fmt.Errorf("creating namespace %s: %w", def.Namespace(), err)
	}
	tbl, err := c.cat.LoadTable(ctx, def.Identifier())
	switch {
	case errors.Is(err, icecat.ErrNoSuchTable):
		tbl, err = c.create(ctx, def)
	case err == nil:
		tbl, err = c.evolve(ctx, tbl, def)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	c.tables[key] = tbl
	return tbl, nil
}

// evolve adds to tbl the columns def has and tbl lacks, so a table created by
// an earlier version of def takes new optional columns; existing rows read
// them as null. A missing required column is an error: old rows would
// violate it. Columns def no longer has are left alone. Tables defined by
// another library (def.External) are not evolved.
func (c *Catalog) evolve(ctx context.Context, tbl *table.Table, def datamodels.TableDef) (*table.Table, error) {
	if def.External != nil {
		return tbl, nil
	}
	var missing []datamodels.Column
	for _, col := range def.Columns {
		if _, ok := tbl.Schema().FindFieldByName(col.Name); !ok {
			missing = append(missing, col)
		}
	}
	if len(missing) == 0 {
		return tbl, nil
	}
	tx := tbl.NewTransaction()
	update := tx.UpdateSchema(true, false)
	for _, col := range missing {
		if col.Required {
			return nil, fmt.Errorf("table lacks required column %q; it cannot be added to existing rows", col.Name)
		}
		update = update.AddColumn([]string{col.Name}, col.Type, col.Doc, false, nil)
	}
	if err := update.Commit(); err != nil {
		return nil, fmt.Errorf("adding columns: %w", err)
	}
	return tx.Commit(ctx)
}

func (c *Catalog) create(ctx context.Context, def datamodels.TableDef) (*table.Table, error) {
	spec, err := def.PartitionSpec()
	if err != nil {
		return nil, err
	}
	order, err := def.SortOrder()
	if err != nil {
		return nil, err
	}
	schema, err := def.Schema()
	if err != nil {
		return nil, err
	}
	props, err := def.Properties()
	if err != nil {
		return nil, err
	}
	return c.cat.CreateTable(ctx, def.Identifier(), schema,
		icecat.WithPartitionSpec(&spec),
		icecat.WithSortOrder(order),
		icecat.WithProperties(props),
	)
}

// inTableOrder returns rows relabelled with tbl's own schema: its column
// order and field IDs, matched by name. Rows built from a TableDef carry the
// def's field IDs (declaration order) in their Arrow field metadata, and
// iceberg-go maps written columns by those IDs; an evolved table (see
// evolve) numbers its added columns after the ones it had, so def-labelled
// rows would land values in the wrong columns. A table column missing from
// rows is written as nulls if optional; a rows column the table lacks, or a
// type mismatch, is an error. The caller releases the result.
func inTableOrder(tbl *table.Table, rows arrow.Table) (arrow.Table, error) {
	n := rows.NumRows()
	tsc, err := table.SchemaToArrowSchema(tbl.Schema(), nil, true, false)
	if err != nil {
		return nil, err
	}
	outFields := tsc.Fields()
	cols := make([]arrow.Column, len(outFields))
	release := func() {
		for i := range cols {
			if cols[i].Data() != nil {
				cols[i].Release()
			}
		}
	}
	used := 0
	for i, f := range outFields {
		idx := rows.Schema().FieldIndices(f.Name)
		switch {
		case len(idx) == 1:
			data := rows.Column(idx[0]).Data()
			if !arrow.TypeEqual(data.DataType(), f.Type) {
				release()
				return nil, fmt.Errorf("column %q is %s, table has %s", f.Name, data.DataType(), f.Type)
			}
			cols[i] = *arrow.NewColumn(f, data)
			used++
		case !f.Nullable:
			release()
			return nil, fmt.Errorf("rows lack required column %q", f.Name)
		default:
			nulls := array.MakeArrayOfNull(memory.DefaultAllocator, f.Type, int(n))
			chunked := arrow.NewChunked(f.Type, []arrow.Array{nulls})
			nulls.Release()
			cols[i] = *arrow.NewColumn(f, chunked)
			chunked.Release()
		}
	}
	if used != int(rows.NumCols()) {
		release()
		return nil, fmt.Errorf("rows have columns the table lacks")
	}
	out := array.NewTable(tsc, cols, n)
	release()
	return out, nil
}

// appendBatchRows is the row count per write batch when appending.
const appendBatchRows = 1 << 20

// Append writes a full-row Arrow table (a file parser's output for def) as
// one snapshot, replacing rather than adding to what is already there: for
// each stream_edid in rows, existing rows of that stream whose
// def.TimeColumn falls within the stream's time range in rows are deleted in
// the same commit (see ReplaceFilter). Re-ingesting a file therefore leaves
// the table as one copy of its rows, and re-ingesting it with a fixed parser
// replaces the old output.
//
// This assumes files of one stream do not overlap in time: a file whose
// range overlaps another file of the same stream replaces that file's rows
// in the overlap. Only the new rows' range is replaced, so if a re-parse
// covers a narrower range than before, the old rows outside it remain.
func (c *Catalog) Append(ctx context.Context, def datamodels.TableDef, rows arrow.Table, opts ...AppendOption) (int64, error) {
	b, err := c.NewBatch(ctx, def)
	if err != nil {
		return 0, err
	}
	if _, err := b.Append(ctx, rows, opts...); err != nil {
		return 0, err
	}
	return b.Commit(ctx)
}

// A Batch stages appends to one table and commits them together: each
// Append is its own snapshot, with its own summary properties, as with
// Catalog.Append, but the catalog is updated once, at Commit. Each staged
// Append sees the ones before it, so a batch leaves the table as the same
// appends committed one by one would. Staged rows are written to storage at
// Append; only the commit is deferred.
type Batch struct {
	c      *Catalog
	def    datamodels.TableDef
	tbl    *table.Table
	tx     *table.Transaction
	staged int64 // rows staged since the last Commit
	n      int   // appends staged since the last Commit
}

// NewBatch starts a batch of appends to def's table, creating or evolving
// the table as Table does.
func (c *Catalog) NewBatch(ctx context.Context, def datamodels.TableDef) (*Batch, error) {
	tbl, err := c.Table(ctx, def)
	if err != nil {
		return nil, err
	}
	return &Batch{c: c, def: def, tbl: tbl, tx: tbl.NewTransaction()}, nil
}

// Append stages rows as one snapshot, as Catalog.Append would commit them,
// and returns the rows staged. An error before anything is staged leaves
// the batch as it was; Failed reports whether an error left it unusable.
func (b *Batch) Append(ctx context.Context, rows arrow.Table, opts ...AppendOption) (int64, error) {
	if rows.NumRows() == 0 {
		return 0, nil
	}
	if b.tx == nil {
		return 0, fmt.Errorf("%s: batch failed earlier", b.def.Name)
	}
	if err := b.def.Validate(rows); err != nil {
		return 0, err
	}
	var o appendOptions
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return 0, err
		}
	}
	filter, err := ReplaceFilter(b.def, rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", b.def.Name, err)
	}
	ordered, err := inTableOrder(b.tbl, rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", b.def.Name, err)
	}
	defer ordered.Release()
	if err := b.tx.OverwriteTable(ctx, ordered, appendBatchRows, o.props, table.WithOverwriteFilter(filter)); err != nil {
		b.tx = nil // the transaction may hold part of this write
		return 0, fmt.Errorf("writing %s: %w", b.def.Name, err)
	}
	b.staged += rows.NumRows()
	b.n++
	return rows.NumRows(), nil
}

// Failed reports whether an Append error left the batch unable to stage
// or commit; what it had staged is lost.
func (b *Batch) Failed() bool { return b.tx == nil }

// Staged is the number of appends staged since the last Commit.
func (b *Batch) Staged() int { return b.n }

// Commit commits the staged appends in one catalog update and returns
// their rows. The batch can then stage more.
func (b *Batch) Commit(ctx context.Context) (int64, error) {
	if b.tx == nil {
		return 0, fmt.Errorf("%s: batch failed earlier", b.def.Name)
	}
	if b.n == 0 {
		return 0, nil
	}
	updated, err := b.tx.Commit(ctx)
	b.n, b.tx = 0, nil
	if err != nil {
		return 0, fmt.Errorf("committing %s: %w", b.def.Name, err)
	}
	n := b.staged
	b.staged = 0
	b.tbl, b.tx = updated, updated.NewTransaction()
	b.c.tables[b.def.Namespace()+"."+b.def.Name] = updated
	return n, nil
}
