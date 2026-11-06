# Level 2: catalog access API and GNATSS posfilter export

## Context

Level 1 is in Iceberg (`gnssa_l1.*`, written by the Go ingest in `dev/level1`). The next step toward Level 2 is running GNATSS in posfilter mode from that catalog instead of from hand-assembled files. The proposal (`plans/iceberg-gnatss-proposal.md`) gives this job to an access API: it "builds the Level 2 and Level 3 tables from Level 1 and produces the export formats (GNATSS posfilter ascii, …)", and says the posfilter format "is not 1-1 with an ideal table schema — reconciling that gap is a core job of the access API, not the storage layer."

Goal of this phase: a Python API over the catalog that, given a site scope and time window, exports a ready-to-run GNATSS posfilter directory (input files + `config.yaml`). Running GNATSS and storing its output as a Level 2 table are later phases.

## Approach

Python, in the package (`src/earthscope_sfg_tools`), because GNATSS is Python, pyiceberg already reads the Go-written catalog (`dev/query_level1_catalog.ipynb`), and the Level 2/3 schemas live in `datamodels/iceberg/schemas/derived.py`. Go remains the Level 1 ingest layer.

Export GNATSS-native files rather than calling `run_posfilter` in memory: GNATSS's loaders then do its exact-float time joins and positional column handling (`ops/kalman.py` reads `gps_df` by column index), and every export is inspectable and re-runnable.

### 1. Catalog access (`earthscope_sfg_tools/catalog/`)

- `open_catalog(warehouse | config)`: pyiceberg `SqlCatalog("default", uri=sqlite:///<wh>/catalog.db, warehouse=file://<wh>)` for local, Glue for S3 — the same two backends as Go's `catalog.Config`.
- `Level1Reader(catalog)` with `read(table, scope, start, end, columns=None, streams=None) -> pandas.DataFrame`: scope + time-window row filter on the table's time column (`TableDef.scope_filter()` from `schemas/_base.py`), optional `stream_edid` selection (e.g. DFOP00 vs GNSS00 antenna positions). Lifts the `read()` helper out of `dev/query_level1_catalog.ipynb` (incl. its GPST shift for `gnss_observations`).
- Reads go through the current snapshot only (never globbing data folders, which hold superseded files).

### 2. Time conversion (`catalog/timescales.py`)

GNATSS's single time scale is "J2000 seconds" on the GPS calendar: `(UTC + leap(t)) − 2000-01-01T12:00:00`, naive. Level 1 stores UTC (`timestamptz`). One function, UTC → (GPS week, TOW) and UTC → J2000-GPS, with leap seconds from a table (not a hard 18), tested against GNATSS's `gps_ws_time_to_astrotime` and `TimeUnixJ2000` on sample instants.

### 3. Posfilter export (`catalog/posfilter.py`)

`export_posfilter(reader, site_meta, scope, out_dir, *, survey=None, start=None, end=None, positions="DFOP00") -> Path` (window = the survey's, or explicit `start`/`end`) writes:

| File | From | Format (must parse with the GNATSS loader) |
|---|---|---|
| `INSPVAA.dat` | `inspva` | `#INSPVAA,…,FINESTEERING,<week>,<tow>,…;<week>,<tow>,lat,lon,alt,north,east,up,roll,pitch,heading,<status>*<crc>` — `constants.L1_DATA_FORMAT["INSPVAA"]` |
| `INSSTDEVA.dat` | `insstdev` | `#INSSTDEVA` lines per `L1_DATA_FORMAT["INSSTDEVA"]` |
| `GPS_POS_FREED` | `gnss_antenna_positions` (stream chosen by `positions`) | `J2000  GPSPOS  ant_x ant_y ant_z ant_sigx ant_sigy ant_sigz`; rows with null sigmas dropped (counted) |
| `pxp_tt` | `travel_times` | `is_j2k` form: `J2000(T_transmit)  tt_1_µs … tt_n_µs`, one row per ping, columns in `config.transponders` order, 0 = no reply |
| `config.yaml` | `site` + paths | `site_id`, `array_center`, `transponders`, `posfilter.atd_offsets`, `posfilter.input_files`, `output.path`; no `solver` section (GNATSS then also requires the ellipsoid and geoid undulation — solver phase) |
| `sound_speed.txt` (optional) | `sound_speed` | `depth speed`, depth negative-down, for the solver phase |
| `summary.json` | — | rows written/kept/dropped per file, internal delays, warnings |

Travel-time convention — **changed during implementation**. GNATSS places a reply at ping time + travel time, and its own DFOP00 loader keeps the TAT in (range − 0.13 s) and removes it per transponder in the solver via `internal_delay`. Removing the TAT at export would place every reply 0.08–0.64 s early (~0.2–1.3 m of antenna motion). So the export writes `TravelTime − trigger_delay` and sets `internal_delay` = the transponder's TAT, which must be constant over the window.

**The TAT comes from the site metadata, not the logged `tat`.** NBR1's TATs were reprogrammed 2025-09-15 ≈02:30 UTC (5209 200→80 ms, 5210 320→360, 5211 440→640), but the SV3 kept logging its old setting (0.20/0.32/0.44 s). Against the acoustic geometry (straight-ray, harmonic-mean sound speed) of a 6 h window after the change, the metadata TATs leave residuals of 2.9 / 3.1 / 33 ms (MAD ≈ 2–3 ms) versus −117 / +43 / +233 ms with the logged ones. The logged TAT is a fallback when the metadata has none, and a warning when it disagrees. A metadata TAT change inside the window is an error (split the window). Note for the Level 2 GARPOS/Community tables: `travel_times.tat` (Level 1) holds the SV3's setting, not the transponder's TAT, so decision 5's "− tat" must use the metadata too.

### 3a. Site parameters from EarthScope metadata

`PosfilterSite.from_metadata(site: Site, campaign: str, survey: str | None)` builds the export's site parameters from the existing models (`datamodels/metadata/earthscope/`; `Site.from_json`, e.g. `.../<network>/<station>/metadata/site_metadata.json`):

| GNATSS config | EarthScope metadata |
|---|---|
| `site_id` | station (scope) |
| `array_center.lat/lon/alt` | `Site.arrayCenter.latitude/longitude/elevation` |
| `transponders[i].pxp_id` | `Benchmark.name` (e.g. `NBR1-1`) |
| `transponders[i].lat/lon/height` | `Benchmark.aPrioriLocation.latitude/longitude/elevation` |
| `transponders[i].internal_delay` | `Benchmark.transponders[].tat[]` active over the window [ms → s] |
| `transponders[i].sv_mean` | unset (GNATSS computes it from the sound speed profile) |
| `MT_ID` → `pxp_id` | `Benchmark.transponders[].address` (`5209`) ↔ `travel_times.MT_ID` (`IR5209`), active over the window |
| `posfilter.atd_offsets` | `Campaign.vessel.atdOffsets[0]` (`x, y, z` → forward, rightward, downward; see open questions) |
| trigger delay | `Campaign.vessel.acousticTransceivers[].triggerDelay` (0.13 for NBR1 2025, `delayIncludedInTWTT: true`); fall back to `TRIGGER_DELAY_SV3` |
| export window | `Campaign.surveys[]` `start`/`end` and `benchmarkIDs` (one posfilter run per survey) |

Transponders are restricted to the survey's `benchmarkIDs` and to transponders whose `start`/`end` cover the window. Missing pieces (no `arrayCenter`, no ATD offset, a reply from an unknown address) are errors, not defaults.

### 4. Level 1 prerequisite: INS status

GNATSS keeps only INSPVAA lines with `FINESTEERING` and `INS_SOLUTION_GOOD`. Level 1 `inspva` drops both the INS status and the header time status (`dev/level1/fileparse/internal/novatel/ins.go` `ParseINSPVA` parses but discards field 12), so the export could not reproduce that filter and would label alignment-phase rows as good. Add `ins_status` (and `time_status`) to `datamodels.INSPVA` / `INSSTDEV` (Go, `dev/level1/datamodels/level1.go`, `ins.go`) and the Python `level1.py` mirror; re-ingest NOV000. The exporter then emits real statuses and lets GNATSS filter.

Done. Existing tables gain the columns in place: `catalog.Table` evolves a loaded table by adding optional columns the TableDef has and the table lacks (a missing required column is an error). Because an evolved table numbers the new columns after the old ones, `catalog.Append` relabels rows with the table's own schema (order and Iceberg field IDs, by name) before writing — iceberg-go maps written columns by field ID, so def-labelled rows would otherwise land values in the wrong columns.

### 5. CLI

`sfg-catalog export-posfilter --warehouse … --site-metadata site_metadata.json --network … --station … --campaign … [--survey ID | --start/--end] --out DIR` (also `python -m earthscope_sfg_tools.catalog`); without `--survey` or a window it writes one directory per survey of the campaign. A run is then `gnatss run DIR/config.yaml --posfilter --no-run-all`.

## Later phases (not in this plan)

- Run GNATSS from the API and store `gps_solution` (transducer ECEF per shot) as `gnssa_l2.community_standard` (schema in `derived.py`, plus Kalman covariance columns).
- `positions="PPP"` once `gnss_antenna_positions_ppp` (Level 2) exists → Level 3.
- Go-side Level 2 writes would need `catalog.Append` to stop requiring `stream_edid` (`dev/level1/catalogDef/catalog/replace.go`).

## Open questions

1. **ATD axis convention**: `AtdOffset{x, y, z}` has no documented frame. `z` is clearly downward (0.928 m), but whether `x`/`y` are forward/rightward or rightward/forward needs confirming (NBR1 2025: `x 0.0, y 0.00053`, so negligible there, but not in general). Also: several `atdOffsets` entries → pick by transducer `serialNumber`.
2. **Antenna positions for Level 2**: Level 1 SV3 GNSS block (≈1 Hz, single-point, 8–30 m sigmas) per the proposal, or wait for PPP? The export supports both; the default is the SV3 stream.
3. **`time.common` = UTC**: assumed (SV3 NOV_INS times line up with GPS−18 s to ~4 ms). GNATSS's own DFOP loader treats it differently; a regression run against a known-good GNATSS result settles it.
4. **NBR1-3's 33 ms residual** with the metadata TAT (NBR1-1/2: ~3 ms): answered by the solver: it is the a-priori positions. Survey 2 (TATs 200/320/440) and survey 3 (80/360/640) give the same array offset from the metadata positions, (+20.4, −11.4, −12.6) m vs (+20.6, −11.9, −12.2) m, so the TATs are right and `Benchmark.aPrioriLocation` is ~27 m off (no earlier GNATSS solution on disk to confirm).
5. **GNATSS pairs replies by arrival order.** Its solver sorts observations by `T_receive` and pairs each ping's replies positionally with `config.transponders`, ignoring `MT_ID` (`solver/utilities.py` → `ops/data.get_data_inputs`). Where two transponders' travel times overlap (NBR1-2/3 in survey 2: 3.41–3.51 vs 3.41–3.57 s) replies are mislabelled: 26 m rms instead of 2 m. Needs a GNATSS fix (sort each ping by transponder order); until then results are only valid when arrival order is fixed.
6. **DFOP00 antenna sigmas.** The DFOP00 GNSS block positions equal the INSPVA positions exactly, but carry 13–17 m sigmas; GNATSS's Kalman then follows integrated INS velocity and drifts 2–3 m from the input track. With the INSSTDEV sigmas (1–2 m) the residual rms halves (survey 3, 6 h: 5.3 → 2.4 m) and the solution does not move. Done as `position_sigmas="insstdev"` / `--position-sigmas insstdev` (`insstdev_sigmas`: nearest INSSTDEV row within 1 s, ENU sigmas rotated to ECEF); antenna rows in INSSTDEV gaps are dropped (NBR1 survey 2: 343 of 8542, in 508 gaps of 3–16 s). Default stays `logged`.

## Status (2026-10-27)

Implemented: steps 1–5 (`src/earthscope_sfg_tools/catalog/`: `reader.py`, `timescales.py`, `site.py`, `posfilter.py`, `cli.py`; Go Level 1 status columns, table evolution and by-name writes). NBR1 2025_A_1126 (6 h of survey 3) exports in ~6 s and `gnatss` posfilter runs on it unchanged: 4170 transducer positions from 4210 replies, Kalman antenna sigma ~0.6–0.8 m, GNATSS's own J2000 matching ours to 1 µs.

Solver: `export_posfilter(..., solver=SolverOptions())` / `--solver` adds the GNATSS `solver` section (WGS84, geoid undulation 0 as the metadata heights are ellipsoidal, `gps_sigma_limit` 1 m, the exported sound speed profile), so `gnatss run config.yaml` goes from the catalog to an array position. NBR1 survey 2 (center, 2364 epochs): array offset (+20.4 E, −11.4 N, −12.6 U) m, rms 2.1 m. That needs `--position-sigmas insstdev` (open question 6) and GNATSS's reply pairing patched at run time (open question 5); the unpatched run gives 26 m rms. Circle drives (survey 1) are outside GNATSS's 150 m `distance_limit` by design.

## Verification

- Unit: each writer's output parsed back with the GNATSS loaders (`load_novatel`, `load_novatel_std`, `load_gps_positions`, `load_travel_times(is_j2k=True)`) and `load_configuration`, compared to the source frames (`tests/test_catalog_posfilter.py`; runs when `SFG_GNATSS_PYTHON` names a Python with GNATSS installed, e.g. a venv with the local clone `../gnatss` — not a project dependency).
- Site mapping: `PosfilterSite.from_metadata` on NBR1's `site_metadata.json` gives the three benchmarks, the ATD offset, trigger delay and per-window TATs; a window across the 2025-09-15 TAT change is rejected.
- Integration: export NBR1 2025_A_1126 survey `2025_A_1126_NBR1_3` from `Data/ICEBERG_L1`, run `gnatss` posfilter on it, check `gps_solution.csv` row count ≈ replies with matched INS/GPS, and transducer positions within the ATD offset of the antenna.
- Regression: if a previous GNATSS posfilter run exists for a campaign, export the same window and compare `gps_solution` (positions to cm, times exact).
