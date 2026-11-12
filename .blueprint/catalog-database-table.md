# Catalog → Database → Table

Blueprint for restructuring `src/go/pkg/catalog` so the hierarchy is a catalog that holds databases (Iceberg namespaces), which hold tables. Status: implemented 2026-11-12 on `ftd-iceberg`.

What the implementation added or changed from the design below:
- `datamodels.CatalogDef` (`SFG`) holds the `DatabaseDef`s; `catalog.Catalog.Build(ctx, def)` and `Database.Build(ctx)` create what is missing and evolve the rest, through the generic iceberg-go `Catalog` interface only, so the same call builds a local warehouse, Glue or REST. CLI: `sfg iceberg build --catalog NAME [--warehouse ...] [--database ...]`.
- `DatabaseOf` matches a table by name **and level** (garpos and community_standard are in both gnssa_l1 and gnssa_l2) and returns `(DatabaseDef, bool)`; `QualifiedName(def)` replaces `def.Namespace()+"."+def.Name`.
- `TableDef` holds `Schema *iceberg.Schema` and `Checks map[string]Check`; `IcebergSchema()` replaces `Schema()`.
- `Table.NewBatch()` takes no ctx; the namespace "comment" is the database's `Doc` (Glue's Description).

**Revised 2026-11-12 (after `dev/build_level0/main.go`):** construction returns the live tree, built eagerly from a definition; lookups are by name and do no I/O. This supersedes `Open`, `Catalog.Build`, `Database.Build`, `Database.Table(ctx, def)` and `Database.LoadTable` in the sections below.

```go
cat, err := catalog.Build(ctx, cfg, datamodels.SFG)   // create/evolve every declared table, open them all
cat, err := catalog.Load(ctx, cfg, datamodels.SFG.Only(datamodels.GNSSObs)) // read-only: open what exists, create nothing
db, err  := cat.Database("gnssa_l0")                  // lookup
tbl, err := db.Table("inspva")                        // lookup; ErrNotInCatalog if Load found it missing
tbl, err := cat.TableOf(datamodels.GarposL1)          // shorthand: declaring database (name+level), then table
```
- `CatalogDef.Only(defs...)` narrows a definition to the databases/tables declaring `defs` (kept as given), so callers build or load part of a catalog; tests use it to build an older schema.
- Build stays a `catalog` function rather than a method on the defs (as in main.go), so `datamodels` does no catalog I/O.
- Sinks (`sink.OpenIceberg`, convert's iceberg sink) build all of `SFG` on open; readers (`convert` iceberg source, `sfg level0 rinex`) `Load` only the tables they read.

## Decisions

| Question | Decision |
|---|---|
| Where a database's name comes from | Declared explicitly in `datamodels` as a `DatabaseDef`, which names the database and lists its tables. Names stay `gnssa_l0`, `gnssa_l1`, `gnssa_l2` (the existing Glue namespaces; no migration). `TableDef.Level` no longer decides the database. |
| Which database a table is in | The one `DatabaseDef` that lists it. Every table is in exactly one. |
| Who owns per-table operations | Our own `catalog.Table`: `Append`, `NewBatch`, `Scan`. |
| Who owns a table | `Database` holds its tables. A `Table` has no reference to its database or catalog. |
| Old `Catalog.Append/NewBatch/Scan/Table` | Removed; callers updated. One way to do each thing. |

## Concerns and boundaries

| Concern | Where it lives | Boundary |
|---|---|---|
| Table shape: columns, partitioning, sort order, level, time column | `datamodels.TableDef` | none (pure data) |
| Database membership: which tables make up each database | `datamodels.DatabaseDef` | none (pure data) |
| Connecting to a catalog: config file, local SQLite warehouse, cloud FileIO | `catalog.Config`, `catalog.Open` | network / filesystem |
| Namespace lifecycle | `catalog.Database` | catalog service |
| Table lifecycle: create, evolve, load | `catalog.Database` | catalog service |
| Writing rows: replace-by-stream-window, batching, snapshot properties | `catalog.Table`, `catalog.Batch` | object storage + catalog commit |
| Reading rows: selection → filter, column order | `catalog.Table.Scan`, `catalog.Selection` | object storage |
| Product exports (RINEX) | `catalog.WriteRINEX` (function) | `io.Writer` |
| Choosing which database a run reads/writes | `sink`, `convert`, `cmd/sfg`, by looking up the table's `DatabaseDef` | none: fixed by the definitions, no CLI flags |

## Layers and dependency direction

```
cmd/sfg  ──►  sink, convert  ──►  catalog  ──►  datamodels
 (interface)     (service)       (data access)   (specification)
```

- `datamodels` imports nothing from `catalog`, and no longer imports iceberg-go's `table` package for identifiers. It declares table shapes and which database each table belongs to, but not how to reach a catalog.
- `catalog` knows Iceberg. It does not know about CLI flags, file types, or conversion specs.
- `sink`/`convert` decide which database to use and hold the `*catalog.Database` / `*catalog.Table` handles.

## Types

```go
// Catalog: a connection to one Iceberg catalog. Owns its Databases.
type Catalog struct {
    cat icecat.Catalog
    dbs map[string]*Database // by name
}

func Open(ctx context.Context, cfg Config) (*Catalog, error) // unchanged
func Check(name string) error                                // unchanged
func Warehouse(name string) string                           // unchanged
func (c *Catalog) Close() error

// Database returns the handle for def's namespace. It does no I/O: the
// namespace is created on the first Database.Table. Repeated calls with
// the same def return the same *Database.
func (c *Catalog) Database(def datamodels.DatabaseDef) *Database

// Databases lists the catalog's namespaces (for CLI listing/validation).
func (c *Catalog) Databases(ctx context.Context) ([]string, error)
```

```go
// Database: one Iceberg namespace. Owns its Tables.
type Database struct {
    cat    icecat.Catalog   // shared with the Catalog; not a *Catalog back-reference
    def    datamodels.DatabaseDef
    tables map[string]*Table // by TableDef.Name
}

func (d *Database) Name() string // d.def.Name
func (d *Database) Def() datamodels.DatabaseDef

// Table returns def's table, creating the namespace and the table if
// needed and evolving an existing table to def's columns (today's
// Catalog.Table). Write path. A def that d's DatabaseDef doesn't list
// is an error, so a table can't be created in the wrong database.
func (d *Database) Table(ctx context.Context, def datamodels.TableDef) (*Table, error)

// LoadTable returns def's existing table without creating or evolving
// anything; a missing namespace or table is an error, as is a def that
// d's DatabaseDef doesn't list. Read path (Scan, export, RINEX).
func (d *Database) LoadTable(ctx context.Context, def datamodels.TableDef) (*Table, error)

// identifier is d.name + def.Name; only Database builds Iceberg identifiers.
func (d *Database) identifier(def datamodels.TableDef) table.Identifier
```

```go
// Table: one Iceberg table and the definition it was opened with. Knows
// nothing about the Database or Catalog that holds it.
type Table struct {
    def datamodels.TableDef
    tbl *table.Table // replaced in place by Batch.Commit
}

func (t *Table) Def() datamodels.TableDef
func (t *Table) Iceberg() *table.Table // escape hatch: gnssiceberg readers, tests

// Append: one snapshot that replaces each stream's window (today's
// Catalog.Append, same semantics).
func (t *Table) Append(ctx context.Context, rows arrow.Table, opts ...AppendOption) (int64, error)

// NewBatch starts staged appends on t. No ctx or error: the table already exists.
func (t *Table) NewBatch() *Batch

// Scan: today's Catalog.Scan, on this table.
func (t *Table) Scan(ctx context.Context, sel Selection) (*arrow.Schema, iter.Seq2[arrow.RecordBatch, error], error)
```

```go
// Batch: staged appends to one Table. Semantics unchanged.
type Batch struct {
    t      *Table
    tx     *table.Transaction
    staged int64
    n      int
}

func (b *Batch) Append(ctx, rows, opts...) (int64, error)
func (b *Batch) AppendParts(ctx, parts, opts...) (int64, error)
func (b *Batch) Commit(ctx) (int64, error) // sets b.t.tbl = updated
func (b *Batch) Failed() bool
func (b *Batch) Staged() int
```

```go
// WriteRINEX becomes a function over a gnss_observations Table; it errors
// if obs.Def().Name != datamodels.GNSSObs.Name.
func WriteRINEX(ctx context.Context, w io.Writer, obs *Table, scope datamodels.Scope,
    fileType, logSource string, day time.Time, settings *rinex.Settings) (int, error)
```

Unchanged, package-level helpers: `Config`, `Local`, `Selection` (+`Filter`), `AppendOption`/`WithSource`/`WithSources`/`WithProperties`, `SourceFile`, `HashFile`, `Window`, `ReplaceFilter`, `StreamWindows`, `ErrNoObservations`.

### Why the cache stays correct without back-references

Today `Batch.Commit` writes the committed table back into `Catalog.tables`. Under this design, `Database.tables` holds `*Table` pointers, and `Batch.Commit` replaces `t.tbl` inside that same `Table`. Every holder of the `*Table` (the database's cache and the caller) sees the new snapshot. The `Table` never needs to know who caches it.

Concurrency is unchanged from today: none of these types are safe for concurrent use. Two batches on one `*Table` must not run at once.

## `datamodels` changes

The databases are declared next to the tables:

```go
// DatabaseDef declares one database (Iceberg namespace) and the tables in it.
type DatabaseDef struct {
    Name   string
    Doc    string
    Tables []TableDef
}

var (
    GNSSA_L0 = DatabaseDef{Name: "gnssa_l0", Doc: "...", Tables: []TableDef{AttitudeAHRS, GNSSObs, INSPVA, INSSTDEV, TWTT, ...}}
    GNSSA_L1 = DatabaseDef{Name: "gnssa_l1", Doc: "...", Tables: []TableDef{...}}
    GNSSA_L2 = DatabaseDef{Name: "gnssa_l2", Doc: "...", Tables: []TableDef{...}}
)

// Databases lists every database, in level order.
var Databases = []DatabaseDef{GNSSA_L0, GNSSA_L1, GNSSA_L2}

// DatabaseOf returns the database that lists def.
func DatabaseOf(def TableDef) (DatabaseDef, error)

// Has reports whether db lists def (by name).
func (db DatabaseDef) Has(def TableDef) bool
```

The initial membership is what `Level` gives today (13 tables in `gnssa_l0`, 4 in `gnssa_l1`, 2 in `gnssa_l2`), so nothing moves in Glue.

| Today | After |
|---|---|
| `TableDef.Namespace()` → `gnssa_l{Level}` | removed; `DatabaseOf(def).Name` |
| `TableDef.Identifier()` | removed; `Database.identifier(def)` (unexported) |
| `TableDef.Level` | kept as the data level (`export.go` uses it for Level 1/2 filtering); it no longer names the database |
| `AllTables` | built from `Databases` (each database's tables in order), so a table can't exist outside a database |
| `TableByName("gnssa_l0.inspva")` matches qualified names | unchanged for callers; a qualified name is checked against the `DatabaseDef` that lists the table |
| `TableNames()` returns qualified names | `db.Name + "." + def.Name` over `Databases`; same output as today |

Tests in `datamodels` enforce: every table is in exactly one `DatabaseDef`; database names are unique; table names are unique within a database; and (for now) every table in `GNSSA_L{n}` has `Level == n`.

## Caller migration

| Call site | Today | After |
|---|---|---|
| `sink.Iceberg` | `cat.Append(ctx, def, rows)` | `dbdef, _ := datamodels.DatabaseOf(def)`; `tbl, _ := cat.Database(dbdef).Table(ctx, def)`; `tbl.Append(ctx, rows)` |
| `sink.Iceberg.Catalog()` | returns `*Catalog` | unchanged |
| `sink` file sink (`sink.go:81`) | `def.Namespace()` dir | `DatabaseOf(def).Name` dir; same layout as today |
| `convert.icebergSource` | `cat.Scan(ctx, def, sel)` ×3 | `cat.Database(DatabaseOf(def)).LoadTable(ctx, def)` once per def, then `tbl.Scan(ctx, sel)` |
| `convert` iceberg sink (`iceberg.go:287`) | `s.cat.NewBatch(ctx, def)` | `tbl, _ := cat.Database(DatabaseOf(def)).Table(ctx, def)`; `tbl.NewBatch()` |
| `convert` result names (`files.go`, `result.go`, `convert.go`, `iceberg.go`) | `def.Namespace()+"."+def.Name` | `DatabaseOf(def).Name+"."+def.Name` (a small `datamodels.QualifiedName(def)` helper) |
| `convert.Spec.Tables` | bare or qualified | unchanged |
| `cmd/sfg level0` RINEX | `cat.WriteRINEX(...)` | `tbl, _ := cat.Database(datamodels.GNSSA_L0).LoadTable(ctx, datamodels.GNSSObs)`; `catalog.WriteRINEX(ctx, w, tbl, ...)` |
| tests (`catalog`, `convert`, `cmd/sfg`, `fileparse`) | `cat.Table/Append/NewBatch` | via `cat.Database(datamodels.GNSSA_L0).Table(...)` etc. |

No new CLI flags: the database follows from the table, and the names match the existing Glue namespaces, so existing tables are found as before.

## Rules to hold the layering

1. Only `Database` builds Iceberg identifiers and calls `CreateNamespace`/`CreateTable`/`LoadTable`.
2. `Table` and `Batch` never reference `Database` or `Catalog`.
3. Database names appear only in the `DatabaseDef` declarations. Nothing builds a name from a level or a string literal.
4. Write path uses `Database.Table` (create + evolve); read path uses `Database.LoadTable` (no side effects). Scans and exports must never create tables.
5. No package above `catalog` imports iceberg-go's `catalog` package; `Table.Iceberg()` is the only escape hatch.

## File layout

| File | Contents |
|---|---|
| `catalog.go` | `Config`, `Open`, `Check`, `Warehouse`, `localProps`, `Catalog`, `Database()`, `Databases()`, `Close` |
| `database.go` | `Database`, `Table`, `LoadTable`, `identifier`, `create`, `evolve` |
| `table.go` | `Table`, `Append`, `NewBatch`, `inTableOrder`, `Batch` |
| `export.go` | `Selection`, `Table.Scan` |
| `replace.go`, `rinex.go`, `localio.go` | as today (`WriteRINEX` becomes a function) |

## Resolved

- `convert` reading one level and writing another: no flags. Source and output databases each come from their table's `DatabaseDef`.
- Database name typos: not possible, since names come only from the `DatabaseDef` values. `LoadTable` still wraps Iceberg's "no such namespace"/"no such table" with the database and table name.

## Open questions

- Should a `DatabaseDef` carry Iceberg namespace properties (e.g. description, owner) applied when the namespace is created? Not needed for the move; easy to add later.
