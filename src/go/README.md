# Go Utilities for EarthScope SFG Tools

This directory builds `sfg`, a single Go binary for converting NovAtel GNSS data to RINEX and Level 0 tables. The Python package bundles it in its wheel.

## Building

### Prerequisites

- **Go** (version in `go.mod`)

The binary is pure Go (no CGO), so it needs no C compiler or shared libraries at build or run time.

### Quick Start

```bash
cd src/go/
make build
```

The binary is written to `src/go/build/sfg_<os>_<arch>`. Its commands:

- `nov0002rnx`, `nova2rnx`, `novb2rnx`: NovAtel NOV000, ASCII and binary logs to RINEX
- `convert`: one source to one sink, every part stated with its own flag. So far: `--source iceberg` (any catalog table, selected by scope, file type, log source and time window) or `--source raw` (raw files of the given types through the Level 0 parsers) to `--sink files` (Parquet, CSV or Arrow) or `--sink iceberg` (re-runs replace rather than duplicate). An Iceberg source with an Iceberg sink copies tables between catalogs, e.g. to sync a local warehouse with a cloud one. `--source-uri` repeats (or takes a shell glob's matches), and each table's snapshots go into one catalog commit unless `--files-per-commit N` caps them. `--result` writes a JSON record of the run; exit codes are 0 ok, 1 fatal, 2 usage, 3 partial. See `plans/sfg-convert-cli.md`.
- `iceberg ingest`, `iceberg export`: the everyday catalog paths, with the catalog's configuration filled in. `--catalog` defaults to the iceberg-go `default-catalog` and `--warehouse` to its configured warehouse. `ingest` takes raw files, globs or campaign directories as arguments and tells each file's type by its name (files of no type are listed as skipped); `export` writes tables to Parquet, CSV or Arrow files in `--out`. `ingest` parses `--workers` files at once (default: one per CPU) and writes up to `--files-per-snapshot` of them (default 32) as one snapshot, each file still replacing only its own streams' time span; `convert` takes the same two flags, defaulting to one snapshot per file. For example:

  ```bash
  sfg iceberg ingest --scope cascadia-gorda/GCC1/2022_A_1065 raw/*
  sfg iceberg export --scope cascadia-gorda/GCC1/2022_A_1065 --table inspva --out out/
  ```
- `level0 rinex`: one GPS day of one `gnss_observations` stream (scope, file type, log source) from the catalog to RINEX
- `nov2level0`: a NovAtel log to Level 0 Arrow IPC tables (legacy; replaced by `convert --source raw`)
- `rnxqc`: RINEX observation quality check
- `crinex`: Hatanaka (CRINEX) compression and decompression

Run `sfg <command> --help` for each command's flags.

Iceberg catalogs are configured the iceberg-go way ([configuration](https://go.iceberg.apache.org/configuration.html)). `--catalog`, `--source-catalog` and `--sink-catalog` take `local`, a warehouse directory with an SQLite `catalog.db`, or the name of a catalog in `.iceberg-go.yaml` (in `$GOICEBERG_HOME`, else the home directory): a `rest`, `glue`, `sql` or `hadoop` catalog over `s3://`, `gs://` or `abfs[s]://` storage. Credentials come from each cloud's default chain (`AWS_REGION`/profile, GCP application default credentials, Azure default credentials).

```yaml
default-catalog: sfg
catalog:
  sfg:                      # AWS Glue over S3
    type: glue
    warehouse: s3://bucket/prefix
  sfg-rest:                 # any Iceberg REST catalog (Polaris, Unity, Lakekeeper, BigLake, ...)
    type: rest
    uri: https://catalog.example.org/api/catalog
    credential: client-id:client-secret
    warehouse: sfg
```

## Python Usage

Building the wheel (`pip install .`, or `pixi run build-go`) compiles the binary and bundles it with the package. The Python wrappers find it with `go_utils.find_binary`:

```python
from earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations import nov0002rnx
from earthscope_sfg_tools.novatel_tools.novatel_level0_operations import nov000_to_level0
from earthscope_sfg_tools.rinex_tools.crinex import crinex_compress
from earthscope_sfg_tools.rinex_tools.quality_control import rnxqc
```

## Troubleshooting

### Binary not found
If you get `BinaryNotFoundError`, ensure:
1. The binary is built in `src/go/build/`, or the package was installed from a wheel built with Go on the `PATH`
2. The binary matches your platform (check with the `file` command)

## Architecture

- **cmd/sfg/**: the `sfg` binary: CLI entry point and one file per command
- **pkg/fileparse/**: raw-file parsers, one per (file type, Level 0 table); `pkg/fileparse/log_sources.md` maps file types to log sources and tables
- **pkg/datamodels/**: Level 0-2 table definitions and row builders
- **pkg/convert/**: `sfg convert`'s sources, sinks, flag checks and result document
- **pkg/catalog/**, **pkg/sink/**: Iceberg catalog access, scans and writers
- **pkg/sfg_utils/**: RINEX conversion helpers and legacy NovAtel parsing
- **Makefile**: Build with platform detection
