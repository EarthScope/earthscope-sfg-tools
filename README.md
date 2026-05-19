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

## Documentation: Generating and Viewing

### Prerequisites

- Activate the Pixi environment:
  ```sh
  pixi shell
  ```

### Generate API Documentation

After making changes to the code, run the API doc generator script to produce up-to-date markdown files and an updated API table of contents:

```sh
python scripts/generate_api_md.py
```

- This will generate markdown files for your API and print out (or write) an `api_toc.yml` file.
- If you have added or removed modules/scripts, **copy the new `api_toc.yml`** to the appropriate location (e.g., update your main `myst.yml` or include it as needed).

### Build and Preview the Documentation Site

To build the docs:

```sh
myst build
```

To serve the docs locally with live reload:

```sh
myst start
```

Then open the provided local URL in your browser to view the documentation.

### Notes

- Commit all hand-written and generated markdown files in `docs/` (except for `_build/`).
- Do **not** commit the `_build/` directory; it contains only build artifacts.
- Remember to update your `myst.yml` with the latest `api_toc.yml` section if your API changes.