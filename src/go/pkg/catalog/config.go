package catalog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/glue"
	_ "github.com/apache/iceberg-go/catalog/hadoop"
	_ "github.com/apache/iceberg-go/catalog/rest"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	"github.com/apache/iceberg-go/config"
	icebergio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud" // s3://, gs:// and abfs[s]:// table IO
	"github.com/uptrace/bun/driver/sqliteshim"
)

// Kind is the kind of Iceberg catalog a Config connects to.
type Kind string

const (
	// Local is a warehouse directory on this machine: an SQLite catalog in
	// <Warehouse>/catalog.db and the table data beside it. No server, no
	// credentials; for development, tests and one-off conversions.
	Local Kind = "local"
	// Glue is the AWS Glue Data Catalog of the account the AWS default
	// credential chain resolves (env, profile, SSO, instance role), with
	// table data on S3 under Warehouse. A "glue" catalog of the iceberg-go
	// configuration file, if there is one, supplies defaults (its warehouse).
	Glue Kind = "glue"
	// Configured is a catalog of the iceberg-go configuration file,
	// .iceberg-go.yaml in $GOICEBERG_HOME or the home directory, which gives
	// its type (rest, glue, sql, hadoop), URI, credential and warehouse.
	// Use it for REST catalogs and anything Local and Glue don't cover.
	Configured Kind = "configured"
)

// Config says how to connect to one Iceberg catalog. Build one with
// LocalConfig, GlueConfig or ConfiguredConfig, or from a --catalog flag with
// ParseConfig:
//
//	catalog.LocalConfig("./warehouse")
//	catalog.GlueConfig("s3://bucket/sfg").WithRegion("us-east-2")
//	catalog.ConfiguredConfig("prod-rest") // a catalog of .iceberg-go.yaml
//
// Fields not used by the Kind must be empty (see Validate).
type Config struct {
	Kind Kind

	// Warehouse is where table data lives.
	//   Local: the directory, created if needed (required).
	//   Glue: the S3 prefix new tables are placed under, e.g.
	//     s3://bucket/sfg (required to Build; Load reads each table's own
	//     location).
	//   Configured: overrides the configured warehouse; "" keeps it.
	Warehouse string

	// Region is the AWS region of a Glue catalog and its S3 data; "" takes
	// the default chain's (AWS_REGION, the profile). Glue only.
	Region string
	// CatalogID is the AWS account ID owning a Glue catalog; "" is the
	// caller's own account. Glue only.
	CatalogID string

	// Name is the catalog of the iceberg-go configuration file; "" is its
	// default-catalog. Configured only.
	Name string

	// Props are extra catalog and FileIO properties, applied last so they
	// override everything above, e.g. glue.endpoint or s3.endpoint for a
	// local AWS emulator.
	Props iceberg.Properties
}

// LocalConfig is the Local catalog in warehouse directory dir.
func LocalConfig(dir string) Config { return Config{Kind: Local, Warehouse: dir} }

// GlueConfig is the Glue catalog placing new tables under warehouse, an
// S3 prefix such as s3://bucket/sfg ("" to only Load).
func GlueConfig(warehouse string) Config { return Config{Kind: Glue, Warehouse: warehouse} }

// ConfiguredConfig is catalog name of the iceberg-go configuration file
// ("" for its default-catalog).
func ConfiguredConfig(name string) Config { return Config{Kind: Configured, Name: name} }

// WithRegion is cfg in AWS region region (Glue).
func (cfg Config) WithRegion(region string) Config { cfg.Region = region; return cfg }

// ParseConfig is the Config of a --catalog flag and an optional
// --warehouse: "local" is Local (warehouse required), "glue" is Glue, and
// any other name, or "", is that catalog of the iceberg-go configuration.
func ParseConfig(name, warehouse string) (Config, error) {
	var cfg Config
	switch Kind(name) {
	case Local:
		cfg = LocalConfig(warehouse)
	case Glue:
		cfg = GlueConfig(warehouse)
	default:
		cfg = ConfiguredConfig(name)
		cfg.Warehouse = warehouse
	}
	return cfg, cfg.Validate()
}

// Validate reports a Config that cannot connect: an unknown Kind, a Local
// catalog without a directory, fields of another Kind, or a Configured
// catalog the iceberg-go configuration does not have. It does no I/O.
func (cfg Config) Validate() error {
	switch cfg.Kind {
	case Local:
		if cfg.Warehouse == "" {
			return errors.New("a local catalog needs a warehouse directory")
		}
	case Glue:
	case Configured:
		if cfg.Name != "" {
			if _, ok := config.EnvConfig.Catalogs[cfg.Name]; !ok {
				return fmt.Errorf("no catalog %q in the iceberg-go configuration (.iceberg-go.yaml in $GOICEBERG_HOME or the home directory); want %s, %s or one of its catalogs", cfg.Name, Local, Glue)
			}
		}
	default:
		return fmt.Errorf("unknown catalog kind %q: want %s, %s or %s", cfg.Kind, Local, Glue, Configured)
	}
	if cfg.Kind != Glue && (cfg.Region != "" || cfg.CatalogID != "") {
		return fmt.Errorf("%s catalog: Region and CatalogID are for %s", cfg.Kind, Glue)
	}
	if cfg.Kind != Configured && cfg.Name != "" {
		return fmt.Errorf("%s catalog: Name is for %s", cfg.Kind, Configured)
	}
	return nil
}

// DefaultWarehouse is cfg's Warehouse, or for a Glue or Configured catalog
// without one, the warehouse the iceberg-go configuration gives it ("" if
// none).
func (cfg Config) DefaultWarehouse() string {
	if cfg.Warehouse != "" || cfg.Kind == Local {
		return cfg.Warehouse
	}
	name := cfg.Name
	if cfg.Kind == Glue {
		name = "glue"
	} else if name == "" {
		name = config.EnvConfig.DefaultCatalog
	}
	return config.EnvConfig.Catalogs[name].Warehouse
}

// String describes the catalog for messages: kind[:name][:warehouse].
func (cfg Config) String() string {
	s := string(cfg.Kind)
	if cfg.Kind == Configured {
		name := cfg.Name
		if name == "" {
			name = "default"
		}
		s += ":" + name
	}
	if cfg.Warehouse != "" {
		s += ":" + cfg.Warehouse
	}
	return s
}

// exists reports an error when cfg is a Local warehouse with no catalog.db,
// so that reading does not create one (open would).
func (cfg Config) exists() error {
	if cfg.Kind != Local {
		return nil
	}
	db := filepath.Join(cfg.Warehouse, "catalog.db")
	if _, err := os.Stat(db); err != nil {
		return fmt.Errorf("local warehouse %s: no catalog.db: %w", cfg.Warehouse, err)
	}
	return nil
}

// open connects to the catalog, creating a Local warehouse if needed.
func (cfg Config) open(ctx context.Context) (icecat.Catalog, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	var (
		name  string
		props iceberg.Properties
		err   error
	)
	switch cfg.Kind {
	case Local:
		// The sql catalog keys its rows by catalog name; existing local
		// warehouses and pyiceberg's load_catalog("default", ...) use "default".
		name = "default"
		props, err = localProps(cfg.Warehouse)
		if err != nil {
			return nil, err
		}
	case Glue:
		name = "glue"
		props = glueProps(cfg)
	case Configured:
		// icecat.Load merges these over the configuration file's catalog.
		name = cfg.Name
		props = iceberg.Properties{}
		if cfg.Warehouse != "" {
			props["warehouse"] = cfg.Warehouse
		}
	}
	maps.Copy(props, cfg.Props)
	cat, err := icecat.Load(ctx, name, props)
	if err != nil {
		return nil, fmt.Errorf("opening catalog %s: %w", cfg, err)
	}
	return cat, nil
}

// localProps are the sql catalog properties of a Local warehouse directory,
// which is created if needed.
func localProps(dir string) (iceberg.Properties, error) {
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

// glueProps are the glue catalog properties of cfg. Credentials come from
// the AWS default chain, or from Props (glue.access-key-id, ...).
func glueProps(cfg Config) iceberg.Properties {
	props := iceberg.Properties{"type": "glue"}
	if cfg.Warehouse != "" {
		props["warehouse"] = cfg.Warehouse
	}
	if cfg.Region != "" {
		props[glue.Region] = cfg.Region
		props[icebergio.S3Region] = cfg.Region
	}
	if cfg.CatalogID != "" {
		props[glue.CatalogIdKey] = cfg.CatalogID
	}
	return props
}
