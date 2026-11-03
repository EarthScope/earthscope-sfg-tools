# Plan: `sfg convert` — explicit source-to-sink CLI

> Source PRD: `.plans/prds/2026-11-02-sfg-convert-cli.md`. Story numbers below refer to its User Stories (1–49).

## Architectural decisions

Durable decisions that apply across all phases:

- **Command surface:** `sfg convert` moves data from one source to one sink. Next to it: `sfg ls`, `sfg crinex` and `sfg qc` (renamed from `rnxqc`). The commands each phase replaces are removed when that phase lands, not aliased.
- **Sources and sinks:**
  - `--source raw|iceberg`, `--sink files|iceberg|rinex`, `--source-uri`, `--sink-uri` and `--scope NET/STA/CAMP` are always required.
  - `--source-type` is required for a raw source.
  - `--source-catalog sql|glue` is required for an Iceberg source, and `--sink-catalog sql|glue` for an Iceberg sink.
  - `--table` is required for an Iceberg source.
  - `--table-format parquet|csv|arrow` is required for a files sink.
- **Nothing is inferred.** Scope never comes from a path, file type never from a file name, and format never from an extension. There's no default catalog and no config file; only the AWS region and credentials come from the environment.
- **Flag applicability:** each source and sink declares the flags it accepts. A flag the chosen pair doesn't accept is a usage error. Invalid pairs fail before any work starts.
- **Module shape:**
  - A source yields batches of (table definition, rows); a sink consumes them and reports what it wrote.
  - The command only validates flags, builds one source and one sink, runs them, and writes the result.
  - Sources and sinks are library code, testable without the binary.
- **Selection:**
  - Selection uses the existing catalog `Selection`: scope, file type, log source, a `[start, end)` window, and columns.
  - For Level 2/3 tables, the file type is the product and the log source is its input.
- **Stream identity:**
  - A stream is identified by `stream_edid` (`network:station:campaign:file_type:log_source`, or `DerivedEDID` for Level 2/3).
  - Iceberg writes use replace-on-append keyed by `stream_edid` and the table's time column.
- **Result document:**
  - Written to `--result PATH|-`. It is versioned (`schema`) and only ever extended.
  - Fields: `command`, `ok`, `source`, `sink`, `scope`, `selection`, `inputs[]`, `streams[]`, `outputs[]` and `errors[]`, as listed in the PRD.
- **Exit codes:** 0 ok, 1 fatal, 2 usage, 3 partial.
- **Logs:** standard error only.
- **RINEX:**
  - One stream per run, written as one file per GPS day under a RINEX v3 long name.
  - `--modulo` is in milliseconds on GPS time; `--rinex-version` defaults to 4.02.
  - Header precedence: station flags > `--station-config` YAML > `--site-log` > `--settings` JSON > defaults.
  - Raw and catalog sources share one epoch grouper and writer.

---

## Phase 1: The frame, and Iceberg → files

**User stories**: 6, 13, 15, 16, 21–26, 39–42, 46, 47

### What to build

The `convert` command and the parts every later phase uses, proven end to end by catalog export:
- the required-flag and inapplicable-flag checks, and rejection of invalid source/sink pairs
- the source/sink interfaces, the result document and the exit codes
- an Iceberg source over SQL and Glue, with `--table` required (bare or namespace-qualified names, a comma-separated list), selection by scope, file type, log source, time window and columns
- a files sink writing one `<table>.<table-format>` file per table in a local directory: Parquet (zstd, Arrow schema stored), CSV (RFC 3339 UTC, empty nulls) or Arrow IPC, with columns in `--columns` order

`export table` is removed.

### Acceptance criteria

- [x] Every required flag, when missing, is a usage error with exit code 2, and so is every inapplicable flag when present (table-driven test).
- [x] An ambiguous table name (`garpos`) without a namespace is a usage error.
- [x] Iceberg → files on a temporary SQL warehouse reads back the right rows in all three formats; the existing export tests pass through the new interface.
- [x] The result document for a successful run and for a usage-error run matches golden JSON.
- [x] Exit codes are checked through the built binary.
- [x] A Glue export of GCC1 `travel_times` (log source DFOP00) returns the same 98,683 rows as `export table` did.
- [x] `export table` is gone, and the README is updated.

Status 2026-11-02: built as `catalog.Catalog.Scan` (read-only: it doesn't create or evolve tables) plus `pkg/convert` (flag checks, `Spec`/`New`/`Run`, Iceberg source, files sink, `Result`, exit codes). `--scope` takes NET, NET/STA or NET/STA/CAMP for an Iceberg source, since reading a whole network is useful; sinks that write rows will need the full scope. Two tables with the same bare name can't share a files sink (their files would collide). Glue check 2026-11-02: 98,683 rows, one stream (`cascadia-gorda:GCC1:2025_A_1126:DFOP00:DFOP00`, 2025-08-21T21:57:19Z to 2025-08-28T02:00:44Z), exit 0.

---

## Phase 2: Raw → files

**User stories**: 1–3, 7–10, 12

### What to build

A raw source:
- `--source-uri` is a file, a glob or a directory.
- With a directory, each `--source-type` uses its filename pattern to find files (QC PIN files under `qc/`). `--max-files` caps how many files of each type are read.
- Every file the run chose is listed in `inputs[]`.
- The type's Level 1 parsers run, and their output is filtered by `--table` and `--log-source`.
- A file that fails to parse is recorded and the run continues.

Paired with the Phase 1 files sink, this replaces `level1 parse`.

### Acceptance criteria

- [x] Raw → Parquet, CSV and Arrow each read back with the parsers' rows (tested with a DFOP00 fixture; the parser packages have no NOV000 file fixture).
- [x] `--table` and `--log-source` limit the output.
- [x] A directory source lists exactly the files chosen; with `--max-files`, each type stops at the cap.
- [x] A corrupt file is reported in `errors[]`, the other files are written, and the exit code is 3.
- [x] `level1 parse` is gone.

Status 2026-11-02: `--source raw` takes `--source-type` (one or more), and optionally `--table`, `--log-source` and `--max-files`; `--start`, `--end`, `--columns` and `--source-catalog` are rejected. It needs the full `--scope`. A file or glob takes exactly one type. Each table is written once with the rows of every file and type that feeds it; a file is parsed once per table. A file that fails is recorded in `inputs[]` (status `failed`) and `errors[]` (stage `parse`) and skipped. No files at all is fatal (exit 1). The filename patterns moved to `convert.Patterns`, shared with `level1 ingest` until Phase 3 removes it.

Real-data check: GCC1 2025_A_1126's 8 DFOP00 files to Parquet in 13 s: `travel_times` 98,683 rows (equal to Glue), `attitude_ahrs` 136,745 and `gnss_antenna_positions` 118,213, one more than Glue in each. Those are the known one-row file-boundary overlaps that replace-on-append collapses on ingest; raw → files keeps every parsed row.

`dev/iceberg_level1.py` still calls `level1 parse`; it's a Python dev harness (Python is on hold), so it was left as is.

---

## Phase 3: Raw → Iceberg

**User stories**: 4, 5, 11, 12, 14

### What to build

An Iceberg sink over SQL and Glue:
- `--sink-uri` is the warehouse.
- Writes use replace-on-append. Missing namespaces and tables are created, and older schemas are evolved.
- The region comes from `--region` or the AWS environment.

This replaces `level1 ingest`, with scope and file types now given explicitly.

### Acceptance criteria

- [x] Raw → SQL Iceberg re-run twice gives the same row counts.
- [x] A bad file gives a partial result with exit code 3; the good files' streams are committed.
- [x] `outputs[]` names each table written, with its row count; `streams[]` has each stream's rows and time span.
- [x] One GCC1 campaign ingested into a local SQL warehouse matches the earlier ingest's per-stream row counts.
- [x] `level1 ingest` is gone.

Status 2026-11-02: sources now hand sinks *groups* (`convert.Group`: an Arrow table plus the file it came from) instead of record batches. A raw source makes one group per parsed file; the Iceberg sink appends each group as one snapshot (`catalog.Append`), so replace-on-append works per file as `level1 ingest` did, and the snapshot summary records the source file and its SHA-256 (`catalog.WithSource`, which ingest never used). The sink creates every table it's handed, even one with no rows. `--sink-catalog sql|glue` is required; `--table-format` is rejected. Iceberg → Iceberg is a usage error until Phase 4. Unlike `level1 ingest`, there's no default set of file types: `--source-type` lists them. `outputs[].rows` counts rows appended, before the one-row file-boundary replacements.

Real-data check: GCC1 2025_A_1126, 146 files of NOV000, NOV770, TCVR00, DFOP00, AHRS00, GNSS00 and CTD, into a fresh local SQL warehouse in 10 min, exit 0, no failed files. All 17 streams in the 13 Level 1 tables match the Glue test catalog row for row (compared through `sfg convert --columns stream_edid,<time column>` on both sides), e.g. NOV770 RANGECMPB 157,811,451 and NOV000 RANGEA 61,772,142. Peak RSS was 35 GB; earlier `level1 ingest` runs in this session peaked at 14–24 GB, so memory under Append is a standing issue for Phase 10's `-j`, not new here.

`pkg/sink` is now used by no command (`sink.Iceberg` was only `level1 ingest`'s); it can be removed separately.

---

## Phase 4: Iceberg → Iceberg copy

**User stories**: 17–20

### What to build

An Iceberg source paired with an Iceberg sink, to sync a local SQL warehouse with Glue in either direction:
- Each named table is scanned with the selection and appended, per stream in time order, to the same table in the sink.
- Rows are copied as they are, file metadata included.
- The sink's snapshot summary records the source catalog, warehouse and selection.
- The same catalog kind with the same warehouse on both sides is a usage error, and so is `--columns`.

### Acceptance criteria

- [x] Copying between two temporary SQL warehouses reads back row-for-row equal; a re-run is idempotent.
- [x] A windowed copy replaces only that window in the sink.
- [x] A sink table with an older schema is evolved.
- [x] Same warehouse on both sides: exit code 2.
- [ ] Manual check: one GCC1 campaign copied from local SQL to the Glue test catalog and back, with per-stream row counts equal at each step. (Glue → local done; local → Glue awaits approval to write to the shared test catalog.)

Status 2026-11-02: when the sink is Iceberg, the Iceberg source groups rows per stream per hour (`[t, t+1h)`, clipped to `--start`/`--end`) in time order rather than per scan batch. The stream_edid's five parts set each group's scope, file type and log source. A first two-column scan finds the streams and their spans. Each group is one snapshot, so replace-on-append deletes only the stream's rows within the group's own span, and a windowed copy leaves the sink's rows outside the copied span alone. Every snapshot's summary carries `sfg.copy-source` (`catalog:uri`) and `sfg.copy-selection` (the selection flags). The same catalog kind and warehouse on both sides is a usage error (a trailing `/` is ignored), and so is `--columns`. `catalog.Scan` loads tables read-only and never creates or evolves them.

Real-data checks, GCC1 2025_A_1126, all compared per stream through `sfg convert --columns stream_edid,<time column>`:
- Local SQL → local SQL, all 13 tables (about 230M rows): 282 s, 10.3 GB peak RSS, exit 0. All 17 streams are equal.
- Glue → local SQL, read-only: the 12 tables other than `gnss_observations` (29.4M rows) took 2 h 27 min, almost all of it waiting on S3. `gnss_observations` for 2025-08-23 (35.9M rows) took 5 min. Both exit 0. The 15 streams of the 12 tables equal the Phase 3 local ingest. For the day of observations, NOV000 RANGEA has 10,141,550 rows and NOV770 RANGECMPB 25,715,418 on both sides.

Added 2026-11-02, batched commits and multiple source URIs:
- An Iceberg sink now commits each table's snapshots together, one catalog commit per table per run, unless `--files-per-commit N` (iceberg sink only; 0 means all) caps it. Each parsed file, or each stream-hour in a copy, is still its own snapshot with its own summary properties.
- `catalog.Batch` stages the overwrites in one transaction, and each staged overwrite sees the ones before it, so the table ends up the same as with per-file commits.
- A failed commit loses only that batch's snapshots. They are reported as "not committed: <files>", and the run continues with a new batch.
- `--source-uri` is repeatable, and arguments after the flags are more source URIs, so `--source-uri raw/*DFOP00.raw` takes every match the shell expands. Paths are deduplicated. An Iceberg source still takes exactly one URI. The result's source endpoint lists `uris` when more than one was given.
- Real data: GCC1's 8 DFOP00 files, given as a shell glob into a fresh SQL warehouse, took 13 s at 311 MB peak RSS, with one commit per table. Stored rows per stream equal the Glue test catalog: 98,683 / 136,744 / 118,212.

---

## Phase 5: Iceberg → RINEX

**User stories**: 27, 30–33, 38

### What to build

A RINEX sink fed by the Iceberg source:
- It takes exactly one stream: a single `--source-type` and `--log-source` from `gnss_observations`.
- Rows are grouped into epochs per GPS day, with `--modulo` applied on GPS time.
- Each day is written under its long name as soon as it closes.
- `--rinex-version` and `--compression none|crx|crx.gz` choose the output form.
- Days the data only partly covers are flagged `partial` in `outputs[]`.
- The epoch grouping moves out of the current catalog RINEX writer into code that Phase 7 will share. Headers use only `--settings` JSON and defaults until Phase 6.

This replaces `level1 rinex`.

### Acceptance criteria

- [ ] More than one stream, or a non-GNSS table, is a usage error.
- [ ] Tests cover multi-day and cross-midnight bucketing, modulo decimation, long names and the partial-day flag.
- [ ] Compressed output decompresses to the uncompressed file.
- [ ] Manual check: three days of GCC1 NOV770 RANGECMP5B from Glue at 1 Hz produce three daily files.
- [ ] `level1 rinex` is gone.

---

## Phase 6: RINEX header resolution

**User stories**: 34–37, 49

### What to build

Full header resolution for the RINEX sink, using gnsstools' `stationmeta.Flags`:
- station flags (`--marker`, `--receiver`, `--antenna-type`), `--station-config` YAML and `--site-log`, with the site-log equipment that was valid at the first epoch
- `--settings` JSON, then defaults

Flag names match `gnss-convert`.

### Acceptance criteria

- [ ] Precedence tests: a flag overrides the YAML, which overrides the site log, which overrides the settings JSON.
- [ ] A site log with an equipment change yields the right equipment on each side of the change.
- [ ] An existing workflows settings JSON produces the same header as before.

---

## Phase 7: Raw NOV000/NOV770 → RINEX

**User stories**: 28, 29

### What to build

The raw source feeding the RINEX sink through the Level 1 parsers. The antenna is chosen by log source (`RANGECMP5B` or `RANGECMP5B_1`). Once parity is shown, `nov0002rnx`, `novb2rnx` and `nov2level1` are removed, along with the `sfg_utils` decoders that only they use.

### Acceptance criteria

- [ ] The same NOV000 fixture converted directly and through a SQL catalog gives byte-identical RINEX.
- [ ] Parity with `nov0002rnx` and `novb2rnx` on fixtures: same epochs and observations, with header differences listed in this plan.
- [ ] Manual check: one GCC1 day from raw files and from Glue compare equal and run through PRIDE.
- [ ] The legacy commands are gone.

---

## Phase 8: `NVA` raw type

**User stories**: 28

### What to build

A raw type for bare NovAtel ASCII logs: RANGEA, plus INSPVAA and INSSTDEVA where present, parsed into the existing Level 1 tables. It works with every sink. Once parity is shown, `nova2rnx` is removed.

### Acceptance criteria

- [ ] `NVA` → files gives the expected rows for each log.
- [ ] Parity with `nova2rnx` on a fixture: same epochs and observations.
- [ ] `nova2rnx` is gone.

---

## Phase 9: `sfg ls`

**User stories**: 48

### What to build

`sfg ls` takes the Iceberg source flags and `--scope`, and lists each stream with its row count and GPS days. For `gnss_observations` the days come from partition metadata, not a scan. With `--result`, the listing is written as JSON.

### Acceptance criteria

- [ ] On a temporary SQL warehouse, the listed streams, counts and days match the data.
- [ ] Days for `gnss_observations` are read without a data scan.
- [ ] The GCC1 Glue listing shows data ending 2025-08-28.

---

## Phase 10: Run controls and `qc`

**User stories**: 43–45

### What to build

Global run controls:
- `--dry-run` lists the files, streams and outputs a run would touch, and writes nothing.
- `-j/--jobs` bounds parallelism.
- `--log-format text|json` sets the log format; logs go to standard error only.

`rnxqc` is renamed to `qc`.

### Acceptance criteria

- [ ] `--dry-run` for every source/sink pair leaves no files or snapshots behind, and its result lists what would be written.
- [ ] `-j 1` and `-j 4` give the same output.
- [ ] `--result -` with `--log-format json` gives parseable JSON on standard output and JSON lines on standard error.
- [ ] `sfg qc` works and `rnxqc` is gone.
