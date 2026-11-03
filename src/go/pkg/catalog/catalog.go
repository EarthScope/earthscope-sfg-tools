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
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/glue"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	icebergio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud" // s3:// table IO for glue
	"github.com/apache/iceberg-go/table"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
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
		// The catalog type comes from props["type"]; Load's name argument is
		// only the catalog's name. The table data IO reads these props too,
		// so the region is set for S3 as well as Glue.
		props := iceberg.Properties{"type": TypeGlue, "warehouse": strings.TrimSuffix(cfg.Warehouse, "/")}
		if cfg.Region != "" {
			props[glue.Region] = cfg.Region
			props[icebergio.S3Region] = cfg.Region
		}
		cat, err := icecat.Load(ctx, "default", props)
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
	ordered, err := inTableOrder(tbl, rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", def.Name, err)
	}
	defer ordered.Release()
	tx := tbl.NewTransaction()
	if err := tx.OverwriteTable(ctx, ordered, appendBatchRows, o.props, table.WithOverwriteFilter(filter)); err != nil {
		return 0, fmt.Errorf("writing %s: %w", def.Name, err)
	}
	updated, err := tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("committing %s: %w", def.Name, err)
	}
	c.tables[def.Namespace()+"."+def.Name] = updated
	return rows.NumRows(), nil
}
