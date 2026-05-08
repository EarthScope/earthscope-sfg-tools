# sv3_models

`earthscope_sfg_tools.datamodels.observationdata.parsing.sv3_models`

Pydantic models representing Sonardyne/SV3 event payloads.

## class `NovatelInterrogationEvent`

Full Sonardyne interrogation event parsed from a DFOP00 JSONL log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `event` | `str` |  |
| `event_id` | `int` | Tracking-cycle ID |
| `observations` | `SonardyneObservations` | Observations |
| `sequence` | `int` | Sequence ID |
| `time` | `TimeData` | Event time |
| `type` | `str` | Interrogation type |

## class `NovatelRangeEvent`

Full Sonardyne range (reply) event parsed from a DFOP00 JSONL log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `event` | `str` |  |
| `event_id` | `int` | Tracking-cycle ID |
| `observations` | `SonardyneObservations` | Event observations |
| `range` | `SonardyneRangeReplyData` | Range reply data |
| `sequence` | `int` | Sequence ID |
| `time` | `TimeData` | Event time |
| `uid` | `Union` | Unique identifier |

## class `SV3GPSQuality`

GPS fix quality codes reported in Sonardyne GNSS data.

## class `SonardyneAHRSData`

AHRS attitude (heading, pitch, roll) and acceleration from a Sonardyne event log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `acx` | `Decimal` | Acceleration X axis in m/s^2 |
| `acy` | `Decimal` | Acceleration Y axis in m/s^2 |
| `acz` | `Decimal` | Acceleration Z axis in m/s^2 |
| `h` | `Decimal` | Heading in degrees |
| `h_mag` | `Union` | Magnetic heading in degrees |
| `p` | `Decimal` | Pitch in degrees |
| `r` | `Decimal` | Roll in degrees |
| `time` | `TimeData` | Time data associated with the log |

## class `SonardyneGNSSData`

GNSS position fix and standard deviations from a Sonardyne event log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `hae` | `Decimal` | Height above ellipsoid in meters |
| `latitude` | `Decimal` | Latitude in degrees |
| `longitude` | `Decimal` | Longitude in degrees |
| `q` | `SV3GPSQuality` | Quality indicator |
| `sdx` | `Union` | Standard deviation east [m] |
| `sdy` | `Union` | Standard deviation north [m] |
| `sdz` | `Union` | Standard deviation up [m] |
| `separation` | `Union` | Separation |
| `time` | `TimeData` | Time data associated with the log |

## class `SonardyneHeadingData`

GNSS dual-antenna heading solution from a Sonardyne event log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `gpst` | `Decimal` | GPS time in seconds since GNSS start time |
| `h` | `Decimal` | GNSS Computed Heading in degrees |
| `p` | `Decimal` | GNSS Computed Pitch in degrees |
| `position_type` | `SonardynePositionType` | Type of position data |
| `receiver_status` | `str` | Status of the receiver |
| `sdh` | `Union` | Standard deviation of heading in degrees |
| `sdp` | `Union` | Standard deviation of pitch in degrees |
| `solution_type` | `SonardyneSolutionStatus` | Solution type of the heading data |
| `sv_used` | `int` | Number of satellites used in the solution |
| `sv_visible` | `int` | Number of satellites visible |
| `time` | `TimeData` | Time data associated with the log |

## class `SonardyneINSData`

NovAtel SPAN INS attitude and velocity solution from a Sonardyne event log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `gpst` | `Decimal` | GPS time in seconds since GNSS start time |
| `h` | `Decimal` | SPAN INS Computed Heading in degrees |
| `p` | `Decimal` | SPAN INS Computed Pitch in degrees |
| `r` | `Decimal` | SPAN INS Computed Roll in degrees |
| `receiver_status` | `str` | Status of the receiver |
| `solution_type` | `SonardyneSolutionStatus` | Solution type of the INS data |
| `time` | `TimeData` | Time data associated with the log |
| `velx` | `Decimal` | SPAN INS measured acceleration X axis in m/s^2 |
| `vely` | `Decimal` | SPAN INS measured acceleration Y axis in m/s^2 |
| `velz` | `Decimal` | SPAN INS measured acceleration Z axis in m/s^2 |

## class `SonardyneInterrogationEvent`

Full Sonardyne interrogation event parsed from a DFOP00 JSONL log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `event` | `str` |  |
| `event_id` | `int` | Tracking-cycle ID |
| `observations` | `SonardyneObservations` | Observations |
| `sequence` | `int` | Sequence ID |
| `time` | `TimeData` | Event time |
| `type` | `str` | Interrogation type |

## class `SonardyneObservations`

Bundle of all sensor observations attached to one Sonardyne event.

**Fields**

| Name | Type | Description |
|---|---|---|
| `AHRS` | `Union` | AHRS data |
| `GNSS` | `Union` | GNSS data |
| `NOV_HEADING` | `Union` | Sonardyne heading data |
| `NOV_INS` | `Union` | Sonardyne INS data |
| `NOV_RANGE` | `Union` | Sonardyne range data |

## class `SonardynePositionType`

NovAtel/Sonardyne position type identifiers.

## class `SonardyneRangeData`

Raw range string and timestamp from a Sonardyne acoustic reply.

**Fields**

| Name | Type | Description |
|---|---|---|
| `raw` | `str` | Raw range data as a string |
| `time` | `TimeData` | Time data associated with the range data |

## class `SonardyneRangeDiagnosticData`

Acoustic signal quality diagnostics for one transponder reply.

**Fields**

| Name | Type | Description |
|---|---|---|
| `dbv` | `Decimal` | Decibel voltage in volts |
| `snr` | `Decimal` | Signal-to-noise ratio in dB |
| `xc` | `Decimal` | Cross-correlation % - signal quality |

## class `SonardyneRangeEvent`

Full Sonardyne range (reply) event parsed from a DFOP00 JSONL log.

**Fields**

| Name | Type | Description |
|---|---|---|
| `event` | `str` |  |
| `event_id` | `int` | Tracking-cycle ID |
| `observations` | `SonardyneObservations` | Event observations |
| `range` | `SonardyneRangeReplyData` | Range reply data |
| `sequence` | `int` | Sequence ID |
| `time` | `TimeData` | Event time |
| `uid` | `Union` | Unique identifier |

## class `SonardyneRangeReplyData`

Acoustic range reply from one transponder, including TAT and diagnostics.

**Fields**

| Name | Type | Description |
|---|---|---|
| `cn` | `str` | transponder ID |
| `diag` | `SonardyneRangeDiagnosticData` | Range diagnostic data |
| `range` | `Decimal` | Two-way travel time in seconds |
| `tat` | `Decimal` | Beacon turn around time in seconds |

## class `SonardyneSolutionStatus`

NovAtel/Sonardyne position solution status codes.

## class `TimeData`

Timestamp block from a Sonardyne SV3 event JSON.

**Fields**

| Name | Type | Description |
|---|---|---|
| `common` | `Decimal` | TZ unaware UNIX time |
| `instrument` | `Decimal` | Instrument time in seconds |
| `start_count` | `int` | Start count for the time |
| `status` | `str` | Status of the time data |
