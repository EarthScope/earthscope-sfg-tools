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
- `level1 parse`: one raw file to Level 1 Arrow IPC tables plus `result.json` (rows per table and log source)
- `level1 ingest`: a campaign's raw files into the Level 1 Iceberg catalog (local SQL or AWS Glue)
- `level1 rinex`: one GPS day of one `gnss_observations` stream (scope, file type, log source) from the catalog to RINEX
- `nov2level1`: a NovAtel log to Level 1 Arrow IPC tables (legacy; replaced by `level1 parse`)
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

- **cmd/**: CLI entry point and one file per command
- **fileparse/**: raw-file parsers, one per (file type, Level 1 table); `fileparse/log_sources.md` maps file types to log sources and tables
- **datamodels/**: Level 1 table definitions and row builders
- **catalog/**, **sink/**: Iceberg catalog access and writers
- **pkg/sfg_utils/**: RINEX conversion helpers and legacy NovAtel parsing
- **Makefile**: Build with platform detection
