# Level 2/3 tables and catalog export

Status as of 2026-10-30. This plan defines the derived (Level 2 and 3) tables, how each one is built, and how data leaves the catalog. It replaces the TileDB `kin_position` and `shotdata` arrays and the export gaps in `plans/level1-export-coverage.md`.

Decisions (2026-10-30):
- **Scope of this repo:** table definitions, raw and product file parsing, and export. Processing — running PRIDE, the GNATSS Kalman smoother, building GNSS-A shots — happens downstream (`earthscope-sfg-workflows`), which writes its results to these tables through `catalog.Append`.
- **Language:** Go holds the table definitions, parsers and exporters. Python stays on hold.
- **Level 3 positions:** the smoother's antenna track (PPP positions fused with INSPVA velocities) is stored as its own Level 2 table, and Level 3 GNSS-A is built from it.
- **Export:** a generic table export, a "days with data" query, and named exporters for the formats a table can't simply be dumped to.

## Tables (`src/go/datamodels/level2.go`, done)

Derived tables carry `Network`, `Station`, `Campaign` and a required `stream_edid`, with no `Log_Source` (proposal decision 3). The stream EDID is `network:station:campaign:<product>:<input>`, built by `DerivedEDID`. The input is the stream the product was built from, minus its scope, with `:` turned into `.`. `Catalog.Append` therefore replaces a rebuilt product the same way it replaces a re-ingested file, with no catalog changes.

| Namespace | Table | Time column | Product, with an example input | Replaces |
|---|---|---|---|---|
| `gnssa_l2` | `gnss_antenna_positions_ppp` | `time` | `PRIDE`, `NOV000.RANGEA` | TileDB `kin_position` |
| `gnssa_l2` | `gnss_antenna_positions_smoothed` | `time` | `SMOOTHED`, `PRIDE.NOV000.RANGEA` | (the workflows keep it in memory) |
| `gnssa_l2` | `garpos` | `ST` | `GNSSA`, `DFOP00.DFOP00` (the Level 1 SV3 positions stream) | TileDB `shotdata`, before refinement |
| `gnssa_l2` | `community_standard` | `T_transmit` | `GNSSA`, as above | — |
| `gnssa_l3` | `garpos` | `ST` | `GNSSA`, `SMOOTHED.PRIDE.NOV000.RANGEA` | TileDB `shotdata` with `isUpdated` |
| `gnssa_l3` | `community_standard` | `T_transmit` | `GNSSA`, as above | — |

The GARPOS and community standard columns are the same as Python's `derived.py`. Three changes go beyond it; `derived.py` should pick them up when Python work resumes:
- `stream_edid` is added.
- `gnss_antenna_positions_smoothed` is new: antenna position columns plus `rho_xy/xz/yz`.
- Level 3 takes its positions from the smoothed track, not straight from PPP.

`TestDerivedTables` (in both `datamodels` and `catalog`) checks the shape of every table, creates them all, and checks that rebuilding a Level 2 GARPOS stream doesn't touch Level 3.

## Who writes each table

| Table | Written by | In this repo |
|---|---|---|
| `gnss_antenna_positions_ppp` | Downstream runs PRIDE (on RINEX from E3); this repo parses PRIDE's output | **P1:** a parser for PRIDE's `kin_` file (X/Y/Z, lat/lon/height, Nsat, PDOP), joined by epoch with its `res_` file for `wrms` (per-epoch weighted RMS of residuals, as GNSSomellier's `pride-ppp` reads it; the workflows' `max_wrms` prefilter uses it). Sigmas are null: PRIDE reports none, and the workflows' 0.1 m is the smoother's assumption. |
| `gnss_antenna_positions_smoothed` | Downstream (GNATSS smoother) | Table definition only. Filter settings (`start_dt`, `gnss_pos_psd`, `vel_psd`, `cov_err`) belong in the snapshot summary. |
| `garpos`, `community_standard` (Levels 2 and 3) | Downstream shot building | Table definitions only. Travel-time correction, TAT and trigger delay from site metadata, attitude source, ATD offsets and the ENU frame are the builder's decisions; `plans/level2-posfilter-export.md` records what is known about them. |

P1 is a product parser, not a raw-file one: its rows have no `Log_Source`, and its stream EDID names the observation stream PRIDE was run on (`DerivedEDID(scope, ProductPRIDE, "NOV000:RANGEA")`), which the caller supplies. `fileparse.Parser` stamps `Log_Source`, so P1 needs a small variant of it for derived tables.

## Export

All export is a library function on `*catalog.Catalog` plus a `sfg export` subcommand. `sfg level1 rinex` moves to `sfg export rinex`.

| # | Command | Output | Replaces |
|---|---|---|---|
| E1 | `sfg export table --table NAME\|NS.NAME --network [--station [--campaign]] [--type FT [--log-source LS]] [--log-source LS] [--start --end] [--columns …] [--format parquet\|csv\|arrow] --out PATH\|-` — **done** (`Catalog.Export`, `catalog.Selection`) | One file of the selected rows, streamed batch by batch. `--type`/`--log-source` pick a file type or one stream (`stream_edid`), and need the full scope; `--log-source` alone matches `Log_Source`. In Level 2/3 tables they are the product and its input. The window is `[start, end)` on the table's time column (GPS time for `gnss_observations`). Parquet uses `sink.Parquet`'s settings; CSV has RFC 3339 UTC times and empty nulls; columns come out in `--columns` order. | `TDB*.read_df`, Python `Level1Reader.read` |
| E2 | `sfg export days --table … --network --station --campaign [--stream …]` | A list of GPS (or UTC) days with rows, per stream. For `gnss_observations` this reads partition metadata (`time_gpst_day`) without scanning; other tables scan the time column. | `get_unique_dates` |
| E3 | `sfg export rinex … --start DAY [--end DAY] [--interval SEC] --out DIR` | One file per GPS day, with long RINEX v3/4 names (`BuildV3ObsFilename`). `--interval` decimates by epoch time, like `tdb2rnx --modulo`. | `tdb2rnx` |
| E4 | `sfg export garpos --level 2\|3 … --out FILE` | The GARPOS observation CSV of a `garpos` table (times as seconds of day from a reference date, as GARPOS expects). GARPOS's settings and site files come from metadata downstream. | `garpos_handler`'s TileDB reads |
| E5 | `sfg level1 parse --format arrow\|parquet\|csv` | Parse straight to files without a catalog. | gaps 7–8 in the coverage doc |

The GNATSS posfilter export stays in Python (`export_posfilter`) for now. It reads Level 1 tables that E1 can also produce.

## Order

1. Tables. **Done.**
2. E1 **(done)** and E2, the generic export and the days query: every downstream reader needs them.
3. E3, catalog RINEX with day ranges, decimation and long names: the input to PRIDE.
4. P1, the PRIDE `kin_`/`res_` parser.
5. E4 (GARPOS) and E5.

With these, downstream can replace every TileDB call in the coverage doc: read Level 1 (E1–E3), write PPP (P1) and its own products (`catalog.Append`), and export GARPOS (E4). Switching the workflows over is phase 5.

## Notes for downstream

- **TileDB naming:** in `kin_position` and the workflows' `prepare_kinematic_data`, the `east/north/up` columns hold ECEF X/Y/Z. Any one-time migration of TileDB data must map them to `ant_x/y/z`.
- **Shot attitude today** comes from the NOV_INS block in each DFOP00 event (`sv3_operations.py`), which Level 1 doesn't store (proposal decision 7). A shot builder reading `inspva` or `attitude_ahrs` will differ slightly from current shot data.
- **ATD axes:** GARPOS prep maps `atdOffsets[0]` x → forward, y → rightward, z → downward.
- **PRIDE rate:** the RINEX config's `modulo_millis` defaults to 0, so production runs PRIDE at full rate.
