# EarthScope SFG Tools

`earthscope-sfg-tools` provides **parsing and core data primitives** for
Seafloor Geodesy (SFG) processing at EarthScope.

## What's in this package

| Module | Description |
|--------|-------------|
| `novatel_tools` | Parse NovAtel GNSS receiver logs; convert to RINEX |
| `rinex_tools` | RINEX file I/O and quality control |
| `sonardyne_tools` | Parse Sonardyne acoustic instrument data (SV3 format) |
| `datamodels` | Pydantic/Pandera data models and validation for SFG data |
| `seafloor_site_tools` | Sound speed profile utilities |
| `tiledb_integration` | Optional TileDB array schema and storage integration |
| `utils` | Shared helpers including wrappers for Go binaries |

## What this package does NOT own

Orchestration logic (pipeline coordination, catalog management, campaign-level
run management, ingest workflows) lives in `earthscope-sfg-workflows`.

## Quick start

```bash
pixi install
pixi run test
```

See [Installation](installation.md) for full setup instructions and
[Development](development.md) for the contributor workflow.
[Code Documentation](api.md) for X.
