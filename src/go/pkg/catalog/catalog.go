// Package catalog reads and writes the SFG Iceberg tables.
//
// It follows the catalog's own hierarchy: a Catalog holds Databases
// (Iceberg namespaces), which hold Tables. What exists is declared in
// datamodels (CatalogDef, DatabaseDef, TableDef); where it lives is a
// Config, so the same definitions build and serve AWS Glue, a REST
// catalog, or a local SQLite warehouse. Build makes the catalog hold a
// definition and Load opens what it already holds; either returns the
// live tree, in which databases and tables are found by name:
//
//	cat, err := catalog.Build(ctx, catalog.Config{Name: "glue"}, datamodels.SFG) // or {Name: catalog.Local, Warehouse: dir}
//	db, err := cat.Database("gnssa_l0")
//	tbl, err := db.Table("inspva")
//	n, err := tbl.Append(ctx, rows)
package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	_ "github.com/apache/iceberg-go/catalog/glue"
	_ "github.com/apache/iceberg-go/catalog/hadoop"
	_ "github.com/apache/iceberg-go/catalog/rest"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	"github.com/apache/iceberg-go/config"
	_ "github.com/apache/iceberg-go/io/gocloud" // s3://, gs:// and abfs[s]:// table IO
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

// Catalog is a connection to one Iceberg catalog (see Config), holding the
// Databases of the definition it was built or loaded with. It is not safe
// for concurrent use.
type Catalog struct {
	def datamodels.CatalogDef
	cat icecat.Catalog
	dbs map[string]*Database
}

// Build opens the catalog cfg names and makes it hold everything def
// declares: it creates each missing database and table, evolves existing
// tables to their definitions (see evolve), and opens them all. It never
// drops or rewrites data, so building again changes nothing.
func Build(ctx context.Context, cfg Config, def datamodels.CatalogDef) (*Catalog, error) {
	cat, err := openIceberg(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return connect(ctx, cat, def, buildDatabase)
}

// Load opens the catalog cfg names and the tables of def it already holds,
// at their latest snapshots, for reading: it creates and evolves nothing.
// A declared table the catalog lacks is left out; Database.Table reports
// it (ErrNotInCatalog).
func Load(ctx context.Context, cfg Config, def datamodels.CatalogDef) (*Catalog, error) {
	if cfg.Name == Local {
		// Reading must not create a warehouse (openIceberg would).
		db := filepath.Join(cfg.Warehouse, "catalog.db")
		if _, err := os.Stat(db); err != nil {
			return nil, fmt.Errorf("local warehouse %s: no catalog.db: %w", cfg.Warehouse, err)
		}
	}
	cat, err := openIceberg(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return connect(ctx, cat, def, loadDatabase)
}

// connect opens each of def's databases in cat with open. It closes cat
// on error.
func connect(ctx context.Context, cat icecat.Catalog, def datamodels.CatalogDef,
	open func(context.Context, icecat.Catalog, datamodels.DatabaseDef) (*Database, error)) (*Catalog, error) {
	c := &Catalog{def: def, cat: cat, dbs: make(map[string]*Database, len(def.Databases))}
	for _, dbdef := range def.Databases {
		if _, dup := c.dbs[dbdef.Name]; dup {
			c.Close()
			return nil, fmt.Errorf("catalog %s declares database %s twice", def.Name, dbdef.Name)
		}
		db, err := open(ctx, cat, dbdef)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.dbs[dbdef.Name] = db
	}
	return c, nil
}

// openIceberg opens the iceberg-go catalog cfg names, creating a Local
// warehouse if needed.
func openIceberg(ctx context.Context, cfg Config) (icecat.Catalog, error) {
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
	return cat, nil
}

// Check reports whether name is a catalog Build and Load can find: Local, a catalog of
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

// Def is the definition the catalog was built or loaded with.
func (c *Catalog) Def() datamodels.CatalogDef { return c.def }

// Database returns the database named name. It does no I/O.
func (c *Catalog) Database(name string) (*Database, error) {
	if db, ok := c.dbs[name]; ok {
		return db, nil
	}
	return nil, fmt.Errorf("catalog %s has no database %q", c.def.Name, name)
}

// TableOf returns def's table in the catalog's database that declares it
// (matched by name and level, see datamodels.CatalogDef.DatabaseOf); a
// shorthand for Database(name).Table(def.Name). It does no I/O.
func (c *Catalog) TableOf(def datamodels.TableDef) (*Table, error) {
	dbdef, ok := c.def.DatabaseOf(def)
	if !ok {
		return nil, fmt.Errorf("catalog %s declares no table %s (level %d)", c.def.Name, def.Name, def.Level)
	}
	db, err := c.Database(dbdef.Name)
	if err != nil {
		return nil, err
	}
	return db.Table(def.Name)
}
