# Plan: TileDB → Apache Iceberg Migration

> Source PRD: `/Users/franklyndunbar/Project/SeaFloorGeodesy/SFGTOOLS/tiledb_to_iceberg_proposal.md`

## Architectural decisions

- **Storage backend**: Apache Iceberg tables, Parquet files on S3 (`us-east-2`)
- **Catalog**: AWS Glue for production; local filesystem catalog for tests (same API, one-line switch)
- **Table namespace**: `sfg` (e.g. `sfg.kin_position`, `sfg.acoustic`)
- **Scope**: `network`, `station`, and `campaign` are first-class columns on every table and part of the partition spec — a single table per data type holds data for all stations, queryable by scope + time range
- **Partitioning**: `(network, station, campaign, date)` on all tables — scope columns use `IdentityTransform`, date uses `DayTransform`
- **Interface contract**: `write_df(df)`, `read_df(start, end, network, station, campaign)`, `get_unique_dates(field, network, station, campaign)` — scope added to read/query calls; `write_df` expects scope columns already present in the DataFrame
- **Scope injection on write**: callers may pass `scope=Scope(network, station, campaign)` to `write_df` to have the columns added automatically if not present in the DataFrame
- **Module layout**: new `iceberg_integration/` module lives alongside `tiledb_integration/` during migration; pipelines switch by changing which class they import
- **Dependency**: `pyiceberg[s3,glue]` added as an optional extra `iceberg` in `pyproject.toml` (mirrors the existing `tiledb` extra)
- **GNSS obs flattening**: the 4-D TileDB array (time, sys, sat, obs) becomes a flat Iceberg table with those same 4 columns plus scope and date partitions — no logic change, just storage shape
- **Timestamp handling**: `pingTime` / `returnTime` float↔datetime conversion stays in the array class, not in the caller

---

## Phase 1: Catalog setup + KinPosition (tracer bullet) ✅

**User stories**: Developer — move away from TileDB; Developer — reference sink tables via S3 URI; Researcher — query data with plain SQL

### What to build

Stand up the `iceberg_integration` module with a catalog factory that returns a Glue catalog (production) or a local filesystem catalog (testing) based on a config flag. Implement `IcebergArray` base class and `IcebergKinPositionArray` as the first concrete table. A developer can write a `KinPositionDataFrame` to a local Iceberg table and read it back with `read_df`. Tests pass without AWS credentials.

### Acceptance criteria

- [x] `iceberg_integration/` module exists with `__init__.py`, `catalog.py`, `schemas.py`, `arrays.py`, `scope.py`
- [x] `pyiceberg[s3,glue]` wired up as `[project.optional-dependencies] iceberg` in `pyproject.toml`
- [x] `get_catalog(mode="local"|"glue")` returns the correct PyIceberg catalog
- [x] `IcebergArray.write_df(df, scope)` appends a validated DataFrame; scope columns injected automatically
- [x] `IcebergArray.read_df(start, end, scope)` returns a DataFrame filtered by time range and scope
- [x] `IcebergArray.get_unique_dates(field, scope)` returns a sorted `np.ndarray` of dates
- [x] `IcebergKinPositionArray` passes a round-trip write→read test using a local catalog
- [x] No TileDB import anywhere in `iceberg_integration/`

---

## Phase 2: Acoustic + IMU tables ✅

**User stories**: Researcher — query acoustic ping data with plain SQL; Researcher — time travel

### What to build

Add `IcebergAcousticArray` and `IcebergIMUPositionArray`. Both use a single time dimension. Verify that the 2-column primary key for `acoustic` (time + transponderID) is enforced correctly via partitioning or a dedup on write. Round-trip tests for both.

### Acceptance criteria

- [x] `IcebergAcousticArray.read_df` slices correctly by time range; pingTime/returnTime round-trip as floats
- [x] `IcebergIMUPositionArray` round-trip test passes, nullable std columns handled
- [x] Acoustic schema uses `pingTime` column name (matches `AcousticDataFrame` pandera schema)
- [x] Scope filter isolates per-station data across both tables

---

## Phase 3: ShotData table ✅

**User stories**: Pipeline developer — `write_df`/`read_df` interface unchanged; Pipeline developer — atomic writes

### What to build

Add `IcebergShotDataArray`. This is the trickiest non-GNSS table: `pingTime` is stored as a float (Unix seconds) in the dataframe schema but must round-trip as a timestamp. Port the float↔`datetime64[ns]` conversion logic from `TDBShotDataArray`. Verify the 2-column key (pingTime + transponderID) deduplication. Verify that a mid-write failure leaves the existing table intact (atomic commit check).

### Acceptance criteria

- [x] `IcebergShotDataArray.write_df` converts float pingTime/returnTime to TimestampTz before writing
- [x] `IcebergShotDataArray.read_df` converts timestamps back to float on return
- [x] Round-trip test with synthetic data preserves pingTime values (allclose rtol=1e-3)
- [x] Empty DataFrame write is a no-op (logged, no Iceberg append called)
- [x] `get_unique_dates` returns correct day count across multi-day writes

---

## Phase 4: GNSS Obs table ✅

**User stories**: User — parse and query by scope/time range; Developer — write Python code referencing sink tables

### What to build

Add `IcebergGNSSObsArray`. Schema aligned with `gnsstools/geodata/gnssiceberg/observations.go` (19 fields, schema version 2) — `constellation`/`signal` as human-readable strings, `epoch` as TimestampTz, rich flag columns. Port `write_epochs()` flattening loop; only the final write call changes from `tiledb.from_pandas` to `table.append`.

### Acceptance criteria

- [x] `write_epochs(epochs)` produces the correct row count (verified: 3 epochs × 3 obs = 9 rows)
- [x] `read_df(start, end, scope)` returns observations with `constellation` and `signal` string columns
- [x] `get_unique_dates` returns correct day count for multi-day writes
- [x] Scope isolation verified: two stations' data does not bleed into each other's reads
- [x] Schema compatible with `gnsstools` gnssiceberg writer (same field names and types)

---

## Phase 5: Historical data migration script ✅

**User stories**: User — access existing data from the new datalake; DevOps — migration is reversible

### What to build

A one-shot CLI script (`migrate_station.py`) that reads each `.tdb` file for a given station using the existing TileDB classes and writes to the corresponding Iceberg tables. Runs array-by-array, logs row counts before and after, and exits non-zero if counts don't match. Original `.tdb` files are never deleted.

### Acceptance criteria

- [x] Script accepts `--tdb-root`, `--network`, `--station`, `--campaign`, `--catalog-mode`, `--warehouse`, `--arrays`, `--dry-run` flags
- [x] Registered as `sfg-migrate-station` CLI entry point in `pyproject.toml`
- [x] Row count before (TileDB) compared to row count after (Iceberg); exits non-zero on mismatch
- [x] `--dry-run` mode prints plan without writing to Iceberg
- [x] `.tdb` files are never deleted or modified
- [ ] Script is idempotent — pending: cross-commit deduplication not yet implemented (known limitation documented in `write_epochs` deduplication test)

---

## Phase 6: Go binary updates

**User stories**: Developer — write Python code referencing sink tables; Developer — move away from TileDB

### What to build

Update `nova2tile`, `novb2tile`, and `nov0002tile` to write Parquet files instead of TileDB arrays. The Python wrappers in `novatel_tools/go_binaries.py` update their output path arguments accordingly. `write_rangea_strings` in `IcebergGNSSObsArray` is updated to ingest the Parquet output.

### Acceptance criteria

- [ ] `nova2tile` writes valid Parquet that PyIceberg can append to the GNSS obs table
- [ ] `novb2tile` and `nov0002tile` produce equivalent row counts to their TileDB counterparts on the same input files
- [ ] `go_binaries.py` wrappers pass tests without a TileDB dependency present
- [ ] `tiledb_integration/` can be removed without breaking any import in `earthscope_sfg_workflows`
