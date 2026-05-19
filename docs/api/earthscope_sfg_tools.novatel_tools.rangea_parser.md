# rangea_parser

`earthscope_sfg_tools.novatel_tools.rangea_parser`

RANGEA ASCII Log Parser

This module provides Python functions to parse NovAtel RANGEA ASCII log strings
into structured observation data, inspired by the Go-lang GNSS tools implementation using
novatelascii.DeserializeRANGEA and observation.Epoch.

The RANGEA log contains GNSS pseudorange, carrier phase, Doppler, and C/N0
measurements for all tracked satellites across multiple constellations.

## class `GNSSEpoch`

A GNSS observation epoch containing all satellite measurements at one time.

This is the Python equivalent of Go's observation.Epoch structure.
An epoch represents all GNSS observations recorded at a single instant,
typically at the receiver's measurement rate (e.g., 1 Hz, 10 Hz).

Attributes:
    time: UTC timestamp of the epoch
    gps_week: GPS week number
    gps_seconds: Seconds into the GPS week
    satellites: Dictionary mapping (system, prn) tuple to Satellite
    receiver_status: Raw receiver status word from header
    num_observations: Total number of observation records

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `datetime` |  |
| `gps_week` | `int` |  |
| `gps_seconds` | `float` |  |
| `satellites` | `dict` |  |
| `receiver_status` | `str` |  |
| `num_observations` | `int` |  |

**Methods**

### `GNSSEpoch.add_satellite(self, sat: earthscope_sfg_tools.novatel_tools.rangea_parser.Satellite) -> None`

Add or update a satellite in this epoch.

### `GNSSEpoch.get_satellite(self, system: earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSSystem, prn: int) -> earthscope_sfg_tools.novatel_tools.rangea_parser.Satellite | None`

Get a satellite by system and PRN.

### `GNSSEpoch.get_systems(self) -> list[earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSSystem]`

Return list of GNSS systems present in this epoch.


## class `GNSSSystem`

GNSS constellation identifiers from NovAtel channel tracking status.

## class `Observation`

A single GNSS observation for one signal from one satellite.

This corresponds to a single observation record within a RANGEA message,
containing pseudorange, carrier phase, Doppler, and signal quality metrics.

**Fields**

| Name | Type | Description |
|---|---|---|
| `signal_type` | `int` |  |
| `pseudorange` | `float` | Pseudorange measurement in meters |
| `pseudorange_std` | `float` | Pseudorange standard deviation in meters |
| `carrier_phase` | `float` | Accumulated Doppler range (ADR) in cycles |
| `carrier_phase_std` | `float` | Carrier phase standard deviation in cycles |
| `doppler` | `float` | Doppler frequency shift in Hz |
| `cn0` | `float` | Carrier-to-noise density ratio in dB-Hz |
| `locktime` | `float` | Continuous tracking time in seconds |
| `tracking_status` | `int` | Raw 32-bit channel tracking status word |
| `half_cycle_ambiguity` | `bool` | True if half-cycle ambiguity is present |
| `phase_lock` | `bool` | True if phase is locked |
| `code_lock` | `bool` | True if code is locked |
| `parity_known` | `bool` | True if parity is known (for navigation data) |

## class `Satellite`

GNSS satellite with all its observations.

A satellite may have multiple observations for different signals
(e.g., GPS satellite might have L1CA, L2C, and L5 observations).

Attributes:
    system: GNSS constellation (GPS, GLONASS, Galileo, etc.)
    prn: Satellite PRN number (or slot for GLONASS)
    fcn: GLONASS frequency channel number (-7 to +6), 0 for other systems
    observations: Dictionary mapping signal type to Observation

**Fields**

| Name | Type | Description |
|---|---|---|
| `system` | `GNSSSystem` |  |
| `prn` | `int` |  |
| `fcn` | `int` |  |
| `observations` | `dict` |  |

**Methods**

### `Satellite.add_observation(self, obs: earthscope_sfg_tools.novatel_tools.rangea_parser.Observation) -> None`

Add an observation for a specific signal type.


## class `SignalType`

Common GNSS signal types (simplified mapping).

## `deserialize_rangea(rangea_string: str) -> earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSEpoch`

Parse a NovAtel RANGEA ASCII log string into an Epoch object.

This function is the Python equivalent of the Go code:
    rangea, err := novatelascii.DeserializeRANGEA(m.Data)
    epoch, err := rangea.SerializeGNSSEpoch(m.Time())

RANGEA Format:
    #RANGEA,<header>;num_obs,<obs1>,...,<obsN>*checksum

Each observation has 10 fields:
    prn, glo_freq, psr, psr_std, adr, adr_std, dopp, cn0, locktime, ch_tr_status

Args:
    rangea_string: Complete RANGEA ASCII log string including header and checksum

Returns:
    Epoch object containing all parsed satellite observations

Raises:
    ValueError: If the string cannot be parsed as a valid RANGEA message

Example:
    >>> rangea = "#RANGEA,USB2,0,73.5,FINESTEERING,2379,414835.000,..."
    >>> epoch = deserialize_rangea(rangea)
    >>> print(f"Epoch time: {epoch.time}, satellites: {epoch.satellite_count}")

## `epoch_to_dict(epoch: earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSEpoch) -> dict`

Convert an Epoch object to a dictionary for serialization.

Args:
    epoch: Epoch object to convert

Returns:
    Dictionary representation suitable for JSON serialization

## `extract_rangea_from_qcpin(source: str | pathlib.Path) -> list[earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSEpoch]`

Extract and parse all RANGEA logs from a QC PIN JSON file.

Thin I/O wrapper around :func:`parse_rangea_epochs_from_dict`.

Args:
    source: Path to the QC PIN file in JSON format.

Returns:
    List of unique GNSSEpoch objects.  Empty list on read/parse error.

## `extract_rangea_strings_from_qcpin(source: str | pathlib.Path) -> list[str]`

Extract raw RANGEA strings from a QC PIN JSON file.

Thin I/O wrapper around :func:`extract_rangea_strings_from_qcpin_dict`.

Args:
    source: Path to the QC PIN file in JSON format.

Returns:
    List of unique raw RANGEA strings.  Empty list on read/parse error.

## `extract_rangea_strings_from_qcpin_dict(data: dict) -> list[str]`

Extract raw RANGEA strings from a decoded QC PIN dict.

Pure function: no filesystem access.

Args:
    data: Decoded JSON object from a QC PIN file.

Returns:
    List of unique raw RANGEA strings found in the dict.

## `parse_rangea_epochs_from_dict(data: dict) -> list[earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSEpoch]`

Extract and parse all RANGEA logs from a pre-decoded QC PIN dict.

Pure function: no filesystem access.  Pass the result of ``json.load()``.

Args:
    data: Decoded JSON object from a QC PIN file.

Returns:
    List of unique GNSSEpoch objects.  Empty list if no valid RANGEA logs.
