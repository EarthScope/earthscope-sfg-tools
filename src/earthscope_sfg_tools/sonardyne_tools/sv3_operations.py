from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path

import pandas as pd
import pymap3d as pm
from pandera.typing import DataFrame
import logging

from ..data_models.community_standards import SFGDSTFSeafloorAcousticData, SFGDTSFSite
from ..data_models.constants import LEAP_SECONDS, TRIGGER_DELAY_SV3
from ..data_models.log_models import SV3InterrogationData, SV3ReplyData
from ..data_models.observables import ShotDataFrame
from ..data_models.sv3_models import NovatelInterrogationEvent, NovatelRangeEvent

logger = logging.getLogger(__name__)


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


def dfop00_to_shotdata(source: str | Path, logger: logging.Logger) -> DataFrame[ShotDataFrame] | None:
    """Parse a DFOP00 JSONL log file into a validated shot-data DataFrame.

    Reads each line of the file as a JSON object.  Lines with ``event =
    'interrogation'`` are parsed as :class:`NovatelInterrogationEvent` records;
    lines with ``event = 'range'`` are paired with the most recent interrogation
    and merged via :func:`merge_interrogation_reply`.  Successfully merged pairs
    are collected into a :class:`ShotDataFrame`.

    Args:
        source: Path to the DFOP00 JSONL file.
        logger: Logger instance used to report file I/O errors, parse failures,
            and empty-result warnings.

    Returns:
        A validated :class:`ShotDataFrame` with one row per successful ping/reply
        pair, or ``None`` if the file cannot be read or contains no valid pairs.
    """
    processed = []

    try:
        with open(source, encoding="utf-8") as f:
            lines = f.readlines()
    except (FileNotFoundError, PermissionError, UnicodeDecodeError) as e:
        logger.error(f"Error reading {source}: {e}")
        return None

    interrogation_parsed = None

    for line in lines:
        data = json.loads(line)
        if data.get("event") == "interrogation":
            try:
                interrogation = NovatelInterrogationEvent(**data)
                interrogation_parsed = novatel_interrogation_to_garpos_interrogation(interrogation)
            except Exception:
                interrogation_parsed = None

        if data.get("event") == "range":
            try:
                reply_data = NovatelRangeEvent(**data)
                reply_data_parsed = novatel_reply_to_garpos_reply(reply_data)
            except Exception:
                reply_data_parsed = None

            if reply_data_parsed is not None and interrogation_parsed is not None:
                try:
                    merged_data = merge_interrogation_reply(interrogation_parsed, reply_data_parsed)
                except AssertionError as e:
                    logger.error(f"Assertion error in merging ping/reply data: {e}")
                    merged_data = None

                if merged_data is not None:
                    processed.append(merged_data)

    if not processed:
        logger.error(f"No valid data found in {source}")
        return None

    df = pd.DataFrame(processed)
    df["isUpdated"] = False
    return ShotDataFrame.validate(df, lazy=True)


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
