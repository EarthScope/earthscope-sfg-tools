# Go Utilities for EarthScope SFG Tools

This directory builds `sfg`, a single Go binary for converting NovAtel GNSS data to RINEX and Level 1 tables. The Python package bundles it in its wheel.

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
- `convert`: one source to one sink, every part stated with its own flag. So far: `--source iceberg` (any catalog table, selected by scope, file type, log source and time window) or `--source raw` (raw files of the given types through the Level 1 parsers) to `--sink files` (Parquet, CSV or Arrow) or `--sink iceberg` (local SQL or AWS Glue; re-runs replace rather than duplicate). An Iceberg source with an Iceberg sink copies tables between catalogs, e.g. to sync a local warehouse with Glue. `--source-uri` repeats (or takes a shell glob's matches), and each table's snapshots go into one catalog commit unless `--files-per-commit N` caps them. `--result` writes a JSON record of the run; exit codes are 0 ok, 1 fatal, 2 usage, 3 partial. See `plans/sfg-convert-cli.md`.
- `level1 rinex`: one GPS day of one `gnss_observations` stream (scope, file type, log source) from the catalog to RINEX
- `nov2level1`: a NovAtel log to Level 1 Arrow IPC tables (legacy; replaced by `convert --source raw`)
- `rnxqc`: RINEX observation quality check
- `crinex`: Hatanaka (CRINEX) compression and decompression

Run `sfg <command> --help` for each command's flags.

## Python Usage

Building the wheel (`pip install .`, or `pixi run build-go`) compiles the binary and bundles it with the package. The Python wrappers find it with `go_utils.find_binary`:

```python
from earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations import nov0002rnx
from earthscope_sfg_tools.novatel_tools.novatel_level1_operations import nov000_to_level1
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
- **pkg/fileparse/**: raw-file parsers, one per (file type, Level 1 table); `pkg/fileparse/log_sources.md` maps file types to log sources and tables
- **pkg/datamodels/**: Level 1-3 table definitions and row builders
- **pkg/convert/**: `sfg convert`'s sources, sinks, flag checks and result document
- **pkg/catalog/**, **pkg/sink/**: Iceberg catalog access, scans and writers
- **pkg/sfg_utils/**: RINEX conversion helpers and legacy NovAtel parsing
- **Makefile**: Build with platform detection
