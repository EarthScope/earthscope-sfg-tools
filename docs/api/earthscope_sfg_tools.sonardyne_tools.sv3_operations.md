# sv3_operations

`earthscope_sfg_tools.sonardyne_tools.sv3_operations`

_No docstring._

## class `PairResult`

Outcome of pairing one interrogation with one reply.

**Fields**

| Name | Type | Description |
|---|---|---|
| `data` | `dict \| None` |  |
| `rejection` | `RejectionReason \| None` |  |

## class `RejectionReason`

_No docstring._

## class `SV3PairingRules`

Thresholds governing interrogation/reply pair validation.

Defaults reproduce the original hardcoded assertion values.

**Fields**

| Name | Type | Description |
|---|---|---|
| `max_roundtrip_seconds` | `float` |  |
| `min_range_metres` | `float` |  |
| `return_time_tolerance_seconds` | `float` |  |

## `build_shotdata(pairs: 'Iterable[tuple[SV3InterrogationData, SV3ReplyData]]', logger: 'logging.Logger', rules: 'SV3PairingRules' = SV3PairingRules(max_roundtrip_seconds=15.0, min_range_metres=0.001, return_time_tolerance_seconds=1e-06)) -> "'DataFrame[GARPOSShotDataFrame] | None'"`

Validate pairs, assemble a DataFrame, and run Pandera schema validation.

The single place that owns DataFrame construction, ``isUpdated`` injection,
and schema enforcement.  Both DFOP00 and QC-JSON callers delegate here.

Args:
    pairs: Iterable of (interrogation, reply) internal-type pairs.
    logger: For rejection logging.
    rules: Validation thresholds.

Returns:
    Validated GARPOSShotDataFrame, or None if no pairs survive validation.

## `dfop00_to_sfgdstf_seafloor_acoustic_data(source: 'str | Path', site_data: 'SFGDTSFSite', logger: 'logging.Logger') -> 'SFGDSTFSeafloorAcousticData | None'`

Convert a DFOP00 log file to the SFG DSTF seafloor acoustic data format.

Calls :func:`dfop00_to_shotdata` to obtain a raw shot DataFrame, then applies
the site ATD (Antenna-to-Transducer) offset to both transmit and receive ECEF
positions before assembling the standardised :class:`SFGDSTFSeafloorAcousticData`
DataFrame with MT IDs, travel times, timestamps, corrected positions, attitude
angles, acoustic diagnostics, and position uncertainties.

Args:
    source: Path to the DFOP00 JSONL file.
    site_data: Site metadata including the 3-element ATD offset vector
        ``[dEast, dNorth, dUp]`` in metres.
    logger: Logger instance used to report conversion failures.

Returns:
    An :class:`SFGDSTFSeafloorAcousticData` instance, or ``None`` if
    shot-data extraction fails.

## `dfop00_to_shotdata(source: 'str | Path', logger: 'logging.Logger') -> "'DataFrame[GARPOSShotDataFrame] | None'"`

Parse a DFOP00 JSONL log file into a validated shot-data DataFrame.

Thin I/O wrapper around :func:`parse_dfop00_lines`.

Args:
    source: Path to the DFOP00 JSONL file.
    logger: Logger instance for I/O errors and empty-result warnings.

Returns:
    A validated :class:`GARPOSShotDataFrame`, or ``None`` on read error or
    no valid pairs.

## `merge_interrogation_reply(interrogation: 'SV3InterrogationData', reply: 'SV3ReplyData') -> 'dict | None'`

Validate and merge a matched interrogation/reply pair into a single dict.

Performs three sanity checks:

1. The reconstructed two-way range is non-zero (``> 1 mm``).
2. The time difference between ping and return is ``<= 15`` seconds.
3. The independently calculated return time matches the logged return time
   to within ``1 µs``.

Args:
    interrogation: GARPOS-formatted interrogation data for the outgoing ping.
    reply: GARPOS-formatted reply data for the incoming acoustic return.

Returns:
    A merged dictionary combining both dataclass instances, or ``None`` if
    any assertion fails (the caller is expected to catch :class:`AssertionError`).

Raises:
    AssertionError: If any of the range or timing sanity checks fail.

## `novatel_interrogation_to_garpos_interrogation(novatel_interrogation: 'NovatelInterrogationEvent') -> 'SV3InterrogationData'`

Convert a Novatel interrogation event to a GARPOS-compatible interrogation record.

Transforms geodetic GNSS coordinates to ECEF, applies the GPS leap-second
offset to the ping timestamp, and packages attitude and position uncertainty
into an :class:`SV3InterrogationData` object.

Args:
    novatel_interrogation: Parsed Novatel interrogation event containing GNSS
        position, AHRS attitude, and a common timestamp.

Returns:
    An :class:`SV3InterrogationData` instance with ECEF position, attitude,
    position standard deviations, and ping time (GPS time).

## `novatel_reply_to_garpos_reply(novatel_reply: 'NovatelRangeEvent') -> 'SV3ReplyData'`

Convert a Novatel range event to a GARPOS-compatible reply record.

Transforms geodetic GNSS coordinates to ECEF, computes the one-way acoustic
travel time by subtracting the transponder turnaround time (TAT) and the
hardware trigger delay from the raw range, and packages all fields into an
:class:`SV3ReplyData` object.

Args:
    novatel_reply: Parsed Novatel range event containing GNSS position, AHRS
        attitude, acoustic range diagnostics, and a common timestamp.

Returns:
    An :class:`SV3ReplyData` instance with ECEF position, attitude, acoustic
    diagnostics, TAT, one-way travel time, and return time (GPS time).

## `pair_events(events: 'Iterable[NovatelInterrogationEvent | NovatelRangeEvent]') -> 'Iterator[tuple[SV3InterrogationData, SV3ReplyData]]'`

Convert events to internal types and yield matched (interrogation, reply) pairs.

Stateful: each range event is paired with the most recent preceding
interrogation.  Range events without a preceding interrogation are skipped.

## `parse_dfop00_lines(lines: 'list[str]', logger: 'logging.Logger', rules: 'SV3PairingRules' = SV3PairingRules(max_roundtrip_seconds=15.0, min_range_metres=0.001, return_time_tolerance_seconds=1e-06)) -> "'DataFrame[GARPOSShotDataFrame] | None'"`

Parse DFOP00 JSONL lines into a validated shot-data DataFrame.

Pure function: no filesystem access.  Pass ``f.readlines()`` output or
any list of JSON strings.

Args:
    lines: Raw text lines from a DFOP00 JSONL file.
    logger: For rejection and empty-result messages.
    rules: Validation thresholds; defaults reproduce original behaviour.

Returns:
    Validated GARPOSShotDataFrame, or None if no valid pairs exist.

## `parse_jsonl_lines(lines: 'Iterable[str]') -> 'list[NovatelInterrogationEvent | NovatelRangeEvent]'`

Parse raw JSONL strings into typed event objects.

Skips lines that fail JSON decoding or Pydantic validation.
