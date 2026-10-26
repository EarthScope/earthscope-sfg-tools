package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/glue"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	_ "github.com/apache/iceberg-go/io/gocloud" // s3:// table IO for glue
	"github.com/apache/iceberg-go/table"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// Catalog types for Config.Type.
const (
	TypeSQL  = "sql"  // SQLite catalog.db in a local warehouse directory
	TypeGlue = "glue" // AWS Glue catalog over an S3 warehouse
)

// Config selects and locates the Iceberg catalog.
type Config struct {
	Type      string // TypeSQL or TypeGlue
	Warehouse string // local directory (sql) or s3://bucket/prefix (glue)
	Region    string // glue only; empty uses the AWS default chain
}

// Catalog is an Iceberg catalog holding the Level 1 tables: either SQLite
// over a local warehouse directory (catalog.db holds the catalog, table data
// lives beside it) or AWS Glue over an S3 warehouse.
type Catalog struct {
	cfg    Config
	cat    icecat.Catalog
	db     *sql.DB // sql only
	tables map[string]*table.Table
}

// Open opens (creating if needed, for sql) the catalog cfg describes. Glue
// credentials come from the standard AWS chain (env, profile, IAM role).
func Open(ctx context.Context, cfg Config) (*Catalog, error) {
	switch cfg.Type {
	case TypeSQL:
		return openSQL(cfg)
	case TypeGlue:
		if !strings.HasPrefix(cfg.Warehouse, "s3://") {
			return nil, fmt.Errorf("glue warehouse %q is not an s3:// URI", cfg.Warehouse)
		}
		props := iceberg.Properties{"warehouse": strings.TrimSuffix(cfg.Warehouse, "/")}
		if cfg.Region != "" {
			props[glue.Region] = cfg.Region
		}
		cat, err := icecat.Load(ctx, TypeGlue, props)
		if err != nil {
			return nil, fmt.Errorf("opening glue catalog: %w", err)
		}
		return &Catalog{cfg: cfg, cat: cat, tables: make(map[string]*table.Table)}, nil
	default:
		return nil, fmt.Errorf("unknown catalog type %q (want %q or %q)", cfg.Type, TypeSQL, TypeGlue)
	}
}

// openSQL opens the SQLite catalog in the warehouse directory. The catalog
// name stays "default" so existing warehouses and pyiceberg's
// load_catalog("default", ...) still see the tables.
func openSQL(cfg Config) (*Catalog, error) {
	warehouse, err := filepath.Abs(cfg.Warehouse)
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
	cfg.Warehouse = warehouse
	return &Catalog{cfg: cfg, cat: cat, db: db, tables: make(map[string]*table.Table)}, nil
}

// Close closes the catalog database (sql); it is a no-op for glue.
func (c *Catalog) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// Table loads def's table, creating its namespace and table if needed.
func (c *Catalog) Table(ctx context.Context, def datamodels.TableDef) (*table.Table, error) {
	key := def.Namespace() + "." + def.Name
	if tbl, ok := c.tables[key]; ok {
		return tbl, nil
	}
	ns := table.Identifier{def.Namespace()}
	// A Glue database needs an explicit location for its tables to land in
	// the S3 warehouse; the sql catalog derives it from its warehouse.
	var nsProps iceberg.Properties
	if c.cfg.Type == TypeGlue {
		nsProps = iceberg.Properties{"location": strings.TrimSuffix(c.cfg.Warehouse, "/") + "/" + def.Namespace() + ".db"}
	}
	err := c.cat.CreateNamespace(ctx, ns, nsProps)
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
	if rows.NumRows() == 0 {
		return 0, nil
	}
	if err := def.Validate(rows); err != nil {
		return 0, err
	}
	var o appendOptions
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return 0, err
		}
	}
	filter, err := ReplaceFilter(def, rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", def.Name, err)
	}
	tbl, err := c.Table(ctx, def)
	if err != nil {
		return 0, err
	}
	tx := tbl.NewTransaction()
	if err := tx.OverwriteTable(ctx, rows, appendBatchRows, o.props, table.WithOverwriteFilter(filter)); err != nil {
		return 0, fmt.Errorf("writing %s: %w", def.Name, err)
	}
	updated, err := tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("committing %s: %w", def.Name, err)
	}
	c.tables[def.Namespace()+"."+def.Name] = updated
	return rows.NumRows(), nil
}
