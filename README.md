# EarthScope Seafloor Geodesy Tools

`earthscope-sfg-tools` contains **parsing and core data primitives** for
Seafloor Geodesy processing.

## Scope of this repository

This repo is the home for:

- NovAtel parsing/conversion (`novatel_tools`, `rinex_tools`)
- Sonardyne parsing (`sonardyne_tools`)
- Shared data models (`datamodels`)
- TileDB schema/array integration (`tiledb_integration`)
- Shared utility helpers used by downstream workflows

This repo intentionally does **not** own orchestration logic (pipeline
coordination, catalog orchestration, ingest workflows, campaign-level run
management). That functionality lives in `earthscope-sfg-workflows`.

## Installation

### Prerequisites

- [pixi](https://pixi.sh) (recommended)

### Quick Start

```bash
pixi install
pixi run test
```

### Development

```bash
pixi run lint
pixi run format-check
pixi run test
```

See `PIXI.md` for environment and task details.