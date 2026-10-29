# Level 1 export coverage, and what TileDB still does

Status as of 2026-10-29. This covers getting data back out of the Iceberg catalog, writing parsed files straight to CSV/Parquet, and what the remaining TileDB integration provides that Iceberg doesn't yet.

## What exists today

### Export from the catalog

| Path | Language | Output |
|---|---|---|
| `Catalog.WriteRINEX` / `sfg level1 rinex` | Go | One GPS day of one `gnss_observations` stream as RINEX 4 |
| `Level1Reader.read(table, scope, start, end, file_type=, log_source=, streams=, where=)` | Python | A pandas DataFrame of any Level 1 table, filtered by scope, UTC window and stream |
| `export_posfilter` / `python -m earthscope_sfg_tools.catalog export-posfilter` | Python | GNATSS posfilter inputs (INSPVAA, INSSTDEVA, GPS_POS_FREED, pxp_tt, sound speed) and config |

### Parse a file without the catalog

| Path | Output |
|---|---|
| `sfg level1 parse --type T --network --station --campaign --out DIR FILE` | `<table>.arrow` (Arrow IPC) per table, plus `result.json` |
| `sink.Parquet` (Go) | Parquet per write: `<dir>/<namespace>/<table>/<table>-<ts>.parquet`, zstd compressed, with the Arrow schema stored. Tested, but **no command uses it**. |
| `sfg nov2level1` (legacy) | NovAtel → Level 1 Arrow IPC; used by Python's `novatel_level1_operations.py` |

There is no CSV writer in Go. In Python, CSV is only available as `Level1Reader.read(...).to_csv()`.

### TileDB integration still in Python (`tiledb_integration/`)

| Array | Contents | Iceberg equivalent |
|---|---|---|
| `TDBGNSSObsArray` | GNSS observations (`write_epochs`, `write_rangea_strings`) | `gnss_observations` |
| `TDBAcousticArray` | Per-ping replies: returnTime, tt, dbv, xc, snr, tat | `travel_times` (DFOP00/QCPIN) and `transceiver_ranges` (TCVR00) |
| `TDBIMUPositionArray` | INS position, velocity, attitude and their sigmas | `inspva` + `insstdev` |
| `TDBKinPositionArray` | **PRIDE kinematic solutions**: lat/lon/height, ENU, number of satellites, pdop, wrms | **none** |
| `TDBShotDataArray` | **Merged shots (Level 2)**: transmit and receive head/pitch/roll and ENU positions per transponder reply | **none** |

Go wrappers in `tiledb_integration` call `sfg` commands that were removed in phase 1 (`nov0002tile`, `nova2tile`, `novb2tile`, `tdb2rnx`), so they fail at runtime. `tile2rinex` and `rinex_qc` are `NotImplementedError` stubs.

How `earthscope-sfg-workflows` uses it, across 9 files:

| Operation | Calls | Where |
|---|---:|---|
| `TDBShotDataArray` | 37 | Written by the SV3 and QC pipelines, rewritten by GNSS refinement, read by `garpos_handler` |
| `TDBKinPositionArray` | 22 | Written from PRIDE `kin_` files in the SV3 and QC pipelines; read by refinement and plotting |
| `tdb2rnx` | 17 | Builds daily RINEX for PRIDE (`time_interval`, `processing_year`, `modulo_millis`) |
| `read_df` / `write_df` | 13 / 8 | Windowed reads and writes |
| `consolidate` | 9 | TileDB fragment maintenance |
| `get_unique_dates` | 4 | Lists the days that have data (shot data, kinematic positions, GARPOS) |
| `write_rangea_strings`, `nov0002tile`, `novatel_770_2tile` | 2 / 3 / 3 | NovAtel → TileDB ingest (QC and SV3 pipelines) |

## Gaps

### Export from the catalog

1. **No Level 2 tables.** Nothing in Iceberg stores PRIDE kinematic positions or merged shot data. These are the two TileDB arrays the workflows use most (59 calls). The GNATSS export reads Level 1 directly, but GARPOS needs shot data with refined positions.
2. **No GARPOS export.** `export_posfilter` covers GNATSS only.
3. **RINEX is one stream and one day per call.** It doesn't cover:
   - **Decimation:** `tdb2rnx --modulo`. The workflows pass `modulo_millis`, and NOV000 is 20 Hz.
   - **Batches:** many days or a whole campaign.
   - **Long-name output:** the RINEX v3 long filename (`sfg_utils.BuildV3ObsFilename`).
   - **Python access:** there is no wrapper.
4. **No "which days have data" query.** This replaces `get_unique_dates`; it could be a scan of `day(time_gpst)` partition values or a `distinct(date(time))`.
5. **No generic table export from the command line.** There is no `sfg`/Python command that writes a table, scope or window to CSV or Parquet. Only Python's `Level1Reader.read` does it, by hand.
6. **Glue was tested on 2026-10-29.** NBR1 2025 (one file per type, all types) was ingested into `s3://test-sfg-api/gnssa-l1-test`, Glue database `gnssa_l1`, `us-east-2`.
   - **Go write:** identical appends to the local SQL catalog.
   - **pyiceberg read:** identical rows on all 21 streams, including `gnss_observations` with `%3A` keys.
   - **RINEX:** `level1 rinex` gave byte-identical output from Glue and SQL (13.5 s against 0.5 s locally).
   - **Fixed:** `catalog.Open` passed `"glue"` as the catalog name instead of `props["type"]`, so Glue had never opened. It also now sets `s3.region`.
   - **Still open (Python, on hold):** the `iceberg` extra installs `pyiceberg[pyiceberg-core]` without `glue`, so `open_catalog(type="glue")` fails with no `boto3`. It needs `pyiceberg[pyiceberg-core,glue]`.
   - **Namespace:** the Glue database name is fixed at `gnssa_l<level>` per account and region. Separate test and production catalogs need different accounts or regions, or a namespace option on `catalog.Config`.

### Parse straight to CSV/Parquet

7. **`level1 parse` writes only Arrow IPC.** The tested `sink.Parquet` has no CLI flag. A `--format arrow|parquet|csv` flag on `parse`, and a `--sink parquet` option on `ingest`, would cover this without a catalog.
8. **There's no CSV writer.** `arrow/csv` in arrow-go would add one in a few lines. `gnss_observations` is far too large to be practical as CSV (about 40M rows per 20 Hz NOV000 day).
9. **No plain NovAtel ASCII file type** (`nova`) in `fileparse`. Only the legacy `nov2level1` reads it.

### File types and log sources not parsed

10. **INOUT0** (SV3 command console) has no parser.
11. **NOV000 DUALANTENNAHEADINGA** (dual-antenna heading) has no table.
12. **Sea-Bird `.cnv`** is tested only on made-up files; no real ones exist.

## TileDB removal blockers (phase 5)

TileDB can go when the workflows have:
- Iceberg tables and writers for **kinematic positions** and **shot data** (gaps 1–2)
- a RINEX path that covers `tdb2rnx`'s uses: catalog RINEX with decimation and batch days, plus a Python wrapper (gap 3)
- a replacement for `get_unique_dates` (gap 4)

`consolidate` has no direct equivalent. Iceberg has snapshot expiry and compaction instead, and they need a maintenance plan. The rest of TileDB's reads and writes already have Level 1 equivalents.
