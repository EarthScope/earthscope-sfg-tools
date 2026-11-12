// Package sink is the Level 0 write layer: a Sink takes the full-row Arrow
// table a file parser produced for a table def and writes it somewhere (an
// Iceberg catalog, Parquet files).
//
// Every Sink validates rows against def before writing, so callers cannot
// skip it. Write only borrows rows: the caller that got the table from a
// parser still owns it and must Release it.
//
// Orchestration (finding files, scope, retries) is the caller's job.
package sink

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// Sink writes one table's rows somewhere.
type Sink interface {
	// Write validates rows against def and writes them, returning the
	// number of rows written. It does not Release rows.
	Write(ctx context.Context, def datamodels.TableDef, rows arrow.Table) (int64, error)
	Close() error
}

// Iceberg writes rows to an Iceberg catalog, local (sql) or Glue.
type Iceberg struct {
	cat *catalog.Catalog
}

// OpenIceberg builds defs' tables (datamodels.SFG.Only) in the catalog cfg
// describes, creating what is missing, as a Sink that writes to them;
// Write rejects any other table. Other tables are left alone, so an
// unrelated or incompatible table cannot block these writes. Close closes
// the catalog.
func OpenIceberg(ctx context.Context, cfg catalog.Config, defs ...datamodels.TableDef) (*Iceberg, error) {
	if len(defs) == 0 {
		return nil, fmt.Errorf("iceberg sink: no tables to write")
	}
	cat, err := catalog.Build(ctx, cfg, datamodels.SFG.Only(defs...))
	if err != nil {
		return nil, err
	}
	return &Iceberg{cat: cat}, nil
}

// Catalog returns the underlying catalog.
func (s *Iceberg) Catalog() *catalog.Catalog { return s.cat }

// Write appends rows to def's table, in the SFG database that declares it,
// as one snapshot (Table.Append validates them).
func (s *Iceberg) Write(ctx context.Context, def datamodels.TableDef, rows arrow.Table) (int64, error) {
	tbl, err := s.cat.TableOf(def)
	if err != nil {
		return 0, err
	}
	return tbl.Append(ctx, rows)
}

// Close closes the catalog.
func (s *Iceberg) Close() error { return s.cat.Close() }

// Parquet writes each Write as a new file
// <dir>/<database>/<table>/<table>-<UTC timestamp>.parquet, zstd
// compressed, with the Arrow schema stored so types round-trip.
type Parquet struct {
	dir string
}

// NewParquet returns a Parquet sink writing under dir.
func NewParquet(dir string) *Parquet { return &Parquet{dir: dir} }

// Write validates rows and writes them to a new Parquet file.
func (s *Parquet) Write(_ context.Context, def datamodels.TableDef, rows arrow.Table) (int64, error) {
	if rows.NumRows() == 0 {
		return 0, nil
	}
	if err := def.Validate(rows); err != nil {
		return 0, err
	}
	dir := filepath.Join(s.dir, def.Name)
	if db, ok := datamodels.DatabaseOf(def); ok {
		dir = filepath.Join(s.dir, db.Name, def.Name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.parquet", def.Name, time.Now().UTC().Format("20060102T150405.000000000")))
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	// WriteTable closes f when it succeeds.
	err = pqarrow.WriteTable(rows, f, 1<<20,
		parquet.NewWriterProperties(parquet.WithCompression(compress.Codecs.Zstd)),
		pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema()),
	)
	if err != nil {
		f.Close()
		return 0, fmt.Errorf("writing %s: %w", path, err)
	}
	return rows.NumRows(), nil
}

// Close is a no-op: each Write closes its own file.
func (s *Parquet) Close() error { return nil }
