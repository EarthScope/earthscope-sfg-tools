# Go architecture: layers and abstractions

Status: **implemented 2026-11-19.** The section [What changed](#what-changed)
summarises the move from the old layout, and
[Notes and limits](#notes-and-limits) lists two behaviours to know about.

## Intent

The Go code turns raw Sonardyne/NovAtel campaign files into Level 0 tables and
moves tables between storage backends (Iceberg, CSV/Parquet, RINEX). This
document fixes its layers so that:

- **Low levels are strict and uniform.** Every parser, row type and builder
  has the same shape, name pattern and location. Adding a file type or log
  source is filling in a known template.
- **High levels have few, good abstractions.** A command picks one `Reader`
  and one `Writer`; it never knows how a NOV770 record is decoded.
- **Docs are followable.** A reader can start at `sfg convert`, walk down one
  layer at a time, and each layer's doc tells them what it owns and where to go
  next. The package tree *is* the layer diagram.

### Goals

- One direction of dependency, enforced by package imports where natural.
- Every writer takes Arrow — including RINEX, so RINEX can be generated from
  Iceberg as well as from raw files.
- Table definitions are the single source of truth for which log sources feed
  which table; the parser registry and `pkg/README.md`'s diagram derive from
  them.
- Re-ingesting is idempotent, and every Iceberg snapshot records the raw files
  it came from.

### Non-goals

- Transforms (derived products such as PPP antenna positions or GARPOS inputs,
  QC such as `rnxqc`). They may later become a third interface between `Reader`
  and `Writer`; nothing here should prevent that.
- Performance work. A file feeding N tables is read N times (see Parsing).
- CLI and `--result` JSON compatibility. Flags and the result schema may
  change; the one Python caller (`novatel_level0_operations.py`) now runs
  `sfg convert --source raw --sink files`.

### Anti-goals

- A parser that knows about Arrow, Iceberg, scopes or EDIDs.
- A writer with a special input type (e.g. RINEX taking epochs).
- Two abstractions for the same thing (`pkg/sink` and `convert.Sink` were removed for `storage.Writer`).
- Log-source lists kept in more than one place.
- Layer rules that exist only as convention where the compiler could enforce them.

## The pipeline

```
                ┌──────────────────── Reader ─────────────────────┐            ┌──── Writer ────┐
raw file ─Parse─► []Row ─Build─► arrow.Table ──┐                    │            │ Iceberg (local)│
                                               ├─► Batch (many per table) ─────►│ Iceberg (Glue) │
Iceberg / CSV / Parquet ── scan ──────────────┘                    │            │ CSV / Parquet  │
                └─────────────────────────────────────────────────┘            │ RINEX          │
                                                                                └────────────────┘
```

## Layers (bottom to top)

Each layer may import only layers below it.

### 0. Identity — `pkg/edid`

**Owns:** `Scope` (network/station/campaign), `LogSource`, stream and derived
EDID construction.

```go
// A log source is a (file type, name) pair: RANGEA in NOV000 and RANGEA in
// QCPIN are different log sources with different stream EDIDs.
type LogSource struct {
    FileType string // e.g. "TCVR00"
    Name     string // e.g. "RANGE_RESULT"; "RANGECMPB_1" for a second antenna
}

var AllLogSources = LogSource{} // zero value: every source that feeds the table
```

**Imports:** nothing in the module.
**Knows nothing of:** files, Arrow, storage.

### 1. Catalog plumbing — `pkg/catalog`

**Owns:** `Config` (local, Glue, configured; optional `DatabasePrefix` for
development namespaces), connecting, `CatalogDef` / `DatabaseDef` / `TableDef` /
`TableRef` types, low-level scan (`Scan`), append, and replace-rows-in-a-window
(`Replace`, for idempotent writes). The types those need live here too:
`SourceFile` (a raw file's path and SHA-256, made by `HashFile` and recorded in
a snapshot with `WithSources`, or as properties with `SourceProps` and read
back with `SourcesOf`) and `Window` (a time range of a table's time
column). `TableDef` has `Sources []edid.LogSource` — the log sources that feed
the table (empty for product tables).
**Imports:** `edid`, iceberg-go, Arrow.
**Knows nothing of:** SFG's tables (those are `datamodels`), file formats,
RINEX.

### 2. Data models — `pkg/datamodels`

**Owns, per table, side by side:** the log-source values that feed it, the
`TableDef` (schema, checks, partitioning, `Sources`), its `Row` struct, and its
`Build` function. Keeping these together means they cannot drift apart, and a
reader of one table sees everything about it in one place. Also the `SFG`
catalog definition.

```go
var TCVR00RangeResult = edid.LogSource{FileType: "TCVR00", Name: "RANGE_RESULT"}

var TransceiverRanges = catalog.TableDef{
    Name:    "transceiver_ranges",
    Sources: []edid.LogSource{TCVR00RangeResult},
    // schema, checks, partitioning ...
}

type TransceiverRangeRow struct {
    LogSource edid.LogSource // every Row carries its log source
    // ... fields
}

// Build stamps each row's stream_edid from scope + row.LogSource.
func BuildTransceiverRanges(rows []TransceiverRangeRow, scope edid.Scope) (arrow.Table, error)
```

**Imports:** `catalog`, `edid`, Arrow.
**Knows nothing of:** how rows are parsed, where tables are stored.

#### Definitions and references

Everything in a catalog is declared as **package-level vars** in
`datamodels`, in three tiers, using the `catalog` definition types:

```go
// Tables: one var per TableDef (schema, Sources, TimeColumn, ...).
var TWTT   = catalog.TableDef{Name: "twtt", /* ... */}
var INSPVA = catalog.TableDef{Name: "inspva", /* ... */}

// Databases: one var per DatabaseDef, one database per level.
var GNSSA_L0 = catalog.DatabaseDef{Name: "gnssa_l0", Tables: []catalog.TableDef{TWTT, INSPVA, /* ... */}}
var GNSSA_L1 = catalog.DatabaseDef{Name: "gnssa_l1", /* ... */}
var GNSSA_L2 = catalog.DatabaseDef{Name: "gnssa_l2", /* ... */}

// The catalog: holds any number of databases.
var SFG = catalog.CatalogDef{Name: "sfg", Databases: []catalog.DatabaseDef{GNSSA_L0, GNSSA_L1, GNSSA_L2}}

// Refs: one var per (database, table) pair the catalog holds.
var (
    L0TWTT   = GNSSA_L0.Ref(TWTT)
    L0INSPVA = GNSSA_L0.Ref(INSPVA)
    L2Garpos = GNSSA_L2.Ref(Garpos)
    // ...
)
```

A `TableDef` says nothing about where it lives, so the same table can sit in
several databases: `CommunityStandard` is in `gnssa_l0` and `gnssa_l2`. A
`catalog.TableRef{Database, Def}` names one of those placements. It's the only
way layers above `datamodels` refer to a table: through `Selection.Tables`,
`Batch.Ref` and `Writer.Accepts`.

Rules:

1. Code uses the declared ref vars (`datamodels.L0TWTT`), never an inline
   `GNSSA_L0.Ref(TWTT)` or a hand-built `TableRef{}`.
2. A test checks that every declared ref's table is in its database, and that
   every (database, table) in `SFG` has exactly one declared ref.
3. Strings reach a ref only through `SFG.Find("gnssa_l0.twtt")` (CLI flags).
   A bare `twtt` works when the name is unambiguous.
4. Adding a database means declaring a `DatabaseDef` var, appending it to
   `SFG.Databases`, and declaring its refs.

The definitions are plain data, with no I/O. A connection pairs a definition
with a `catalog.Config` (see
[Connecting to a catalog](#connecting-to-a-catalog)). `catalog.Load` or
`catalog.Build` then returns the live tree, which mirrors the definition:
`*Catalog` → `*Database` → `*Table`. Only `storage/iceberg` ever touches that
live tree. Everything else passes refs around.

```
DEFINITIONS (datamodels vars)            LIVE (catalog, inside storage/iceberg)
CatalogDef  SFG                          *Catalog     Load/Build(ctx, cfg, def)
 └ DatabaseDef  GNSSA_L0 …                └ *Database
    └ TableDef  TWTT …                       └ *Table   ← cat.TableOf(ref)
          ▲
     TableRef  L0TWTT = GNSSA_L0.Ref(TWTT)
```

File writers (CSV/Parquet) map a ref to `<dir>/<database>/<table>/…`, so file
output mirrors the catalog's layout.

### 3. Parsing — `pkg/fileparse`

**Owns:** turning **one file into one Row type**. Nothing else.

```go
// One function per (file type, Row type), in the file type's subpackage.
func ParseRanges(path string, src edid.LogSource) ([]datamodels.TransceiverRangeRow, error)
```

Rules:

1. One parse function per (file type, Row type). It returns Go structs from
   `datamodels`, never Arrow.
2. `src` selects one log source, or `edid.AllLogSources` for every log source
   in the file that feeds that Row type. Rows always carry their `LogSource`,
   so a mixed slice can still be stamped correctly by Build.
3. A parse function never sees a scope or EDID; stamping happens in Build.
4. A log source not in the table's `Sources` for this file type is an error
   returned before the file is opened (`Parser.Parse` wraps
   `ErrUnknownLogSource`). Source lists are fixed (including the `_1`
   second-antenna variants), and `Parse` also fails if the rows built carry a
   stream no declared source accounts for.
5. A file feeding several Row types is read once per Row type. Each format's
   low-level decoder (e.g. `readTCVR00File`) is a shared private helper below
   its parse functions.

**Registry.** `fileparse.Parsers` lists one entry per (file type, table):

```go
type Parser struct {
    FileType string
    Ref      catalog.TableRef // the gnssa_l0 table Parse produces
    // unexported: Parse ∘ Build, type-erased
}

// Entry pairs a typed parse function with its Build (generic over the Row type).
func Entry[R any](fileType string, ref catalog.TableRef,
    parse func(path string, src edid.LogSource) ([]R, error),
    build func(rows []R, scope edid.Scope) (arrow.Table, error)) Parser

func (p Parser) Sources() []edid.LogSource // Ref.Def.Sources filtered to FileType
func (p Parser) Parse(path string, src edid.LogSource, scope edid.Scope) (arrow.Table, error)

var ErrUnknownLogSource = errors.New("log source not produced by this parser")
```

`Entry` pairs each typed parse function with its Build, so callers above get a
uniform list (`fileparse.Parsers`, `ForFileType`). `CheckRegistry` checks that
every `(FileType, table)` implied by any table's `Sources` has a registry
entry, and vice versa; `datamodels.CheckDefinitions` checks the definitions
and refs, and `datamodels.SourcesMermaid` draws the diagram in `pkg/README.md`.

**Layout (loose where natural):** `fileparse/` holds the registry and file-type
detection (`Patterns`, `FileTypeOf`); one subpackage per format —
`tcvr00/`, `dfop00/`, `nov770/`, `nov000/`, `nva/`, `qcpin/`, `gnss00/`,
`ahrs00/` — with small formats sharing (`ctd/` holds CTD and SEABIRD) and shared
decoders in their own packages (`novatel/` for the NovAtel ASCII used by NOV000,
NVA and QCPIN; `internal/gpsa`, `internal/sv3` and `internal/rowutil`).

**Imports:** `datamodels`, `edid`; format decoders (gnsstools). Format
subpackages do not import Arrow or `storage`.

### 4. Storage — `pkg/storage`

**Owns:** the two high-level interfaces, the types they exchange, and their
implementations.

```go
type Selection struct {
    Scope      edid.Scope
    Tables     []catalog.TableRef
    LogSources []edid.LogSource // empty = all
    Columns    []string         // empty = all
    Window     *catalog.Window  // optional time window; nil = unbounded
}

// Selection.Filter(def) and Selection.StreamMatcher() turn a Selection into the
// row filter and stream_edid test the Readers apply.

// A Batch is one piece of one table. A Reader yields many per table:
// one per raw file, or one per stream-hour from Iceberg.
type Batch struct {
    Ref    catalog.TableRef
    Table  arrow.Table
    Source *catalog.SourceFile // path + SHA-256; nil when not from a raw file
    Err    error               // set: this piece failed; Table is nil
}

// ErrSkipped is the Err of a Batch for a raw file of no recognised type: it is
// skipped, not a failure.
var ErrSkipped = errors.New("not a recognised raw file")

type Reader interface {
    Tables() []catalog.TableRef // what it can yield
    Read(ctx context.Context, sel Selection, yield func(Batch) error) error
}

type Writer interface {
    Accepts(catalog.TableRef) bool
    Write(ctx context.Context, b Batch) ([]Output, error) // called many times per table
    Close() error
}

type Output struct {
    Ref   catalog.TableRef
    Kind  string // "file" when empty; the Iceberg Writer's is "iceberg"
    Path  string
    Rows, Bytes int64
}
```

A failure in one batch (e.g. one corrupt raw file) is a `Batch` with `Err` set
(or a failed `Write`); it is reported for that batch and does not stop the run.
Calling `yield` hands it the Batch's `Table`, whatever `yield` returns: the
Reader neither uses nor releases it afterwards (`convert.Run` releases it once
the Writer returns).
A Writer reports its outputs either through `Write`'s return value or through
an `Outputs() []Output` method that `convert.Run` calls after `Close`, not both.

| Implementation | Reader | Writer | Package |
|---|---|---|---|
| Raw files (registry: Parse → Build) | ✓ | — | `storage/raw` |
| Iceberg — `iceberg.NewLocal(ctx, dir, def)`, `iceberg.NewGlue(ctx, warehouse, def)` | ✓ | ✓ | `storage/iceberg` |
| CSV / Parquet (the Writer also writes Arrow IPC) | ✓ | ✓ | `storage/files` |
| RINEX (`gnss_observations` only) | — | ✓ | `storage/rinex` |

- **Iceberg Writer:** for each stream in a batch, replaces the rows in the
  batch's time span (idempotent re-ingest) and records `Source` (path,
  SHA-256) in the snapshot as `sfg.source-file` / `sfg.source-sha256`; a batch
  with no `Source` records none. Local and Glue are one implementation with two
  constructors; they differ only in `catalog.Config` (see
  [Connecting to a catalog](#connecting-to-a-catalog)).
- **Time sanity before a replace.** The replaced span is each stream's
  [min, max] time in the batch, so one bad time (a packet with no time read as
  1970) would delete every other file's rows of that stream.
  `catalog.StreamWindows`, which every `Replace` uses, rejects a time before
  1980-01-06T00:00:00Z (GPS time zero) or after now+24h with an error naming
  the stream, column and value. The batch fails (the run is partial or fatal);
  its rows are not dropped silently.
- **Files Writer:** one file per table and output name,
  `<dir>/<database>/<table>/<name>.<ext>`, where the name is the source file's
  name without its extension (`part-00000` when a Batch has no source). Batches
  of one source share a file. Each file is written to a temporary file and
  renamed into place at `Close`, so a failure leaves nothing partial; empty
  batches write nothing. `Write` returns no outputs: the files are reported by
  `Outputs()` after `Close`. The files Reader reads the same layout back.
- **Provenance through files.** `Batch.Source` is always the *raw* file, never
  an intermediate one. A Parquet file stores its batches' `Source` in its
  key-value metadata under `sfg.source-file` / `sfg.source-sha256`
  (`sfg.source-files`, a JSON array, if it held several), and the files Reader
  restores `Batch.Source` from it; a Parquet file without it (written by
  another tool) reads with `Source = nil`. CSV cannot carry metadata, so
  CSV-read batches have `Source = nil`, and an Iceberg sink records no source
  for them.
- **Raw Reader selection.** It honours `Tables`, `LogSources` with a `Name`
  and `Scope` (which stamps the rows); a `Window`, `Columns` or a `LogSource`
  without a `Name` is an error from `Read`.
- **RINEX Writer:** `rinex.NewWriter(dir, settings, moduloMillis)` converts
  Arrow → gnsstools epochs with `rinex.ToEpochs`. A RINEX file holds one
  antenna's stream: a batch of a different stream makes `Write` return an
  error wrapping `rinex.ErrMultipleStreams` (pick one with
  `Selection.LogSources`), the Writer takes no more batches and `Close` writes
  nothing. Header settings, decimation modulo and output directory come from its
  constructor. Holds epochs until `Close`, so de-duplication and cycle-slip
  flagging work across all files, then writes daily files, reported by
  `Outputs()` after `Close` (so `Write` returns none).

**Imports:** `fileparse` (raw only), `datamodels`, `catalog`, `edid`,
gnsstools (rinex only).

#### Connecting to a catalog

An Iceberg connection is two independent things:

- **Where** — a `catalog.Config`: a local warehouse directory, AWS Glue, or a
  catalog from `.iceberg-go.yaml` (REST, etc.).
- **What** — a `catalog.CatalogDef`: the predetermined databases and tables.
  `datamodels.SFG` is all of them; `CatalogDef.Only(refs...)`
  narrows it to a subset, keeping each database's name and doc.

The connection sees only the tables in its definition: a Reader cannot yield
anything else and the Writer's `Accepts` rejects anything else. By default the
constructors **load** (open what the catalog already holds, create nothing);
`iceberg.Create()` **builds** instead (create missing databases and tables,
evolve existing ones to their definitions — never drops data). Readers load;
Writers that may meet a new catalog build.

**Warehouse guard.** With `Create()` and a non-empty `Config.Warehouse`,
`iceberg.New` first loads (changing nothing) the tables the catalog already
holds and requires each table's `Location()` to lie under the warehouse
(trailing slashes ignored; a local warehouse matches its absolute path and its
`file://` form). A table elsewhere is an error naming the table, its location
and the warehouse, suggesting `--database-prefix`, before anything is created
or evolved. It stops e.g. `--warehouse s3://…/sfg-dev` without
`--database-prefix dev_` from evolving and writing the production `gnssa_l0`
tables. Production use, where the warehouse is where the tables live, is
unaffected.

```go
import (
    "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
    "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
    "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/iceberg"
)

// A fixed set of tables: two Level 0 tables in gnssa_l0.
shots := datamodels.SFG.Only(datamodels.L0TWTT, datamodels.L0INSPVA)

// AWS Glue. Credentials come from the AWS default chain (env, profile, SSO,
// instance role); the warehouse is the S3 prefix new tables are placed under.
glue, err := iceberg.NewGlue(ctx, "s3://my-bucket/sfg", shots,
    iceberg.Region("us-east-2"),
    // iceberg.CatalogID("123456789012"), // another account's Glue catalog
)
if err != nil { ... }
defer glue.Close()

// Local warehouse: an SQLite catalog in ./warehouse/catalog.db, data beside
// it. Create() builds the two tables if the warehouse is new.
local, err := iceberg.NewLocal(ctx, "./warehouse", shots, iceberg.Create())
if err != nil { ... }
defer local.Close()

// Copy one station's campaign from Glue to the local warehouse.
res, err := convert.Run(ctx, glue, local, storage.Selection{
    Scope: edid.Scope{Network: "cascadia", Station: "NCC1", Campaign: "2025_A_1126"},
})
```

**Development databases.** `iceberg.DatabasePrefix("dev_")` sets
`catalog.Config.DatabasePrefix`. The code keeps referring to `gnssa_l0`, but
the connection reads and writes the Iceberg namespace `dev_gnssa_l0`. Pair it
with a separate warehouse such as `s3://<bucket>/sfg-dev`. That way, testing
against real Glue never touches the production databases. The prefix is part
of *where* (the `Config`), not *what*: definitions and refs never change. On
the CLI it's `--database-prefix dev_` for `sfg iceberg`, and
`--source-database-prefix` / `--sink-database-prefix` for `sfg convert`.

```go
dev, err := iceberg.NewGlue(ctx, "s3://my-bucket/sfg-dev", datamodels.SFG,
    iceberg.Region("us-east-2"), iceberg.DatabasePrefix("dev_"), iceberg.Create())
```

Both constructors are thin wrappers over the general one, which also covers
REST and other catalogs of `.iceberg-go.yaml`:

```go
iceberg.New(ctx, catalog.ConfiguredConfig("prod-rest"), datamodels.SFG)
// NewLocal(ctx, dir, def, opts...) == New(ctx, catalog.LocalConfig(dir), def, opts...)
// NewGlue(ctx, wh, def, opts...)   == New(ctx, catalog.GlueConfig(wh), def, opts...)  // + Region/CatalogID options
// Options: Create, Region, CatalogID, DatabasePrefix, Properties.
```

On the command line the same pair is `--catalog local|glue|<name>` plus
`--warehouse` (`sfg iceberg`) or `--source-catalog`/`--sink-catalog` plus
`--source-uri`/`--sink-uri` (`sfg convert`), and the tables are chosen with
`--table` (which builds the `Only` subset).

### 5. Runner — `pkg/convert`

**Owns:** connecting one `Reader` to one `Writer`: validating the request up
front (`Writer.Accepts` for every table the Reader will yield; a refusal is a
`*UsageError`), running it (`convert.Run`), collecting per-batch status and
`Output`s into a `Result`, and the exit code. Exit codes (`ExitCode`):
`ExitOK` 0; `ExitFatal` 1 (a fatal error, or nothing was written); `ExitUsage`
2; `ExitPartial` 3 (some batches failed, others were written). A Reader that
yields no batch at all has matched nothing: `Run` records the error
"selection matched no rows" and the run is `ExitFatal`; so is a run whose every
input was skipped ("no recognised input files"). A raw file that parses
to zero rows still yields a batch, so it is a success. A Writer whose `Close`
fails (a "sink" error) is `ExitFatal` when nothing was written, else
`ExitPartial`.

`convert` also holds what turns flags into a conversion: `Spec` (one conversion
as given on the command line), `CheckFlags`, `New` (builds the Reader and Writer
each flag implies, opening an Iceberg one only when run) and `Conversion.Run`,
and `CheckLogSource(ls, refs...)`, which rejects a log source no table of
`refs` (default: every Level 0 table) lists, as a `*UsageError`. They stay here
rather than in `cmd/sfg` because `Conversion` builds Readers and Writers.
**Imports:** `storage` and its subpackages, `fileparse`, `edid`, `catalog`,
`datamodels`.

### 6. CLI — `cmd/sfg`

**Owns:** flag parsing into a `convert.Spec`, the `--result` file and the exit
code. No pipeline logic. `sfg rinex` and `sfg level0` are "raw or Iceberg
Reader → RINEX Writer".

Known exception: `sfg rinex` (and `sfg level0 rinex`) builds its Reader and
Selection in `cmd/sfg` (`rinexRun.reader`) rather than through
`convert.Spec`/`New`, since RINEX is not yet a `convert` sink. Follow-up: a
RINEX sink in `convert`, after which the command is flag parsing only.

## Approaches considered (package layout)

- **A. Package per layer** — *chosen.* The tree is the layer diagram and
  imports enforce the rules; struct, schema and log sources live together in
  `datamodels`. Most churn. Loosened where a split would be artificial (see
  Parsing layout, CSV+Parquet in one package).
- **B. Keep the old packages, reshape inside.** Least churn, but layers are
  only a naming convention; nothing stops parse code importing Arrow.
- **C. One package per format** (parse + rows + build together). Tables fed by
  several formats (`gnss_observations` has four) need their Row type
  elsewhere anyway, which breaks the pattern.

Other decisions made in discussion:

- Parse returns one Row type per call, not a map of every type in the file.
- RINEX takes Arrow like every other writer.
- Reader and Writer are interfaces, with one implementation per backend.
- Local and Glue share one Iceberg implementation.
- `LogSource` is a (file type, name) type, and each value is listed next to the table it feeds.
- Re-ingesting replaces data by time span, and writers receive data as `Batch`es so the source file's provenance travels with it.

## What changed

- Parsing is `file → []Row` in format subpackages; `Build*` and the `Row`
  structs are in `datamodels` next to their `TableDef`. `fileparse` is the
  registry.
- Log sources are `edid.LogSource` values in `datamodels`, listed on
  `TableDef.Sources`; `pkg/README.md`'s diagram is generated
  (`go run ./dev/gen_readme`).
- `pkg/sink`, `convert.Source`/`Sink`/`Rows`/`Group` and `pkg/rinexout` are gone:
  `storage.Reader`/`Writer`/`Batch` and the `storage/rinex` Writer replace them.
- Scan and replace are in `catalog`; `Selection` is in `storage`.
- Every ref is a declared var in `datamodels` (`L0TWTT`, ...).
- `level0_abstractions.md` is replaced by this document.
- Python calls `sfg convert --source raw --sink files --table-format parquet`
  and reads `<out>/gnssa_l0/<table>/*.parquet`.

## Notes and limits

- **CSV is lossy for strings.** In a CSV string column an empty string reads
  back as null, so Parquet is the lossless format.
- **CSV loses provenance.** A CSV cannot carry the raw file's path and SHA-256,
  so batches read from CSV have `Source = nil` and an Iceberg sink records no
  `sfg.source-*` properties for them. Parquet keeps them in its key-value
  metadata.
- **`lock_time_max` is not stored in `gnss_observations`.** `rinex.ToEpochs`
  infers the bound from the log-source type: RANGECMPB (a saturating 21-bit
  counter) and RANGECMP5B (4-bit buckets) are bounded, RANGEA lock times are
  exact, and any other source is left exact with a warning.

## Left to the builder

- The exact names of constants and constructors.
- The generic helper that turns each (Parse, Build) pair into a registry entry.
