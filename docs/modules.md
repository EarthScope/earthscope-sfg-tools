# Modules

This page describes the main package modules and key public functions/classes.

## datamodels

Data model layer for metadata, validated tabular data, and typed observation records.

### metadata

Core metadata models and import utilities.

| Symbol | Type | Description |
|---|---|---|
| `Site` | class | EarthScope site metadata model. |
| `Vessel` | class | Vessel metadata model used for campaign and survey context. |
| `Campaign` | class | Campaign-level metadata model. |
| `Survey` | class | Survey metadata model. |
| `Benchmark` | class | Benchmark metadata model. |
| `MetaDataCatalog` | class | Metadata catalog container and access model. |
| `SFGDTSFSite` | class | Community-format site metadata model. |
| `classify_survey_type` | function | Classifies survey type from available metadata fields. |
| `import_site` | function | Imports/parses a site metadata representation into model form. |
| `import_vessel` | function | Imports/parses a vessel metadata representation into model form. |

### observationdata

Validated observation and parsing schemas used across GNSS/acoustics workflows.

| Symbol | Type | Description |
|---|---|---|
| `AcousticDataFrame` | class | Validated acoustic observation dataframe schema. |
| `GARPOSShotDataFrame` | class | GARPOS-compatible shot-data schema. |
| `SFGDSTFSeafloorAcousticData` | class | Community seafloor acoustic data model. |
| `KinPositionDataFrame` | class | Kinematic GNSS position dataframe schema. |
| `IMUPositionDataFrame` | class | IMU position dataframe schema. |
| `INSPVAASchema` | class | NovAtel INS position/velocity/attitude schema. |
| `INSSTDEVSchema` | class | NovAtel INS standard-deviation schema. |
| `SoundVelocityDataFrame` | class | Sound velocity profile dataframe schema. |
| `NovatelInterrogationEvent` | class | Parsed NovAtel interrogation event record. |
| `NovatelRangeEvent` | class | Parsed NovAtel range event record. |
| `SV3InterrogationData` | class | Sonardyne SV3 interrogation model. |
| `SV3ReplyData` | class | Sonardyne SV3 reply model. |

## novatel_tools

NovAtel conversion, parsing, and helper operations for GNSS/RINEX workflows.

| Symbol | Type | Description |
|---|---|---|
| `nova2rnx` | function | Converts supported NovAtel logs to RINEX output. |
| `novatel_ascii_2rinex` | function | Converts NovAtel ASCII input to RINEX. |
| `nov0002rnx` | function | Runs NOV000 conversion path to RINEX. |
| `novatel_binary_2rinex` | function | Converts NovAtel binary input to RINEX. |
| `novatel_2rinex` | function | Alias for binary NovAtel-to-RINEX conversion. |
| `rnxqc` | function | Runs RINEX quality control workflow. |
| `deserialize_rangea` | function | Parses a RANGEA payload into typed records. |
| `extract_rangea_from_qcpin` | function | Extracts RANGEA blocks from QCPIN content. |
| `epoch_to_dict` | function | Converts parsed epoch records into dictionary form. |
| `GNSSEpoch` | class | Parsed GNSS epoch container. |
| `GNSSSystem` | class/enum | GNSS constellation/system identifier type. |
| `Observation` | class | GNSS observation record model. |
| `Satellite` | class | Satellite observation grouping model. |
| `MetadataModel` | class | Shared NovAtel metadata model. |

## seafloor_site_tools

Sound speed processing helpers for CTD and Seabird-derived workflows.

| Symbol | Type | Description |
|---|---|---|
| `seabird_to_soundvelocity` | function | Converts Seabird CTD-style data into sound velocity records. |
| `ctd_to_svp_v1` | function | Builds an SVP (v1 method) from CTD input. |
| `ctd_to_svp_v2` | function | Builds an SVP (v2 method) from CTD input. |
| `interpolate_svp` | function | Interpolates a sound velocity profile onto target depths. |

## sonardyne_tools

SV3 acoustic parsing and data-merging utilities for shot-level processing.

| Symbol | Type | Description |
|---|---|---|
| `dfop00_to_sfgdstf_seafloor_acoustic_data` | function | Converts DFOP00 records into community seafloor acoustic format. |
| `dfop00_to_shotdata` | function | Converts DFOP00 payloads into shot-data rows. |
| `merge_interrogation_reply` | function | Merges interrogation and reply streams into paired observations. |
| `novatel_interrogation_to_garpos_interrogation` | function | Maps NovAtel interrogation records to GARPOS format. |
| `novatel_reply_to_garpos_reply` | function | Maps NovAtel reply records to GARPOS format. |
| `qcjson_to_shotdata` | function | Converts QC JSON output to shot-data table format. |
| `batch_qc_by_day` | function | Runs QC processing grouped by day windows. |

## tiledb_integration (optional)

Optional TileDB-backed schemas and array wrappers. This module requires the `tiledb` extra.

| Symbol | Type | Description |
|---|---|---|
| `TBDArray` | class | Base TileDB array wrapper used by typed array integrations. |
| `TDBAcousticArray` | class | TileDB array wrapper for acoustic observations. |
| `TDBGNSSObsArray` | class | TileDB array wrapper for GNSS observation records. |
| `TDBIMUPositionArray` | class | TileDB array wrapper for IMU position records. |
| `TDBKinPositionArray` | class | TileDB array wrapper for kinematic position records. |
| `TDBShotDataArray` | class | TileDB array wrapper for shot-data records. |
| `AcousticArraySchema` | class/object | Acoustic TileDB schema definition. |
| `GNSSObsSchema` | class/object | GNSS observation TileDB schema definition. |
| `IMUPositionArraySchema` | class/object | IMU position TileDB schema definition. |
| `KinPositionArraySchema` | class/object | Kinematic position TileDB schema definition. |
| `ShotDataArraySchema` | class/object | Shot data TileDB schema definition. |
| `config` | object | TileDB configuration object. |
| `ctx` | object | TileDB context object. |
| `filters` | object | Shared TileDB filter definitions. |
| `tile2rinex` | function | Placeholder; not yet implemented in this package. |
| `rinex_qc` | function | Placeholder; not yet implemented in this package. |
| `novatel_770_2tile` | function | Placeholder; not yet implemented in this package. |
