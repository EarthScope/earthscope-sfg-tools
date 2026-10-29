# Consolidate Go parsing into one module and remove TileDB

## Context

Go parsing of raw seafloor geodesy files lives in two modules that have drifted apart:

- **`src/go`**: the `sfg` binary built into the Python wheel (`hatch_build.py`) and called from Python through `find_binary` / `GoBinaryRunner`.
  - `pkg/sfg_utils` parses NOV000 (RANGEA, INSPVAA, INSSTDEVA) and NovAtel ASCII and binary, with its own GPSA reassembly.
  - The commands write RINEX, TileDB arrays, and Level 1 Arrow IPC (`nov2level1`).
  - It depends on gnsstools v0.88, and on TileDB-Go, which makes it a cgo build that needs TileDB headers at wheel build time.
- **`dev/level1`**: the Iceberg Level 1 ingest.
  - It has one parser per (file type, table) for NOV000, NOV770, DFOP00, QCPIN, AHRS00, GNSS00, TCVR00, CTD and Sea-Bird.
  - It has the Level 1 table definitions, an Iceberg catalog (SQL or Glue) with replace-on-append, and two drivers (`main.go` and `cmd/sfglevel1`).
  - It depends on gnsstools v0.117.

NOV000 and NovAtel binary parsing therefore exist twice. The `dev/level1` copy has the INS and time statuses, per-antenna log sources, RANGECMP5 and drop counts; the `sfg_utils` copy doesn't. Python can only reach `src/go`, and only `dev/level1` writes Iceberg.

**Decision (2026-10-29): TileDB is being retired in favor of Iceberg tables in SQL (local) and Glue catalogs.** TileDB code is removed, not ported.

## Goal

- **One Go module, `src/go`,** owns raw-file parsing. It ships as one `sfg` binary with no cgo.
- **`fileparse` is the only parser** of each raw file type. The RINEX commands and Python's Level 1 calls both go through it.
- **No TileDB** remains in Go, Python, tests or build configuration.

## Phases

Each phase builds and passes its checks before the next starts.

### 1. Remove TileDB from `src/go`

- **Delete** the `nov0002tile`, `nova2tile`, `novab2tile` and `tdb2rnx` commands, `pkg/sfg_utils/novatel_tdb.go`, and the TileDB helpers in `pkg/sfg_utils/utils.go`. Drop TileDB-Go from `go.mod`.
- **Build without cgo:** remove the TileDB prefix logic from `src/go/Makefile`, and the TileDB header check from `hatch_build.py`. Today `hatch_build.py` silently skips building `sfg` when the headers are missing.
- **Verify:**
  - `CGO_ENABLED=0 go build ./...` and `go test ./...` pass in `src/go`.
  - `sfg --help` lists the remaining commands.
  - A wheel built outside a TileDB environment contains the `sfg` binary.

### 2. Move `dev/level1` into `src/go`

- Move `datamodels`, `fileparse` and `sink` into `src/go`, and move `catalogDef/catalog` up to `catalog`. Delete `catalogDef/table`, which nothing imports.
- Move `fileparse/gpsa` and `fileparse/xsens` under `fileparse/internal/`; only parsers use them.
- Bump `src/go` to gnsstools v0.117 (and geodata v0.117). Update import paths, and delete the `dev/level1` module.
- **Verify:**
  - Every moved test passes in `src/go`.
  - The existing `sfg` commands still pass their tests on the new gnsstools: `pkg/sfg_utils` tests, plus a RINEX conversion of one NOV000 and one NOV770 file compared with the output before the bump.

### 3. One CLI for Level 1

- Add an `sfg level1` command group:
  - **`parse`:** file type, scope and a path in; one Arrow IPC file per table out, plus parse counts as JSON. This is the Python hook from the Level 0 parser PRD.
  - **`ingest`:** the existing driver's behavior, with flags for data directory, warehouse, catalog type (`sql`/`glue`), region, file types and maximum number of files. It keeps the file patterns and the opt-in list for QCPIN and GNSS00 positions.
- Remove `dev/level1/main.go`, `cmd/sfglevel1`, `sfg nov2level1` and `pkg/sfg_utils/level1_arrow.go`.
- **Verify:**
  - `sfg level1 ingest` on NBR1 2025 into a scratch SQL warehouse gives the same per-table row counts as the current driver.
  - `sfg level1 parse` output passes each table's `Validate`.

### 4. One NovAtel parser

- Expose record-level decoding from `fileparse` for the RINEX commands: NovAtel epochs from NOV000, NovAtel ASCII and NOV770 (per antenna). The table functions already build these internally.
- Rewrite `nov0002rnx`, `nova2rnx` and `novb2rnx` on top of it.
- Delete the duplicate parsing in `pkg/sfg_utils/parsing.go`: `ScanFileNOV000`, `ProcessFileNOV000`, `ProcessFileNOVASCII`, `ProcessFileNOVB`, the GPSA reassembly, and `ascii_workarounds.go`, along with their tests.
- **Verify:** RINEX from real NOV000, NovAtel ASCII and NOV770 files (NBR1 2025, SLT1 2026) matches the phase 2 output. Any difference has to be explained, for example epochs that used to be kept but are now dropped for an UNKNOWN time status, and accepted before deleting the old code.

### 5. Python

- Point `novatel_tools/novatel_level1_operations.py` at `sfg level1 parse`, and update `tests/test_novatel_level1.py`.
- **Remove TileDB from Python:**
  - Delete `tiledb_integration/` and `tests/test_tiledb_arrays.py`.
  - Remove the TileDB references in `__init__.py`, `datamodels/observationdata/parsing/ppp.py`, `datamodels/observationdata/garpos/observables.py`, `datamodels/metadata/earthscope/catalogs.py` and `utils/system_utils.py`.
- **Remove TileDB from the build configuration:**
  - In `pyproject.toml`: the `tiledb` extra; the pixi `tiledb` feature and the `tiledb`/`geolab` environments; and the `pandas<3` pin, which exists only for tiledb-py.
  - The TileDB mentions in the README and docs.
- **Verify:** `pixi run test` and `ruff check` pass, and `grep -ri tiledb src tests pyproject.toml hatch_build.py` finds nothing.

## Status

**Phase 1 (2026-10-29): done.** `sfg` builds with `CGO_ENABLED=0` and links only system libraries. A wheel built outside conda includes it. Python's `tiledb_integration` wrappers still call the removed commands until phase 5.

**Phase 2 (2026-10-29): done.**
- **Layout:** `src/go` now holds `datamodels`, `fileparse` (with `gpsa`/`xsens` under `internal/`), `catalog` and `sink`. The two drivers sit in `src/go/devcmd/` until phase 3 replaces them.
- **Dependencies:** gnsstools and geodata are at v0.117. iceberg-go v0.7.0, arrow-go v18.8.0 and sqliteshim v1.3.0 are pinned as `dev/level1` had them, and no dependency is older than it was there. sqliteshim uses pure-Go SQLite without CGO, so the binary stays cgo-free.
- **Tests:** all Go tests pass.
- **RINEX before and after the gnsstools bump:**
  - Epoch counts, pseudoranges, SNR and all other phases are identical. NBR1 2025 NOV000 (3,360 epochs) and NOV770 (48,403 epochs) were compared field by field.
  - The differences all come from gnsstools commit 8585bff6, "novatel: correct LLI semantics across all four range decoders", which was checked against NovAtel App Suite:
    - phase LLI `2` (half-cycle unresolved) is no longer set where parity is known: 32,672 NOV000 fields and 938,867 NOV770 fields;
    - RANGEA gains the binary paths' phase alignment: GLONASS L2P +0.25 cycles (13,440 NOV000 fields);
    - the QZSS L2 code is `2S` instead of the invalid `2M`;
    - the GLONASS `SLOT / FRQ #` header line with unknown (-128) frequency numbers is no longer written.
  - An SLT1 2026 NOV770 (RANGECMP5) file produces no `.rnx` with either binary. That's existing behavior, not part of this change.

**Phase 3 (2026-10-29): done, with one item moved to phase 5.**
- **New commands:** `sfg level1 parse` and `sfg level1 ingest` replace the `devcmd` drivers, which are deleted.
  - `parse` writes `<table>.arrow` per table plus `result.json`, which holds rows per table and per log source, and the failed tables. It exits non-zero only if every table fails, or on bad arguments.
  - `ingest` keeps the old driver's flags, file patterns and opt-in file types (QCPIN and GNSS00), and adds TCVR00. GNSS00's GSV/GSA/VTG tables therefore run only with `--types GNSS00`. That includes its GGA positions, which the deleted `sfglevel1` script left out.
- **Same results:** on NBR1 2025, one file per type, every file and table the old driver wrote gives identical row counts. The old driver stopped at TCVR00, for which it had no file pattern.
- **Moved to phase 5:** removing `sfg nov2level1` and `level1_arrow.go`. `novatel_level1_operations.py` and its tests call `nov2level1`, including its plain NovAtel ASCII (`nova`) mode, which no `fileparse` file type covers. Removing it before Python moves would break Python. So phase 5's Python change now has to come before phase 4, which deletes `ScanFileNOV000` and the other functions `nov2level1` uses.

**Level 1 coverage check (2026-10-29), ahead of phase 4.** Python work is on hold; the goal is that every file type parses to its tables in the catalog.
- **Parse sweep:** `sfg level1 parse` ran on 426 distinct files from 17 campaigns, 2022–2026: every AHRS00, DFOP00, GNSS00, TCVR00 and CTD file, and up to 4 spread-out NOV000, NOV770 and QCPIN files per campaign. None failed. Every empty table matches its raw file:
  - NOV000 and NOV770 files at the start of a session log only `UNKNOWN` time (week 0).
  - Some DFOP00 files have only zero ranges (no reply), interrogations only, or an `ERR3` GNSS block throughout.
  - Some TCVR00 files hold only setting exchanges.
  - 2024 NOV000 files log no INSSTDEVA.
- **Catalog check:** `sfg level1 ingest` ran on all 17 campaigns (2 files per type, all types) into one SQL warehouse. All 13 tables were read back with pyiceberg/pyarrow. Stored rows equal ingested rows, except one identical record that consecutive DFOP00 files share at their boundary, which replace-on-append deduplicates. All times fall between 2021 and 2026.
- **Fixed:**
  - **Week-0 NovAtel logs.** A receiver adjusting its clock logs `COARSEADJUSTING` with week 0. Those INSPVAA/INSSTDEVA rows landed in 1980, so each file's replace window started in 1980 and deleted every earlier file's rows in its stream; NDP1 2023 lost 23,693 rows per table. NOV000 and NOV770 now also skip logs with week 0.
  - **Local partition paths.** iceberg-go's `LocalFS` percent-decodes `file://` paths. `gnss_observations` data, partitioned on `stream_edid`, was therefore written under `…:…` while manifests record `…%3A…`, and pyiceberg could not open it. `catalog` now registers a `file://` IO that uses paths as written, as iceberg-go's S3 IO already does, so Glue was never affected. Local warehouses written before this, such as `Data/ICEBERG_L1`, need re-ingesting.
  - **`make build`** was a no-op once `build/` existed, because the target and directory share a name. It is now `.PHONY`.
- **Not covered:**
  - **Sea-Bird:** no real `.cnv` files exist, so it is tested on synthetic files only.
  - **INOUT0:** no parser.
  - **NOV000 DUALANTENNAHEADINGA:** no table.

## Downstream: `earthscope-sfg-workflows`

The workflows repo uses `tiledb_integration` (array classes such as `TDBKinPositionArray`) in 11 files:
- its SV3, QC and GNSS-refinement pipelines;
- `session.py`, `data_mgmt/utils.py` and `plotting.py`;
- a pipeline integration test.

It pins this package to commit `8e64f62` with the `tiledb` and `iceberg` extras. That pin protects it, so nothing breaks until it upgrades. But it can't move past phase 5 until its pipelines read and write Iceberg instead of TileDB, and that migration belongs to the workflows repo. Phases 1–4 don't change any Python API the workflows repo uses.

## Out of scope

- **RINEX from stored data.** `tdb2rnx` (TileDB → RINEX) goes away with nothing to replace it. RINEX from `gnss_observations` in Iceberg is new work, if needed.
- **Moving generic NovAtel decoding into gnsstools** (INSPVAA/INSSTDEVA, BESTPOS). Worth doing later; Sonardyne, GPSA, SV3 and NMEA stay here.
- **The Level 0 parser PRD's remaining API:** per-table counts in the result, and partial-failure handling. Phase 3's `parse` command reports the counts parsers already return; reworking the API follows the PRD separately. The PRD's log-source selection was dropped on 2026-10-29: parse functions map file type → table and return every log source.

## Open questions

1. **Order of phases 1 and 2.** This plan removes TileDB first, which makes the move smaller and lets the cgo removal be verified on its own. The other order would let the gnsstools bump land first, at the cost of moving code that's about to be deleted.
2. **Other users of the `sfg` TileDB commands** outside the workflows repo, such as notebooks or the geolab images (`sfg-geolab`, `seafloor_geodesy_notebooks`). Check before phase 1.
3. **`es_sfgtools`** (a sibling repo) has its own `tiledb_tools` and NovAtel operations. It's separate from this package and isn't affected, but is it still in use?
