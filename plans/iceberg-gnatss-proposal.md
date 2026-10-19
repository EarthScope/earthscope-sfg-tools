Working Group: @Franklyn Dunbar @Mike Gottlieb @Rachel Akie

# GNSS-A Data Lake & Access API — Proposal

This document is a proposal for extending current utilities to support GNSS-A data processing with GNATSS, and to move away from TileDB-oriented data lakes in favor of Apache Iceberg.

**TLDR:** We want to create Iceberg tables for the assorted log types that are relevant to GNSS-A. Rows will contain the data, source file type, and scope (network, station, campaign). We will partition the warehouse by network, station, campaign. We can perform downstream pre-processing from these Level 1 data (i.e. PPP from the GNSS logs, GNSS-A in GARPOS/GNATSS format).

It was noted in the last meeting that we are going to prefer INS\* logs from the AHRS NovAtel files for waveglider attitude, which makes 'Posfilter mode' the most straightforward approach for using GNATSS.

These tables are the raw parsed outputs from our full rate files. We will need to put an API in front of some of them. The format for the ascii files that GNATSS expects to run in 'Posfilter mode' is not 1-1 with an ideal table schema — reconciling that gap is a core job of the access API, not the storage layer.

## Standardization decisions

Before finalizing the schemas (see Schemas at the end of this doc), several cross-cutting questions needed answers so every table (and the API in front of them) agrees on the same conventions. Resolved:

1. **Standardized time column.** Every table carries a `time` column (or, where a row spans two distinct instants — travel times, GARPOS — the existing named columns, e.g. `T_transmit`/`T_receive`) as `timestamptz`, UTC, microsecond precision. No separate stored "scientific timescale" column — GNATSS's TT/J2000 scale, GPS time, GPS-week+TOW, and GARPOS's seconds-of-day are all derived by the API at read time. They're annoying. **Exception:** `gnss_observations.epoch` holds GPS time (no leap-second shift), because the table follows the gnsstools `observations` layout and gnsstools readers/RINEX writers assume GPST there. NovAtel INS logs (INSPVA, INSSTDEV) are converted from GPS time to UTC at ingest, and logs with header time status `UNKNOWN` (no time fix yet) are dropped.
2. **Scope fields.** Every table carries exactly `Network`, `Station`, `Campaign` — matching the warehouse partitioning scheme.
3. **Log\_Source.** Present on every Level 1 (raw, single-source) table; intentionally absent from the Level 2/3 derived tables (GARPOS format, Community standard format), since a merged/derived row can combine more than one source log.
4. **Sampling rate.** Level 1 tables stay at their native sampling rate (true to "Level 1 = raw parsed output") — no resampling onto a shared grid at ingest.
5. **Raw vs. corrected travel time.** Level 1 stores the raw two-way range and `tat` as logged. The `− tat − trigger_delay` correction is applied when building Level 2 (GARPOS `TT`, Community `TravelTime`). The trigger delay stays hardcoded in the processing logic; it is not stored in the catalog.
6. **Coordinates.** Level 1 antenna positions and the Community standard tables are ECEF. The GARPOS tables store GARPOS's native local ENU (`ant_e/n/u`, relative to the site array center), georectified by the API at build time from the site metadata. Position uncertainties are stored as ECEF std devs in metres, rotated from the logged ENU std devs at ingest; `rho_xy/xz/yz` correlations are not stored for now (GNATSS defaults them to 0).
7. **SV3 observation blocks.** The `GNSS` fix embedded in SV3 events is the source for the Level 1 antenna-position table. `NOV_INS` and `NOV_HEADING` are not stored for now — attitude comes from INSPVA.

### Type conventions

Types in the schemas are Iceberg primitive types: `string`, `int` (32-bit), `long` (64-bit), `float` (32-bit), `double` (64-bit), `boolean`, `timestamptz` (µs, UTC). Fields parsed as Python `Decimal` today (SV3/Sonardyne models) are stored as `double` — Iceberg `decimal(p,s)` would need a fixed precision/scale per column and buys nothing at sensor precision. Constraints are carried over from the existing pandera/pydantic models and should be enforced at ingest.

Every table in Schemas also carries the three scope columns and (Level 1 only) `Log_Source`; they are listed in full in the first schema and abbreviated as *scope* afterwards:

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| `Network` | `string` | required | Network identifier |
| `Station` | `string` | required | Station identifier |
| `Campaign` | `string` | required | Campaign identifier |
| `Log_Source` | `string` | required (Level 1 only) | Source log file or log type |

## Level 1 Tables

- Two way travel times (per transponder)
- GNSS pseudo range antenna positions
- GNSS observation logs
- NovAtel INSPVA logs
- NovAtel INSSTDEV logs
- Sonardyne Roll/Pitch/Heading (AHRS) logs
- Sound speed — we can add scope to the rows, the partitioning should keep things separate as well.

## Level 2 Tables

- GNSS antenna positions (PPP kinematic solution)
- GNSS-A Community standard format data (pseudo range antenna positions)
- GNSS-A GARPOS format data (pseudo range antenna positions)

For now we should just use separate tables for GNSS-A data that has been 'updated' with PPP kinematic solutions.

## Level 3 Tables

- GNSS-A Community standard format data (with antenna positions sourced from Level 2 GNSS antenna positions)
- GNSS-A GARPOS format data (with antenna positions sourced from Level 2 GNSS antenna positions)

## Access API

The API sits between the Iceberg tables and the processing codes (GNATSS, GARPOS). It builds the Level 2 and Level 3 tables from Level 1 and produces the export formats (GNATSS posfilter ascii, GARPOS csv, time-scale conversions). How it does this will be specified separately; this document covers the catalog only.

## Schemas

### Two way travel times

Source files: Sonardyne DFOP00 JSONL (`range` and `interrogation` events); Sonardyne QC PIN JSON (`QCPIN`).

Parsed today into: `SonardyneRangeReplyData` / `SV3ReplyData` → `AcousticDataFrame` (`transponderID`, `pingTime`, `returnTime`, `tt`, `dbv`, `xc`, `snr`, `tat`).

Note: the current parser stores `tt = range − tat − TRIGGER_DELAY_SV3` (`sonardyne_tools/sv3_operations.py`), i.e. a *corrected* travel time, not the raw two-way range. To stay true to "Level 1 = raw", this table stores the raw range and the turn-around time; the correction is applied when building Level 2 (decision 5).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `MT_ID` | `string` | required | ID of mirror / seafloor transponder (was `transponderID`) |
| `T_transmit` | `timestamptz` | required | Transmission (ping) time of acoustic signal, UTC (was `pingTime`, GPS seconds) |
| `T_receive` | `timestamptz` | required | Reception (return) time of acoustic signal, UTC (was `returnTime`, GPS seconds) |
| `TravelTime` | `double` | 0 ≤ x ≤ 600 | Raw two-way range as logged \[s\] — authoritative; do not recompute from `T_receive − T_transmit` |
| `dbV` | `float` | — | Decibel-voltage signal strength (int in `AcousticDataFrame`, float32 in TileDB and DSTF — stored as float) |
| `acc` | `int` | 0 ≤ x ≤ 100 | Cross-correlation accuracy % (was `xc`; DSTF's name for this quantity) |
| `aSNR` | `double` | −100 ≤ x ≤ 100, nullable | Signal-to-noise ratio \[dB\] (was `snr`) |
| `tat` | `double` | 0 ≤ x ≤ 10, nullable | Beacon turn-around time \[s\] (converted from ms at parse time) — no DSTF equivalent, Level-1-only |

### INSPVA

Source files: NovAtel `NOV000.bin` (`INSPVAA` log).

Parsed today into: `INSPVAASchema` (`community/posfilter_models.py`).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `time` | `timestamptz` | required | Epoch, UTC (replaces raw `week`+`seconds` and the J2000 `time` float; GPS-week/TOW derivable by the API if a consumer needs it) |
| `lat` | `double` | −90 ≤ x ≤ 90 | Geodetic latitude \[deg\] |
| `lon` | `double` | −180 ≤ x ≤ 180 | Geodetic longitude \[deg\] |
| `alt` | `double` | — | Geodetic altitude \[m\] |
| `north` | `double` | — | Velocity, local-tangent North \[m/s\] |
| `east` | `double` | — | Velocity, local-tangent East \[m/s\] |
| `up` | `double` | — | Velocity, local-tangent Up \[m/s\] |
| `roll` | `double` | −180 ≤ x ≤ 180 | Roll angle \[deg\] |
| `pitch` | `double` | −90 ≤ x ≤ 90 | Pitch angle \[deg\] |
| `heading` | `double` | 0 ≤ x < 360 | Heading angle \[deg\] |

### INSSTDEV

Source files: NovAtel `NOV000.bin` (`INSSTDEVA` log).

Parsed today into: `INSSTDEVSchema` (`community/posfilter_models.py`).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `time` | `timestamptz` | required | Epoch, UTC (replaces raw `week`+`seconds` and the J2000 `time` float) |
| `lat_sig` | `double` | ≥ 0.0 | Latitude std dev \[deg\] |
| `lon_sig` | `double` | ≥ 0.0 | Longitude std dev \[deg\] |
| `alt_sig` | `double` | ≥ 0.0 | Altitude std dev \[m\] |
| `north_sig` | `double` | ≥ 0.0 | North velocity std dev \[m/s\] |
| `east_sig` | `double` | ≥ 0.0 | East velocity std dev \[m/s\] |
| `up_sig` | `double` | ≥ 0.0 | Up velocity std dev \[m/s\] |
| `cov_rr` | `double` | ≥ 0.0 | Roll std dev \[deg\], as logged (GNATSS's name; NovAtel logs σ, not variance) |
| `cov_pp` | `double` | ≥ 0.0 | Pitch std dev \[deg\], as logged |
| `cov_hh` | `double` | ≥ 0.0 | Heading (azimuth) std dev \[deg\], as logged |
| `ext_sol_stat` | `string` | — | Extended solution status (present in `INSSTDEVSchema`; was missing here) |
| `time_since_update` | `string` | — | Time since last update |

### GNSS Observations

Source files:

- NovAtel ASCII (`NOVATEL`): `RANGEA` log
- NovAtel `NOV000.bin`: `RANGEA` log
- NovAtel `NOV770` binary: `RANGECMP` (ID 140) and `RANGECMP5` (ID 2537) logs, per antenna (`antindex` 0/1)
- Sonardyne QC PIN JSON (`QCPIN`): `RANGEA` strings in the `NOV_RANGE` block

Parsed today into: TileDB `GNSSObsSchema` (`tiledb_integration/schemas.py`) — dims `time` (int64 ms), `sys`, `sat`, `obs`; attrs `range`, `phase`, `doppler`, `snr`, `slip`, `flags`, `fcn`. This table replaces it with one row per (epoch, satellite, signal) and µs time + ps remainder instead of ms.

Note: we might need to add another field to distinguish file source as well. `Log_Source` will help keep multiple antenna observations separate.

**Implemented differently:** the table follows the gnsstools `observations` layout (`station_id`, `session_id`, `epoch` in GPST, `epoch_pico`, `constellation` as `GPS`/`GLO`/…) with `Network`, `Campaign`, `Log_Source` appended — see `datamodels/iceberg/schemas/level1.py`. Parsing uses the gnsstools NovAtel codecs via `sfg nov2level1`; NOV770 antennas get `Log_Source` `NOV770_ANT<n>`. The table below is the original proposal.

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `time` | `timestamptz` | required | Observation epoch, UTC (replaces `time_utc` + `time_gpst` — GPST wall clock is now an API-side derivation, not a stored column, removing the old gotcha where `time_gpst` lived in a `timestamptz` column without actually holding a UTC instant) |
| `time_sub_us_ps` | `int` | required, 0 ≤ x < 10⁶ | Sub-microsecond remainder of the epoch, in picoseconds |
| `clock_offset` | `double` | nullable | Receiver clock offset \[s\] |
| `constellation` | `string` | required, one of G R E C J I S | One-character RINEX constellation code |
| `satellite_id` | `int` | required | RINEX satellite number: PRN, or slot number for GLONASS |
| `signal` | `string` | required | RINEX 3 observation code, e.g. 1C |
| `code` | `double` | nullable | Pseudorange \[m\] (TileDB `range`) |
| `phase` | `double` | nullable | Carrier phase \[cycles\] |
| `doppler` | `double` | nullable | Doppler \[Hz\] |
| `snr` | `double` | nullable | Carrier-to-noise density \[dB-Hz\] |
| `loss_of_lock` | `boolean` | nullable | RINEX LLI bit 0; null when there is no phase observation |
| `half_cycle_ambiguity` | `boolean` | nullable | RINEX LLI bit 1; null when there is no phase observation |
| `boc_tracking` | `boolean` | nullable | RINEX LLI bit 2; null when there is no phase observation |
| `lock_time` | `double` | nullable | Continuous phase lock time \[s\] |
| `epoch_flag` | `int` | nullable | 0 = OK, \>1 = event |
| `fcn` | `int` | nullable, −7 ≤ x ≤ 6 | GLONASS frequency channel number; null for other constellations or when unknown — 0 is a valid channel, not a null-sentinel |
| `write_time` | `timestamptz` | nullable | When this row was written to Iceberg |
| `arrival_time` | `timestamptz` | nullable | When the epoch arrived at the ingest pipeline (`arrival_time − time` = ingest latency) |

### Attitude (AHRS)

Source files: Sonardyne DFOP00 JSONL and Sonardyne QC PIN JSON (`QCPIN`): the `AHRS` block of SV3 events.

Parsed today into: `SonardyneAHRSData` + `TimeData` (`parsing/sv3_models.py`).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `time` | `timestamptz` | required | Epoch, UTC (replaces the raw `time.common` UNIX decimal and the hardcoded `+LEAP_SECONDS` GPS-time approximation currently used downstream — real leap-second handling moves to the API) |
| `time_instrument` | `double` | ≥ 0 | Instrument-relative time \[s\] — a device clock reading, not an absolute instant; kept separately for drift/reset diagnostics |
| `time_start_count` | `int` | ≥ 0 | Start count for the time |
| `time_status` | `string` | — | Status of the time data |
| `acx` | `double` | — | Acceleration, X axis \[m/s²\] |
| `acy` | `double` | — | Acceleration, Y axis \[m/s²\] |
| `acz` | `double` | — | Acceleration, Z axis \[m/s²\] |
| `h` | `double` | 0 ≤ x ≤ 360 | Heading \[deg\] |
| `h_mag` | `double` | 0 ≤ x ≤ 360, nullable | Magnetic heading \[deg\] |
| `p` | `double` | −90 ≤ x ≤ 90 | Pitch \[deg\] |
| `r` | `double` | −180 ≤ x ≤ 180 | Roll \[deg\] |

### Sound Speed

Source files: CTD casts (`CTD`); Sea-Bird profiles (`SEABIRD`, including `svpavg` files).

Parsed today into: `SoundVelocityDataFrame` (`soundvelocity.py`).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `time` | `timestamptz` | required | Cast/profile time, UTC — **new**; the original schema had no time axis at all, which breaks "query everything by time" as a lake-wide guarantee |
| `depth` | `double` | 0 ≤ x ≤ 10000 | Depth \[m\] |
| `speed` | `double` | 0 ≤ x ≤ 3800 | Sound speed \[m/s\] (the pandera model also requires `speed` unique per profile to keep interpolation well-posed — enforce per `time`, not table-wide) |

### GNSS antenna positions

Source files: Sonardyne DFOP00 JSONL and Sonardyne QC PIN JSON (`QCPIN`): the `GNSS` block of SV3 events.

Parsed today into: `SonardyneGNSSData` (`latitude`, `longitude`, `hae`, `q`, `sdx/sdy/sdz`). Column names match GNATSS's `ANT_GPS_GEOCENTRIC(_STD)`. Today these columns are built ad hoc in `earthscope-sfg-workflows` (`pipelines/shotdata_gnss_refinement.py`), which fills `ant_sigx` from `latitude_std` in degrees — this table replaces that with metres-valued ECEF sigmas.

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* + `Log_Source` | | | |
| `time` | `timestamptz` | required | Epoch, UTC (was a bare float of unspecified epoch) |
| `ant_x` | `double` | required | Antenna ECEF position X \[m\] (from lat/lon/hae at ingest) |
| `ant_y` | `double` | required | Antenna ECEF position Y \[m\] |
| `ant_z` | `double` | required | Antenna ECEF position Z \[m\] |
| `ant_sigx` | `double` | ≥ 0, nullable | Antenna position std dev X, ECEF \[m\] — rotated from logged ENU `sdx/sdy/sdz`; null when Sonardyne reports the −1.0 "not available" sentinel |
| `ant_sigy` | `double` | ≥ 0, nullable | Antenna position std dev Y, ECEF \[m\] |
| `ant_sigz` | `double` | ≥ 0, nullable | Antenna position std dev Z, ECEF \[m\] |
| `q` | `int` | 0–9 | GPS fix quality (`SV3GPSQuality`: 0 = no fix, 1 = single point, 4 = RTK fixed, 5 = RTK float, …) |
| `number_of_satellites` | `int` | ≥ 0, nullable | Number of satellites used in the solution |
| `pdop` | `double` | ≥ 0, nullable | Position dilution of precision |
| `wrms` | `double` | ≥ 0, nullable | Weighted RMS of position residuals \[m\] |

`number_of_satellites`, `pdop` and `wrms` match the Level 2 GNSS antenna positions table. The SV3 `GNSS` block doesn't report them, so they're null for SV3-sourced rows today.

### GNSS antenna positions (Level 2)

Source files: none directly. Derived from PPP kinematic solutions computed from the Level 1 GNSS observations.

Parsed today into: `KinPositionDataFrame` (`parsing/ppp.py`) / TileDB `KinPositionArraySchema`. It has the same columns as the Level 1 GNSS antenna positions table, and there is no separate PPP table. No `Log_Source` (decision 3). PPP `latitude/longitude/height` are converted to ECEF at ingest (decision 6).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* | | | |
| `time` | `timestamptz` | required | Solution epoch, UTC |
| `ant_x` | `double` | required | Antenna ECEF position X \[m\] |
| `ant_y` | `double` | required | Antenna ECEF position Y \[m\] |
| `ant_z` | `double` | required | Antenna ECEF position Z \[m\] |
| `ant_sigx` | `double` | ≥ 0, nullable | Antenna position std dev X, ECEF \[m\] |
| `ant_sigy` | `double` | ≥ 0, nullable | Antenna position std dev Y, ECEF \[m\] |
| `ant_sigz` | `double` | ≥ 0, nullable | Antenna position std dev Z, ECEF \[m\] |
| `q` | `int` | 0–9, nullable | GPS fix quality; null for PPP rows |
| `number_of_satellites` | `int` | ≥ 0 | Number of satellites used in the solution |
| `pdop` | `double` | ≥ 0 | Position dilution of precision |
| `wrms` | `double` | ≥ 0 | Weighted RMS of position residuals \[m\] |

### GNSS-A GARPOS format

Source files: none directly. Derived from Level 1 travel times, GNSS antenna positions and INSPVA (Level 3: Level 2 GNSS antenna positions).

Built today as: `ObservationData` (`earthscope-sfg-workflows/modeling/garpos_tools/schemas.py`). Antenna positions are local ENU about `Site.arrayCenter`, georectified from ECEF by the API at build time (decision 6).

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* | | | |
| `SET` | `string` | default `S01` | Name of observation subset |
| `LN` | `string` | default `L01` | Name of survey line |
| `MT` | `string` | required | ID of mirror transponder (must match the site-parameter file) |
| `TT` | `double` | required | Corrected travel time \[s\] (raw range − `tat` − trigger delay) |
| `ResiTT` | `double` | default 0.0 | Travel-time residual \[ms\] (populated by the GARPOS run) |
| `TakeOff` | `double` | default 0.0 | Ray-path takeoff angle \[deg\] (populated by the GARPOS run) |
| `gamma` | `double` | default 0.0 | Sound-speed variation correction term \[m/s\] |
| `flag` | `boolean` | default false | True = this shot is excluded from processing (mis-response) |
| `ST` | `timestamptz` | required | Transmission time of acoustic signal, UTC (API converts to GARPOS's native seconds-of-day at read time, given a reference date) |
| `ant_e0` | `double` | required | GNSS antenna position, eastward of the array center, at `ST` \[m\] |
| `ant_n0` | `double` | required | GNSS antenna position, northward of the array center, at `ST` \[m\] |
| `ant_u0` | `double` | required | GNSS antenna position, upward of the array center, at `ST` \[m\] |
| `head0` | `double` | required | Heading at `ST` \[deg\] |
| `pitch0` | `double` | required | Pitch at `ST` \[deg\] |
| `roll0` | `double` | required | Roll at `ST` \[deg\] |
| `RT` | `timestamptz` | required | Reception time of acoustic signal, UTC (same API conversion as `ST`) |
| `ant_e1` | `double` | required | GNSS antenna position, eastward of the array center, at `RT` \[m\] |
| `ant_n1` | `double` | required | GNSS antenna position, northward of the array center, at `RT` \[m\] |
| `ant_u1` | `double` | required | GNSS antenna position, upward of the array center, at `RT` \[m\] |
| `head1` | `double` | required | Heading at `RT` \[deg\] |
| `pitch1` | `double` | required | Pitch at `RT` \[deg\] |
| `roll1` | `double` | required | Roll at `RT` \[deg\] |

### GNSS-A Community standard format

Source files: derived from Level 1 travel times, GNSS antenna positions and INSPVA (Level 3: Level 2 GNSS antenna positions). Transducer positions also need the antenna-to-transducer offsets from the lever arms file (`LEVERARM`); nothing parses that file today, so the offsets come from site metadata (`Campaign.vessel.atdOffsets`).

Built today as: `SFGDSTFSeafloorAcousticData` (`community/community_standards.py`). Optional columns are nullable.

| Field | Type | Constraints | Description |
| --- | --- | --- | --- |
| *scope* | | | |
| `MT_ID` | `string` | required | ID of mirror transponder |
| `TravelTime` | `double` | required, ≥ 0 | Corrected travel time \[s\] (raw range − `tat` − trigger delay) |
| `T_transmit` | `timestamptz` | required | Transmission time, UTC |
| `X_transmit` | `double` | required | Transmit position X, ECEF \[m\] |
| `Y_transmit` | `double` | required | Transmit position Y, ECEF \[m\] |
| `Z_transmit` | `double` | required | Transmit position Z, ECEF \[m\] |
| `T_receive` | `timestamptz` | required | Reception time, UTC |
| `X_receive` | `double` | required | Receive position X, ECEF \[m\] |
| `Y_receive` | `double` | required | Receive position Y, ECEF \[m\] |
| `Z_receive` | `double` | required | Receive position Z, ECEF \[m\] |
| `TDC_ID` | `string` | optional | Transducer ID |
| `aSNR` | `double` | optional | Signal-to-noise ratio \[dB\] |
| `acc` | `int` | optional, 0 ≤ x ≤ 100 | Cross-correlation accuracy \[%\] |
| `dbV` | `float` | optional | Decibel-voltage signal strength |
| `quality_flag` | `string` | optional | Shot quality flag |
| `ant_X0` | `double` | optional | Antenna position X, ECEF, at transmit \[m\] |
| `ant_Y0` | `double` | optional | Antenna position Y, ECEF, at transmit \[m\] |
| `ant_Z0` | `double` | optional | Antenna position Z, ECEF, at transmit \[m\] |
| `ant_sigX0` | `double` | optional, ≥ 0 | Antenna position std dev X at transmit \[m\] |
| `ant_sigY0` | `double` | optional, ≥ 0 | Antenna position std dev Y at transmit \[m\] |
| `ant_sigZ0` | `double` | optional, ≥ 0 | Antenna position std dev Z at transmit \[m\] |
| `ant_X1` | `double` | optional | Antenna position X, ECEF, at receive \[m\] |
| `ant_Y1` | `double` | optional | Antenna position Y, ECEF, at receive \[m\] |
| `ant_Z1` | `double` | optional | Antenna position Z, ECEF, at receive \[m\] |
| `ant_sigX1` | `double` | optional, ≥ 0 | Antenna position std dev X at receive \[m\] |
| `ant_sigY1` | `double` | optional, ≥ 0 | Antenna position std dev Y at receive \[m\] |
| `ant_sigZ1` | `double` | optional, ≥ 0 | Antenna position std dev Z at receive \[m\] |
| `heading0` | `double` | optional, 0 ≤ x ≤ 360 | Heading at transmit \[deg\] |
| `pitch0` | `double` | optional, −90 ≤ x ≤ 90 | Pitch at transmit \[deg\] |
| `roll0` | `double` | optional, −180 ≤ x ≤ 180 | Roll at transmit \[deg\] |
| `heading1` | `double` | optional, 0 ≤ x ≤ 360 | Heading at receive \[deg\] — **missing from `SFGDSTFSeafloorAcousticData` today** |
| `pitch1` | `double` | optional, −90 ≤ x ≤ 90 | Pitch at receive \[deg\] — **missing from `SFGDSTFSeafloorAcousticData` today** |
| `roll1` | `double` | optional, −180 ≤ x ≤ 180 | Roll at receive \[deg\] |
