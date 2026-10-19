package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	"github.com/apache/iceberg-go/table"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
)

// Catalog is a SQLite-backed Iceberg catalog over a local warehouse
// directory: catalog.db holds the catalog, table data lives beside it.
type Catalog struct {
	cat    *sqlcat.Catalog
	db     *sql.DB
	tables map[string]*table.Table
}

// Open opens (creating if needed) the catalog in the warehouse directory.
func Open(warehouse string) (*Catalog, error) {
	warehouse, err := filepath.Abs(warehouse)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(warehouse, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open(sqliteshim.ShimName, filepath.Join(warehouse, "catalog.db"))
	if err != nil {
		return nil, err
	}
	cat, err := sqlcat.NewCatalog("default", db, sqlcat.SQLite, iceberg.Properties{
		"warehouse": "file://" + warehouse,
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Catalog{cat: cat, db: db, tables: make(map[string]*table.Table)}, nil
}

// Close closes the catalog database.
func (c *Catalog) Close() error { return c.db.Close() }

// Table loads def's table, creating its namespace and table if needed.
func (c *Catalog) Table(ctx context.Context, def schemas.TableDef) (*table.Table, error) {
	key := def.Namespace() + "." + def.Name
	if tbl, ok := c.tables[key]; ok {
		return tbl, nil
	}
	ns := table.Identifier{def.Namespace()}
	err := c.cat.CreateNamespace(ctx, ns, nil)
	if err != nil && !errors.Is(err, icecat.ErrNamespaceAlreadyExists) {
		return nil, fmt.Errorf("creating namespace %s: %w", def.Namespace(), err)
	}
	tbl, err := c.cat.LoadTable(ctx, def.Identifier())
	if errors.Is(err, icecat.ErrNoSuchTable) {
		tbl, err = c.create(ctx, def)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	c.tables[key] = tbl
	return tbl, nil
}

func (c *Catalog) create(ctx context.Context, def schemas.TableDef) (*table.Table, error) {
	spec, err := def.PartitionSpec()
	if err != nil {
		return nil, err
	}
	order, err := def.SortOrder()
	if err != nil {
		return nil, err
	}
	return c.cat.CreateTable(ctx, def.Identifier(), def.Schema(),
		icecat.WithPartitionSpec(&spec),
		icecat.WithSortOrder(order),
		icecat.WithProperties(iceberg.Properties{"comment": def.Doc}),
	)
}

// appendBatchRows is the row count per write batch when appending.
const appendBatchRows = 1 << 20

// Append appends a full-row Arrow table (a file parser's output for def) as
// one snapshot. Re-appending the same file duplicates its rows.
func (c *Catalog) Append(ctx context.Context, def schemas.TableDef, rows arrow.Table) (int64, error) {
	if rows.NumRows() == 0 {
		return 0, nil
	}
	if err := def.Validate(rows); err != nil {
		return 0, err
	}
	tbl, err := c.Table(ctx, def)
	if err != nil {
		return 0, err
	}
	updated, err := tbl.AppendTable(ctx, rows, appendBatchRows, nil)
	if err != nil {
		return 0, fmt.Errorf("appending to %s: %w", def.Name, err)
	}
	c.tables[def.Namespace()+"."+def.Name] = updated
	return rows.NumRows(), nil
}
