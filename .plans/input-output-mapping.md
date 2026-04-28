# Input Output Mapping

## Purpose

This document maps the main input formats that `earthscope-sfg-tools` is expected to ingest to the normalized outputs, dataframe products, community-standard interchange products, and downstream interoperability formats that the package should produce.

The mapping is based on:

- the legacy `earthscope-sfg` package in `es_sfgtools`
- the migration PRD in `.plans/prds/2026-04-27-migrate-parsing-functionality.md`
- the reviewed `gnatss` repository outputs and data contracts

## Design Intent

The package should expose four layers of outputs for most workflows:

1. **Normalized model output** for parser and library users
2. **Validated dataframe output** for analysis workflows
3. **Community-standard interchange output** for seafloor geodesy exchange and archival
4. **Tool-specific compatibility output** for downstream tools such as `gnatss`

In this structure, the seafloor community standard should be treated as the primary generalized export surface. Tool-specific outputs such as GNATSS Level-2 CSV should be modeled as adapters or profiles derived from the normalized and community-standard layers.

## Core Input To Output Mapping

| Input Family | Raw Input Format | Primary Parser Outcome | Validated/Tabular Output | Standardized or Derived Output | Notes |
| --- | --- | --- | --- | --- | --- |
| NovAtel GNSS | `#RANGEA` ASCII log string | `GNSSEpoch` with `Satellite` and `Observation` records | Epoch-to-dict records, optional observation dataframe | GNSS observation extracts for QC or later adapters | Pure parser output; useful for inspection and downstream transformations |
| NovAtel GNSS | QC PIN / QC JSON containing `NOV_RANGE.raw` payloads | List of parsed `GNSSEpoch` records | Flattened epoch/observation dataframe | Candidate input to GNSS-A and community-standard adapters | Should support raw extraction plus parsed output |
| Sonardyne SV3 | DFOP00 event stream file | Parsed interrogation and reply event models | `ShotDataFrame` | Seafloor community-standard acoustic dataframe; GNATSS `gps_solution.csv` after adapter step | High-value migration path for EarthScope workflows |
| Sonardyne SV3 | QC JSON file with interrogation and range blocks | Parsed interrogation and reply event models | `ShotDataFrame` | Seafloor community-standard acoustic dataframe; GNATSS `gps_solution.csv` after adapter step | Equivalent downstream product to DFOP00 path |
| Sonardyne SV3 | Parsed shot-level acoustic observations | Internal acoustic/domain records | `SFGDSTFSeafloorAcousticData` | Seafloor community-standard acoustic export | Good bridge for archival, interchange, and downstream adapters |
| Sound Speed | Seabird file | Parsed sound velocity samples | `SoundVelocityDataFrame` | Solver-ready 2-column sound speed text export | Should preserve interpolation behavior explicitly |
| Sound Speed | CTD two-column CSV/text | Parsed sound velocity samples | `SoundVelocityDataFrame` | Solver-ready 2-column sound speed text export | Support both legacy CTD variants |
| GNSS Position Solutions | PRIDE kinematic output files | Parsed antenna position time series | Position dataframe in J2000 + ECEF + uncertainty form | Converted `GPS_POS_FREED`-style input for posfilter; indirect source for `gps_solution.csv` | Not a direct replacement for `gps_solution.csv`; requires conversion plus fusion with acoustic and attitude data |
| NovAtel Navigation / Attitude | `INSPVA` / `INSSTDEV` style records or equivalent parsed inputs | Time-aligned platform position and attitude models | Position/attitude dataframe with uncertainties | Seafloor community-standard optional fields; GNATSS Level-2 full export fields | Important for rich standardized and GNATSS-compatible exports |
| NovAtel Binary / ASCII Observation Files | `.raw`, `.bin`, ASCII NovAtel files | File conversion wrapper result | Generated file path list | Daily RINEX files via Go wrapper | Optional integration layer, not parser core |
| TileDB-backed GNSS Observations | TileDB arrays | Loaded observation objects or frames | Typed TileDB-backed observation table | RINEX export or analysis-ready datasets | Optional integration surface |

## Recommended Normalized Outputs

These are the outputs that `earthscope-sfg-tools` should treat as first-class internal products.

| Output Type | Purpose | Why It Matters |
| --- | --- | --- |
| `GNSSEpoch`-style model | Canonical parsed representation for NovAtel RANGEA | Preserves detailed measurement structure before flattening |
| Interrogation / reply domain models | Canonical parsed representation for SV3 events | Keeps parsing and merging logic testable |
| `ShotDataFrame` | Operational dataframe for shot-level GNSS-A workflows | Natural bridge between parsing and export |
| `SoundVelocityDataFrame` | Operational dataframe for sound speed workflows | Common input for solver and QC pipelines |
| Community-standard acoustic dataframe | Archival / interchange representation | Aligns with broader seafloor geodesy data exchange |

## Recommended Standardized Outputs

These are the generalized output formats that should be treated as first-class exchange products.

| Output Format | Priority | Produced From | Intended Consumer | Recommendation |
| --- | --- | --- | --- | --- |
| Seafloor community-standard acoustic dataframe | Highest | `ShotDataFrame` plus site metadata and offsets | Exchange, archival, standards-based downstream workflows | Treat this as the primary generalized export target |
| Seafloor community-standard site metadata model | Highest | Survey/site metadata, vessel offsets, array configuration | Standards-based workflow metadata exchange | Export alongside standardized acoustic data where possible |
| GNSS-A community-standard observation table | High | Normalized shot-level transmit/receive geometry and uncertainties | Future interchange layers and tool adapters | Use as the semantic bridge between internal models and tool-specific exports |
| Solver-ready sound speed text file | High | `SoundVelocityDataFrame` | Solver workflows including `gnatss` | Useful lightweight standard-adjacent export |
| RINEX file outputs | Medium | NovAtel wrapper workflows | GNSS post-processing tools | Keep as optional integration |
| TileDB observation arrays | Medium | Parsed GNSS observations | Advanced storage / analytics workflows | Keep optional |

## Recommended Compatibility Outputs

These are the downstream-oriented outputs most worth supporting explicitly.

| Output Format | Priority | Produced From | Intended Consumer | Recommendation |
| --- | --- | --- | --- | --- |
| `gps_solution.csv` | Highest | GNSS-A community-standard observation table or `ShotDataFrame` adapter | `gnatss` solver | Make this the primary tool-specific interoperability export |
| GNATSS Level-2 schema object | Highest | Same data as `gps_solution.csv`, before serialization | Internal validation plus future writers | Implement as an adapter/profile over the generalized observation table |
| `process_dataset.nc` reader support | Medium | External `gnatss` run results | Analysis notebooks, post-processing | Prioritize read support before native write support |
| `residuals.csv` reader support | Medium | External `gnatss` run results | QC and diagnostics | Useful for workflow round-tripping |
| QC PNG plots | Low | Residual or ENU diagnostics | Human review | Prefer reproducible plotting functions over file exports |
| `outliers.csv`, `dist_center.csv`, `deletions.csv` | Low | Solver/QC state | `gnatss` rerun workflows | Support only if needed for tight workflow interoperability |

## Generalized Output Hierarchy

The preferred output hierarchy for `earthscope-sfg-tools` should be:

1. **Normalized internal model**
2. **Validated dataframe representation**
3. **Seafloor community-standard export**
4. **Tool-specific compatibility export**

This hierarchy keeps the standards-facing layer stable even if individual downstream tools change their preferred file formats.

## Proposed End To End Workflow Mapping

### NovAtel RANGEA Path

| Input | Internal Output | Optional Export |
| --- | --- | --- |
| Raw `#RANGEA` string | `GNSSEpoch` | JSON/dict serialization, flattened dataframe |
| QC PIN / QC JSON with `NOV_RANGE.raw` | `list[GNSSEpoch]` | Flattened observation dataframe, future standards adapters |

### SV3 Acoustic Path

| Input | Internal Output | Optional Export |
| --- | --- | --- |
| DFOP00 file | interrogation/reply models -> merged shot records | `ShotDataFrame`, seafloor community-standard acoustic dataframe, GNATSS `gps_solution.csv` |
| QC JSON file | interrogation/reply models -> merged shot records | `ShotDataFrame`, seafloor community-standard acoustic dataframe, GNATSS `gps_solution.csv` |

### PRIDE Kinematic Path

| Input | Internal Output | Optional Export |
| --- | --- | --- |
| PRIDE kinematic position file | antenna position dataframe with times, ECEF coordinates, and uncertainties | converted `GPS_POS_FREED`-style file for posfilter |
| PRIDE kinematic position file + acoustic travel times + attitude/orientation inputs | standardized shot-level observation table | seafloor community-standard acoustic export, GNATSS `gps_solution.csv` |

### Sound Speed Path

| Input | Internal Output | Optional Export |
| --- | --- | --- |
| Seabird file | `SoundVelocityDataFrame` | solver-ready sound speed text |
| CTD file | `SoundVelocityDataFrame` | solver-ready sound speed text |

### GNSS Conversion Path

| Input | Internal Output | Optional Export |
| --- | --- | --- |
| NovAtel ASCII / binary observation files | wrapper result metadata | RINEX files |
| TileDB GNSS array | loaded observation table | RINEX files, analysis datasets |

## Suggested Implementation Order

1. Implement normalized parser outputs for RANGEA, SV3, and sound speed workflows.
2. Implement validated dataframe adapters such as `ShotDataFrame` and `SoundVelocityDataFrame`.
3. Implement seafloor community-standard export models and writers.
4. Implement a GNATSS Level-2 compatibility model and `gps_solution.csv` writer as an adapter over the standardized layer.
5. Add optional RINEX and TileDB integration layers.
6. Add read support for GNATSS solver outputs such as `process_dataset.nc` and `residuals.csv`.

## Recommended Phase 1 Deliverables

For the first implementation phase, the most valuable input-to-output mappings are:

- DFOP00 -> `ShotDataFrame` -> seafloor community-standard acoustic export -> GNATSS `gps_solution.csv`
- QC JSON -> `ShotDataFrame` -> seafloor community-standard acoustic export -> GNATSS `gps_solution.csv`
- PRIDE kinematic positions -> converted GNSS position input -> fused shot-level table -> seafloor community-standard acoustic export -> GNATSS `gps_solution.csv`
- Seabird / CTD -> `SoundVelocityDataFrame` -> solver-ready sound speed text
- RANGEA / QC PIN -> `GNSSEpoch` and flattened observation tables

## Notes

- The seafloor community standard should be treated as the primary generalized exchange layer for acoustic outputs.
- `gps_solution.csv` remains the most important reviewed tool-specific interoperability target from `gnatss`.
- The Level-2 GNATSS schema should be represented internally as an adapter/profile over a generalized validated model/table, not just as a CSV writer.
- PRIDE kinematic files are suitable upstream GNSS position inputs, but they do not by themselves contain the transmit/receive acoustic geometry needed for `gps_solution.csv`.
- Solver result products such as `process_dataset.nc` are useful, but they are secondary to parser and export interoperability for this package.
- Diagnostic files should be treated as optional workflow artifacts rather than primary package outputs.
