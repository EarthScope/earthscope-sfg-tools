# Go Utilities for EarthScope SFG Tools

This directory contains high-performance Go utilities for converting NovAtel GNSS data to RINEX and TileDB formats.

## Building

### Prerequisites

- **Go** 1.25.0 or later
- **Conda/Micromamba** with the following environment:
  - TileDB development libraries (libtiledb)
  - C compiler (clang or gcc)

### Quick Start

```bash
cd go/

# Build binaries for your platform
make build

# Bundle with dependent libraries (macOS/Linux)
make bundle
```

The binaries will be available in `go/build/` as:
- `nova2rnx_<os>_<arch>`
- `nova2tile_<os>_<arch>`
- `nov0002rnx_<os>_<arch>`
- `nov0002tile_<os>_<arch>`
- `tdb2rnx_<os>_<arch>`
- `rnxqc_<os>_<arch>`
- (and others)

### Environment Setup

```bash
# Ensure you have a conda environment with TileDB and C tools
conda install tiledb clang patchelf  # patchelf needed on Linux

# Set conda prefix for the build
export CONDA_PREFIX=$CONDA_PREFIX
cd go/
make build
```

### TileDB on macOS

Ensure TileDB is available:

```bash
conda install tiledb
make build
make bundle  # This handles RPATH linking automatically
```

## Python Usage

Once binaries are built and available, use them via Python:

```python
from earthscope_sfg_tools.novatel_tools import go_binaries

# Convert NovAtel ASCII to RINEX
exit_code = go_binaries.nova2rnx(
    input_files=["data.txt"],
    settings_file="settings.json",
    modulo=1000  # 1 Hz decimation
)

# Convert NovAtel ASCII to TileDB
exit_code = go_binaries.nova2tile(
    input_files=["data.txt"],
    tdb_path="s3://bucket/array",
    num_procs=10
)
```

## Troubleshooting

### Binary not found
If you get `BinaryNotFoundError`, ensure:
1. Binaries are built in `go/build/`
2. Binaries are in system `PATH`
3. Binaries match your platform (check with `file` command)

### TileDB linking errors on macOS
The `make bundle` command automatically handles RPATH. If issues persist:
```bash
# Inspect binary RPATH
otool -L go/build/nova2tile_darwin_arm64
```

### CGO errors
Ensure `CONDA_PREFIX` is set and contains TileDB:
```bash
echo $CONDA_PREFIX
ls $CONDA_PREFIX/lib/libtiledb*
```

## Architecture

- **cmd/**: CLI entry points for each tool
- **pkg/sfg_utils/**: Shared parsing and utility functions
- **Makefile**: Build orchestration with platform detection
- **bundle_dylibs.py**: Automatic library bundling for self-contained binaries

## Optional in Python Package

These Go utilities are **optional**. The main Python package works without them.
To use them, either:
1. Build and bundle them yourself (instructions above)
2. Install a pre-built binary distribution if available
3. Install from a package manager once available

The Python wrappers gracefully fall back with clear error messages if binaries are unavailable.
