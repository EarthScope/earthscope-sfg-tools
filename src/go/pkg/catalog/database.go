package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// ErrNotInCatalog is Database.Table's error for a table the database
// declares but a loaded catalog does not hold.
var ErrNotInCatalog = errors.New("not in the catalog")

// Database is one database (Iceberg namespace) of a Catalog, as its
// DatabaseDef declares it. It owns its Tables, opened when the catalog was
// built or loaded, and only it names them in the catalog.
type Database struct {
	def    datamodels.DatabaseDef
	tables map[string]*Table
}

// Name is the database's name, its Iceberg namespace.
func (d *Database) Name() string { return d.def.Name }

// Def is the database's definition.
func (d *Database) Def() datamodels.DatabaseDef { return d.def }

// Table returns the table named name. It does no I/O.
func (d *Database) Table(name string) (*Table, error) {
	if t, ok := d.tables[name]; ok {
		return t, nil
	}
	if d.def.Has(datamodels.TableDef{Name: name}) {
		return nil, fmt.Errorf("%s.%s: %w", d.def.Name, name, ErrNotInCatalog)
	}
	return nil, fmt.Errorf("database %s declares no table %q", d.def.Name, name)
}

// newDatabase is def's Database with no tables open yet. It rejects a
// table name declared twice, which would otherwise shadow the first.
func newDatabase(def datamodels.DatabaseDef) (*Database, error) {
	seen := make(map[string]bool, len(def.Tables))
	for _, t := range def.Tables {
		if seen[t.Name] {
			return nil, fmt.Errorf("database %s declares table %s twice", def.Name, t.Name)
		}
		seen[t.Name] = true
	}
	return &Database{def: def, tables: make(map[string]*Table, len(def.Tables))}, nil
}

// buildDatabase creates def's namespace if it is missing, with def.Doc as
// its description ("comment"; Glue's Description), then creates or evolves
// each of its tables. A new namespace gets no location: the catalog places
// its tables at <warehouse>/<namespace>.db/<table>, or where the server
// decides.
func buildDatabase(ctx context.Context, cat icecat.Catalog, def datamodels.DatabaseDef) (*Database, error) {
	var props iceberg.Properties
	if def.Doc != "" {
		props = iceberg.Properties{"comment": def.Doc}
	}
	err := cat.CreateNamespace(ctx, table.Identifier{def.Name}, props)
	if err != nil && !errors.Is(err, icecat.ErrNamespaceAlreadyExists) {
		return nil, fmt.Errorf("creating database %s: %w", def.Name, err)
	}
	d, err := newDatabase(def)
	if err != nil {
		return nil, err
	}
	for _, tdef := range def.Tables {
		id := table.Identifier{def.Name, tdef.Name}
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
			return nil, fmt.Errorf("%s.%s: %w", def.Name, tdef.Name, err)
		}
		d.tables[tdef.Name] = &Table{def: tdef, tbl: tbl}
	}
	return d, nil
}

// loadDatabase opens the tables of def the catalog holds; it creates
// nothing, not even the namespace.
func loadDatabase(ctx context.Context, cat icecat.Catalog, def datamodels.DatabaseDef) (*Database, error) {
	d, err := newDatabase(def)
	if err != nil {
		return nil, err
	}
	for _, tdef := range def.Tables {
		tbl, err := cat.LoadTable(ctx, table.Identifier{def.Name, tdef.Name})
		if errors.Is(err, icecat.ErrNoSuchTable) || errors.Is(err, icecat.ErrNoSuchNamespace) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("loading %s.%s: %w", def.Name, tdef.Name, err)
		}
		d.tables[tdef.Name] = &Table{def: tdef, tbl: tbl}
	}
	return d, nil
}

// create creates def's table at id with its schema, partition spec, sort
// order and properties.
func create(ctx context.Context, cat icecat.Catalog, id table.Identifier, def datamodels.TableDef) (*table.Table, error) {
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
func evolve(ctx context.Context, tbl *table.Table, def datamodels.TableDef) (*table.Table, error) {
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
func addMissingColumns(ctx context.Context, tbl *table.Table, def datamodels.TableDef) (*table.Table, error) {
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
