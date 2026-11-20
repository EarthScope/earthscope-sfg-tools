// Package catalog reads and writes the SFG Iceberg tables.
//
// It follows the catalog's own hierarchy: a Catalog holds Databases
// (Iceberg namespaces), which hold Tables. What exists is declared by a
// CatalogDef, DatabaseDefs and TableDefs (definitions.go; package
// datamodels declares the SFG ones); where it lives is a Config
// (config.go: Local, Glue or Configured), so the same definitions build and
// serve a local SQLite warehouse, AWS Glue, or a REST catalog. Build makes
// the catalog hold a definition and Load opens what it already holds;
// either returns the live tree (catalog.go), in which databases and tables
// are found by name:
//
//	cat, err := catalog.Build(ctx, catalog.GlueConfig("s3://bucket/sfg"), datamodels.SFG) // or catalog.LocalConfig(dir)
//	db, err := cat.Database("gnssa_l0") // the definition's name, whatever Config.DatabasePrefix says
//	tbl, err := db.Table("inspva") // or cat.TableOf(datamodels.GNSSA_L0.Ref(datamodels.INSPVA))
//	n, err := tbl.Append(ctx, rows)
package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"
)

// Catalog is a connection to one Iceberg catalog (see Config), holding the
// Databases of the definition it was built or loaded with. It is not safe
// for concurrent use.
type Catalog struct {
	def CatalogDef
	cat icecat.Catalog
	dbs map[string]*Database
}

// Build opens the catalog cfg names and makes it hold everything def
// declares: it creates each missing database and table, evolves existing
// tables to their definitions (see evolve), and opens them all. It never
// drops or rewrites data, so building again changes nothing.
func Build(ctx context.Context, cfg Config, def CatalogDef) (*Catalog, error) {
	cat, err := cfg.open(ctx)
	if err != nil {
		return nil, err
	}
	return connect(ctx, cat, cfg.DatabasePrefix, def, buildDatabase)
}

// Load opens the catalog cfg names and the tables of def it already holds,
// at their latest snapshots, for reading: it creates and evolves nothing.
// A declared table the catalog lacks is left out; Database.Table reports
// it (ErrNotInCatalog).
func Load(ctx context.Context, cfg Config, def CatalogDef) (*Catalog, error) {
	if err := cfg.exists(); err != nil {
		return nil, err
	}
	cat, err := cfg.open(ctx)
	if err != nil {
		return nil, err
	}
	return connect(ctx, cat, cfg.DatabasePrefix, def, loadDatabase)
}

// connect opens each of def's databases in cat, in the namespace prefix +
// its name, with open. It closes cat on error.
func connect(ctx context.Context, cat icecat.Catalog, prefix string, def CatalogDef,
	open func(context.Context, icecat.Catalog, string, DatabaseDef) (*Database, error)) (*Catalog, error) {
	c := &Catalog{def: def, cat: cat, dbs: make(map[string]*Database, len(def.Databases))}
	for _, dbdef := range def.Databases {
		if _, dup := c.dbs[dbdef.Name]; dup {
			c.Close()
			return nil, fmt.Errorf("catalog %s declares database %s twice", def.Name, dbdef.Name)
		}
		db, err := open(ctx, cat, prefix, dbdef)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.dbs[dbdef.Name] = db
	}
	return c, nil
}

// Close closes the catalog's connections.
func (c *Catalog) Close() error {
	if closer, ok := c.cat.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Def is the definition the catalog was built or loaded with.
func (c *Catalog) Def() CatalogDef { return c.def }

// Database returns the database the definition names name (without any
// DatabasePrefix). It does no I/O.
func (c *Catalog) Database(name string) (*Database, error) {
	if db, ok := c.dbs[name]; ok {
		return db, nil
	}
	return nil, fmt.Errorf("catalog %s has no database %q", c.def.Name, name)
}

// TableOf returns ref's table: Database(ref.Database).Table(ref.Def.Name).
// It does no I/O.
func (c *Catalog) TableOf(ref TableRef) (*Table, error) {
	db, err := c.Database(ref.Database)
	if err != nil {
		return nil, err
	}
	return db.Table(ref.Def.Name)
}

// ErrNotInCatalog is Database.Table's error for a table the database
// declares but a loaded catalog does not hold.
var ErrNotInCatalog = errors.New("not in the catalog")

// Database is one database (Iceberg namespace) of a Catalog, as its
// DatabaseDef declares it. It owns its Tables, opened when the catalog was
// built or loaded, and only it names them in the catalog.
type Database struct {
	def    DatabaseDef
	prefix string // Config.DatabasePrefix
	tables map[string]*Table
}

// Namespace is the database's Iceberg namespace in the catalog: its
// definition's name behind Config.DatabasePrefix.
func (d *Database) Namespace() string { return d.prefix + d.def.Name }

// Table returns the table named name. It does no I/O.
func (d *Database) Table(name string) (*Table, error) {
	if t, ok := d.tables[name]; ok {
		return t, nil
	}
	if d.def.Has(TableDef{Name: name}) {
		return nil, fmt.Errorf("%s.%s: %w", d.def.Name, name, ErrNotInCatalog)
	}
	return nil, fmt.Errorf("database %s declares no table %q", d.def.Name, name)
}

// newDatabase is def's Database with no tables open yet. It rejects a
// table name declared twice, which would otherwise shadow the first.
func newDatabase(prefix string, def DatabaseDef) (*Database, error) {
	seen := make(map[string]bool, len(def.Tables))
	for _, t := range def.Tables {
		if seen[t.Name] {
			return nil, fmt.Errorf("database %s declares table %s twice", def.Name, t.Name)
		}
		seen[t.Name] = true
	}
	return &Database{def: def, prefix: prefix, tables: make(map[string]*Table, len(def.Tables))}, nil
}

// buildDatabase creates def's namespace (prefix + def.Name) if it is missing, with def.Doc as
// its description ("comment"; Glue's Description), then creates or evolves
// each of its tables. A new namespace gets no location: the catalog places
// its tables at <warehouse>/<namespace>.db/<table>, or where the server
// decides.
func buildDatabase(ctx context.Context, cat icecat.Catalog, prefix string, def DatabaseDef) (*Database, error) {
	ns := prefix + def.Name
	var props iceberg.Properties
	if def.Doc != "" {
		props = iceberg.Properties{"comment": def.Doc}
	}
	err := cat.CreateNamespace(ctx, table.Identifier{ns}, props)
	if err != nil && !errors.Is(err, icecat.ErrNamespaceAlreadyExists) {
		return nil, fmt.Errorf("creating database %s: %w", ns, err)
	}
	d, err := newDatabase(prefix, def)
	if err != nil {
		return nil, err
	}
	for _, tdef := range def.Tables {
		id := table.Identifier{ns, tdef.Name}
		tbl, err := cat.LoadTable(ctx, id)
		if errors.Is(err, icecat.ErrNoSuchTable) {
			tbl, err = create(ctx, cat, id, tdef)
			if errors.Is(err, icecat.ErrTableAlreadyExists) {
				// Another writer created it since; use theirs.
				tbl, err = cat.LoadTable(ctx, id)
			}
		}
		if err == nil {
			tbl, err = evolve(ctx, tbl, tdef)
		}
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", ns, tdef.Name, err)
		}
		d.tables[tdef.Name] = &Table{def: tdef, tbl: tbl}
	}
	return d, nil
}

// loadDatabase opens the tables of def the catalog holds; it creates
// nothing, not even the namespace.
func loadDatabase(ctx context.Context, cat icecat.Catalog, prefix string, def DatabaseDef) (*Database, error) {
	d, err := newDatabase(prefix, def)
	if err != nil {
		return nil, err
	}
	for _, tdef := range def.Tables {
		tbl, err := cat.LoadTable(ctx, table.Identifier{d.Namespace(), tdef.Name})
		if errors.Is(err, icecat.ErrNoSuchTable) || errors.Is(err, icecat.ErrNoSuchNamespace) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("loading %s.%s: %w", d.Namespace(), tdef.Name, err)
		}
		d.tables[tdef.Name] = &Table{def: tdef, tbl: tbl}
	}
	return d, nil
}

// create creates def's table at id with its schema, partition spec, sort
// order and properties.
func create(ctx context.Context, cat icecat.Catalog, id table.Identifier, def TableDef) (*table.Table, error) {
	spec, err := def.PartitionSpec()
	if err != nil {
		return nil, err
	}
	order, err := def.SortOrder()
	if err != nil {
		return nil, err
	}
	schema, err := def.IcebergSchema()
	if err != nil {
		return nil, err
	}
	props, err := def.Properties()
	if err != nil {
		return nil, err
	}
	return cat.CreateTable(ctx, id, schema,
		icecat.WithPartitionSpec(&spec),
		icecat.WithSortOrder(order),
		icecat.WithProperties(props),
	)
}

// evolve adds to tbl the columns def has and tbl lacks, so a table created by
// an earlier version of def takes new optional columns; existing rows read
// them as null. A missing required column is an error: old rows would
// violate it. Columns def no longer has are left alone. Tables defined by
// another library (def.External) are not evolved.
//
// A commit that loses to a concurrent writer (e.g. another worker evolving
// the same table) is retried from the table's new metadata, so concurrent
// Builds all succeed.
func evolve(ctx context.Context, tbl *table.Table, def TableDef) (*table.Table, error) {
	if def.External != nil {
		return tbl, nil
	}
	const attempts = 3
	for i := 1; ; i++ {
		updated, err := addMissingColumns(ctx, tbl, def)
		if err == nil || !errors.Is(err, table.ErrCommitFailed) || i == attempts {
			return updated, err
		}
		if err := tbl.Refresh(ctx); err != nil {
			return nil, fmt.Errorf("reloading after a conflicting commit: %w", err)
		}
	}
}

// addMissingColumns commits one schema update adding def's columns tbl lacks.
func addMissingColumns(ctx context.Context, tbl *table.Table, def TableDef) (*table.Table, error) {
	var missing []iceberg.NestedField
	for _, col := range def.Schema.Fields() {
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

// Table is one Iceberg table and the definition it was opened with. It
// knows nothing of the Database or Catalog that opened it. Commits through
// it (Append, Replace) move it to the new snapshot. It is not safe for
// concurrent use.
type Table struct {
	def TableDef
	tbl *table.Table
}

// appendBatchRows is the row count per write batch when appending.
const appendBatchRows = 1 << 20

// AppendOption configures one Append.
type AppendOption func(*appendOptions)

type appendOptions struct {
	props iceberg.Properties
}

// WithProperties adds props to the summary of the snapshot Append commits,
// e.g. the source file or the scope it was parsed for.
func WithProperties(props map[string]string) AppendOption {
	return func(o *appendOptions) {
		if o.props == nil {
			o.props = iceberg.Properties{}
		}
		for k, v := range props {
			o.props[k] = v
		}
	}
}

// Def is the table's definition.
func (t *Table) Def() TableDef { return t.def }

// Iceberg is the underlying iceberg-go table, at the snapshot the Table last
// read or committed.
func (t *Table) Iceberg() *table.Table { return t.tbl }

// Append writes a full-row Arrow table (a file parser's output for the
// table's definition) as one snapshot and returns the number of rows
// written. It only adds rows: appending the same file twice duplicates its
// rows (Replace is the idempotent write). Rows are matched to the table's columns by name (see inTableOrder),
// so a table evolved since its definition was written is filled correctly.
// The Table moves to the new snapshot.
func (t *Table) Append(ctx context.Context, rows arrow.Table, opts ...AppendOption) (int64, error) {
	var o appendOptions
	for _, opt := range opts {
		opt(&o)
	}
	n := rows.NumRows()
	if n == 0 {
		return 0, nil
	}
	// Write with the table's own column order and field IDs, not the
	// definition's: they differ once the table has evolved.
	ordered, err := inTableOrder(t.tbl, rows)
	if err != nil {
		return 0, fmt.Errorf("appending to %s: %w", t.def.Name, err)
	}
	defer ordered.Release()
	tx := t.tbl.NewTransaction()
	if err := tx.AppendTable(ctx, ordered, appendBatchRows, o.props); err != nil {
		return 0, fmt.Errorf("appending to %s: %w", t.def.Name, err)
	}
	tbl, err := tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("committing to %s: %w", t.def.Name, err)
	}
	t.tbl = tbl
	return n, nil
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
