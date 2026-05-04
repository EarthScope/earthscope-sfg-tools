from __future__ import annotations

import json
from collections.abc import Iterable, Iterator
from dataclasses import dataclass
from datetime import UTC, datetime
from enum import Enum, auto
from pathlib import Path

import pandas as pd
import pymap3d as pm
from pandera.typing import DataFrame
import logging

from ..datamodels.observationdata.community.community_standards import SFGDSTFSeafloorAcousticData
from ..datamodels.metadata.community.site import SFGDTSFSite
from ..datamodels.observationdata.constants import LEAP_SECONDS, TRIGGER_DELAY_SV3
from ..datamodels.observationdata.parsing.log_models import SV3InterrogationData, SV3ReplyData
from ..datamodels.observationdata.garpos.observables import GARPOSShotDataFrame
from ..datamodels.observationdata.parsing.sv3_models import NovatelInterrogationEvent, NovatelRangeEvent

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# C3: Pairing rules and pipeline stages
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class SV3PairingRules:
    """Thresholds governing interrogation/reply pair validation.

    Defaults reproduce the original hardcoded assertion values.
    """
    max_roundtrip_seconds: float = 15.0
    min_range_metres: float = 1e-3
    return_time_tolerance_seconds: float = 1e-6


class RejectionReason(Enum):
    ZERO_RANGE = auto()
    ROUNDTRIP_TOO_LONG = auto()
    RETURN_TIME_MISMATCH = auto()


@dataclass
class PairResult:
    """Outcome of pairing one interrogation with one reply."""
    data: dict | None
    rejection: RejectionReason | None = None


def _validate_pair(
    interrogation: SV3InterrogationData,
    reply: SV3ReplyData,
    rules: SV3PairingRules,
) -> PairResult:
    """Validate one interrogation/reply pair against pairing rules.

    Returns a PairResult — never raises.
    """
    rng = float(reply.tt) + float(reply.tat) + TRIGGER_DELAY_SV3
    if abs(rng) <= rules.min_range_metres:
        return PairResult(data=None, rejection=RejectionReason.ZERO_RANGE)

    time_diff = abs(float(reply.returnTime) - float(interrogation.pingTime))
    if time_diff > rules.max_roundtrip_seconds:
        return PairResult(data=None, rejection=RejectionReason.ROUNDTRIP_TOO_LONG)

    range_original = float(reply.tt) + float(reply.tat)
    calc_return = float(interrogation.pingTime) + range_original
    if abs(calc_return - float(reply.returnTime)) >= rules.return_time_tolerance_seconds:
        return PairResult(data=None, rejection=RejectionReason.RETURN_TIME_MISMATCH)

    return PairResult(data=dict(interrogation) | dict(reply))


def parse_jsonl_lines(
    lines: Iterable[str],
) -> list[NovatelInterrogationEvent | NovatelRangeEvent]:
    """Parse raw JSONL strings into typed event objects.

    Skips lines that fail JSON decoding or Pydantic validation.
    """
    events: list[NovatelInterrogationEvent | NovatelRangeEvent] = []
    for line in lines:
        try:
            data = json.loads(line)
        except json.JSONDecodeError:
            continue
        event_type = data.get("event")
        try:
            if event_type == "interrogation":
                events.append(NovatelInterrogationEvent(**data))
            elif event_type == "range":
                events.append(NovatelRangeEvent(**data))
        except Exception:
            continue
    return events


def pair_events(
    events: Iterable[NovatelInterrogationEvent | NovatelRangeEvent],
) -> Iterator[tuple[SV3InterrogationData, SV3ReplyData]]:
    """Convert events to internal types and yield matched (interrogation, reply) pairs.

    Stateful: each range event is paired with the most recent preceding
    interrogation.  Range events without a preceding interrogation are skipped.
    """
    current_interro: SV3InterrogationData | None = None
    for event in events:
        if isinstance(event, NovatelInterrogationEvent):
            try:
                current_interro = novatel_interrogation_to_garpos_interrogation(event)
            except Exception:
                current_interro = None
        elif isinstance(event, NovatelRangeEvent) and current_interro is not None:
            try:
                reply = novatel_reply_to_garpos_reply(event)
                yield current_interro, reply
            except Exception:
                continue


def build_shotdata(
    pairs: Iterable[tuple[SV3InterrogationData, SV3ReplyData]],
    logger: logging.Logger,
    rules: SV3PairingRules = SV3PairingRules(),
) -> "DataFrame[GARPOSShotDataFrame] | None":
    """Validate pairs, assemble a DataFrame, and run Pandera schema validation.

    The single place that owns DataFrame construction, ``isUpdated`` injection,
    and schema enforcement.  Both DFOP00 and QC-JSON callers delegate here.

    Args:
        pairs: Iterable of (interrogation, reply) internal-type pairs.
        logger: For rejection logging.
        rules: Validation thresholds.

    Returns:
        Validated GARPOSShotDataFrame, or None if no pairs survive validation.
    """
    processed: list[dict] = []
    for interrogation, reply in pairs:
        result = _validate_pair(interrogation, reply, rules)
        if result.data is not None:
            processed.append(result.data)
        else:
            logger.debug("Rejected pair [%s] for transponder %s", result.rejection, reply.transponderID)

    if not processed:
        logger.error("No valid pairs found")
        return None

    df = pd.DataFrame(processed)
    df["isUpdated"] = False
    return GARPOSShotDataFrame.validate(df, lazy=True)


def novatel_interrogation_to_garpos_interrogation(
    novatel_interrogation: NovatelInterrogationEvent,
) -> SV3InterrogationData:
    """Convert a Novatel interrogation event to a GARPOS-compatible interrogation record.

    Transforms geodetic GNSS coordinates to ECEF, applies the GPS leap-second
    offset to the ping timestamp, and packages attitude and position uncertainty
    into an :class:`SV3InterrogationData` object.

    Args:
        novatel_interrogation: Parsed Novatel interrogation event containing GNSS
            position, AHRS attitude, and a common timestamp.

    Returns:
        An :class:`SV3InterrogationData` instance with ECEF position, attitude,
        position standard deviations, and ping time (GPS time).
    """
    east_ecef, north_ecef, up_ecef = pm.geodetic2ecef(
        float(novatel_interrogation.observations.GNSS.latitude),
        float(novatel_interrogation.observations.GNSS.longitude),
        float(novatel_interrogation.observations.GNSS.hae),
    )
    return SV3InterrogationData(
        head0=novatel_interrogation.observations.AHRS.h,
        pitch0=novatel_interrogation.observations.AHRS.p,
        roll0=novatel_interrogation.observations.AHRS.r,
        east0=east_ecef,
        north0=north_ecef,
        up0=up_ecef,
        east_std0=novatel_interrogation.observations.GNSS.sdx,
        north_std0=novatel_interrogation.observations.GNSS.sdy,
        up_std0=novatel_interrogation.observations.GNSS.sdz,
        pingTime=float(novatel_interrogation.time.common) + LEAP_SECONDS,
    )


def novatel_reply_to_garpos_reply(novatel_reply: NovatelRangeEvent) -> SV3ReplyData:
    """Convert a Novatel range event to a GARPOS-compatible reply record.

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
    """
    east_ecef, north_ecef, up_ecef = pm.geodetic2ecef(
        float(novatel_reply.observations.GNSS.latitude),
        float(novatel_reply.observations.GNSS.longitude),
        float(novatel_reply.observations.GNSS.hae),
    )
    travel_time = float(novatel_reply.range.range) - float(novatel_reply.range.tat) - TRIGGER_DELAY_SV3
    return SV3ReplyData(
        transponderID=novatel_reply.range.cn,
        head1=novatel_reply.observations.AHRS.h,
        pitch1=novatel_reply.observations.AHRS.p,
        roll1=novatel_reply.observations.AHRS.r,
        east1=east_ecef,
        north1=north_ecef,
        up1=up_ecef,
        east_std1=novatel_reply.observations.GNSS.sdx,
        north_std1=novatel_reply.observations.GNSS.sdy,
        up_std1=novatel_reply.observations.GNSS.sdz,
        tat=novatel_reply.range.tat,
        snr=novatel_reply.range.diag.snr,
        dbv=novatel_reply.range.diag.dbv,
        xc=novatel_reply.range.diag.xc,
        returnTime=float(novatel_reply.time.common) + LEAP_SECONDS,
        tt=travel_time,
    )


def merge_interrogation_reply(
    interrogation: SV3InterrogationData,
    reply: SV3ReplyData,
) -> dict | None:
    """Validate and merge a matched interrogation/reply pair into a single dict.

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
    """
    rng = float(reply.tt) + float(reply.tat) + TRIGGER_DELAY_SV3
    assert abs(rng) > 1e-3, (
        f"Transponder {reply.transponderID} has range={abs(round(rng, 1))} "
        f"for ping at {interrogation.pingTime} {datetime.fromtimestamp(float(interrogation.pingTime), tz=UTC)}"
    )
    time_difference = abs(float(reply.returnTime) - float(interrogation.pingTime))
    assert time_difference <= 15, (
        "Calculated time difference between ping and return is too large: "
        f"{time_difference} seconds for transponder {reply.transponderID}"
    )

    range_original = float(reply.tt) + float(reply.tat)
    calc_return_time = float(interrogation.pingTime) + range_original
    assert abs(calc_return_time - float(reply.returnTime)) < 1e-6, (
        f"Calculated return time {calc_return_time} does not match reply return time {reply.returnTime}"
    )
    return dict(interrogation) | dict(reply)


def parse_dfop00_lines(
    lines: list[str],
    logger: logging.Logger,
    rules: SV3PairingRules = SV3PairingRules(),
) -> "DataFrame[GARPOSShotDataFrame] | None":
    """Parse DFOP00 JSONL lines into a validated shot-data DataFrame.

    Pure function: no filesystem access.  Pass ``f.readlines()`` output or
    any list of JSON strings.

    Args:
        lines: Raw text lines from a DFOP00 JSONL file.
        logger: For rejection and empty-result messages.
        rules: Validation thresholds; defaults reproduce original behaviour.

    Returns:
        Validated GARPOSShotDataFrame, or None if no valid pairs exist.
    """
    return build_shotdata(pair_events(parse_jsonl_lines(lines)), logger, rules)


def dfop00_to_shotdata(source: str | Path, logger: logging.Logger) -> "DataFrame[GARPOSShotDataFrame] | None":
    """Parse a DFOP00 JSONL log file into a validated shot-data DataFrame.

    Thin I/O wrapper around :func:`parse_dfop00_lines`.

    Args:
        source: Path to the DFOP00 JSONL file.
        logger: Logger instance for I/O errors and empty-result warnings.

    Returns:
        A validated :class:`GARPOSShotDataFrame`, or ``None`` on read error or
        no valid pairs.
    """
    try:
        with open(source, encoding="utf-8") as f:
            lines = f.readlines()
    except (FileNotFoundError, PermissionError, UnicodeDecodeError) as e:
        logger.error(f"Error reading {source}: {e}")
        return None
    return parse_dfop00_lines(lines, logger)


def dfop00_to_sfgdstf_seafloor_acoustic_data(
    source: str | Path,
    site_data: SFGDTSFSite,
    logger: logging.Logger,
) -> SFGDSTFSeafloorAcousticData | None:
    """Convert a DFOP00 log file to the SFG DSTF seafloor acoustic data format.

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
    """
    shotdata = dfop00_to_shotdata(source, logger)
    if shotdata is None:
        logger.error(f"Failed to convert {source} to ShotDataFrame")
        return None

    x_transmit = shotdata.east0.apply(lambda x: x + site_data.ATDoffset[0])
    y_transmit = shotdata.north0.apply(lambda x: x + site_data.ATDoffset[1])
    z_transmit = shotdata.up0.apply(lambda x: x + site_data.ATDoffset[2])

    x_receive = shotdata.east1.apply(lambda x: x + site_data.ATDoffset[0])
    y_receive = shotdata.north1.apply(lambda x: x + site_data.ATDoffset[1])
    z_receive = shotdata.up1.apply(lambda x: x + site_data.ATDoffset[2])

    df = pd.DataFrame(
        {
            "MT_ID": shotdata.transponderID,
            "TravelTime": shotdata.tt,
            "T_transmit": shotdata.pingTime,
            "X_transmit": x_transmit,
            "Y_transmit": y_transmit,
            "Z_transmit": z_transmit,
            "T_receive": shotdata.returnTime,
            "X_receive": x_receive,
            "Y_receive": y_receive,
            "Z_receive": z_receive,
            "roll0": shotdata.roll0,
            "pitch0": shotdata.pitch0,
            "heading0": shotdata.head0,
            "roll1": shotdata.roll1,
            "pitch1": shotdata.pitch1,
            "heading1": shotdata.head1,
            "ant_X0": shotdata.east0,
            "ant_Y0": shotdata.north0,
            "ant_Z0": shotdata.up0,
            "ant_X1": shotdata.east1,
            "ant_Y1": shotdata.north1,
            "ant_Z1": shotdata.up1,
            "aSNR": shotdata.snr,
            "dbV": shotdata.dbv,
            "acc": shotdata.xc,
            "ant_sigX0": shotdata.east_std0,
            "ant_sigY0": shotdata.north_std0,
            "ant_sigZ0": shotdata.up_std0,
            "ant_sigX1": shotdata.east_std1,
            "ant_sigY1": shotdata.north_std1,
            "ant_sigZ1": shotdata.up_std1,
        }
    )

    return SFGDSTFSeafloorAcousticData(df)
