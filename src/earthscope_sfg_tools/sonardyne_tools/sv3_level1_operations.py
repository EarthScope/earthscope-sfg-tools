"""
Level 1 parsers for Sonardyne SV3 event logs (DFOP00 JSONL and QCPIN JSON).

Each SV3 event carries several observation blocks, so one source file feeds
three Level 1 tables: travel times (range + interrogation events), GNSS
antenna positions (``GNSS`` block) and attitude (``AHRS`` block).

Unlike :mod:`.sv3_operations`, these parsers keep the raw two-way range
and do not require a ``NOV_INS`` block: travel-time correction and QC
belong to Level 2. Each ``*_to_*`` function returns a ``pyarrow.Table``
matching the Iceberg table definition in
:mod:`earthscope_sfg_tools.datamodels.iceberg.schemas`.
"""

import json
import logging
import math
from collections.abc import Iterable
from datetime import UTC, datetime, timedelta
from decimal import Decimal, localcontext
from pathlib import Path
from typing import Any

import pyarrow as pa
import pymap3d as pm

from ..datamodels.iceberg.schemas import (
    ATTITUDE_AHRS,
    GNSS_ANTENNA_POSITIONS,
    TRAVEL_TIMES,
    Scope,
)
from ..datamodels.observationdata.parsing.sv3_models import (
    SonardyneInterrogationEvent,
    SonardyneRangeEvent,
)
from .sv3_operations import parse_jsonl_lines

logger = logging.getLogger(__name__)

SV3Event = SonardyneInterrogationEvent | SonardyneRangeEvent

_UNIX_EPOCH = datetime(1970, 1, 1, tzinfo=UTC)


def unix_to_utc(seconds: Decimal | float) -> datetime:
    """Convert SV3 ``time.common`` UNIX seconds to an aware UTC datetime (µs).

    Uses a local Decimal context because ``datamodels.observationdata.constants``
    sets the global precision to 10 digits, which would round a UNIX time to
    whole seconds.
    """
    with localcontext() as ctx:
        ctx.prec = 28
        micros = int(Decimal(str(seconds)) * 1_000_000)
    return _UNIX_EPOCH + timedelta(microseconds=micros)


# ---------------------------------------------------------------------------
# Reading events
# ---------------------------------------------------------------------------


def read_dfop00_events(source: str | Path) -> list[SV3Event]:
    """Read a DFOP00 JSONL file into SV3 events, skipping invalid lines."""
    with open(source, encoding="utf-8") as f:
        return parse_jsonl_lines(f)


def read_qcpin_events(source: str | Path) -> list[SV3Event]:
    """Read a QCPIN JSON file into SV3 events: its interrogation, then its ranges.

    Blocks that fail model validation are skipped. Falls back to latin-1 when
    the file is not valid UTF-8.
    """
    try:
        with open(source, encoding="utf-8") as f:
            raw = json.load(f)
    except UnicodeDecodeError:
        with open(source, encoding="latin-1") as f:
            raw = json.load(f)

    events: list[SV3Event] = []
    for key, value in raw.items():
        if not isinstance(value, dict):
            continue
        try:
            if key == "interrogation":
                events.insert(0, SonardyneInterrogationEvent(**value))
            elif value.get("event") == "range":
                events.append(SonardyneRangeEvent(**value))
        except Exception as e:
            logger.debug(f"Skipping invalid QCPIN block {key!r} in {source}: {e}")
    return events


# ---------------------------------------------------------------------------
# Events -> rows
# ---------------------------------------------------------------------------


def travel_time_rows(events: Iterable[SV3Event]) -> list[dict[str, Any]]:
    """Pair each range event with the most recent preceding interrogation.

    Range events with no preceding interrogation are dropped.
    """
    rows: list[dict[str, Any]] = []
    interrogation: SonardyneInterrogationEvent | None = None
    for event in events:
        if isinstance(event, SonardyneInterrogationEvent):
            interrogation = event
            continue
        if interrogation is None:
            continue
        reply = event.range
        rows.append(
            {
                "MT_ID": reply.cn,
                "T_transmit": unix_to_utc(interrogation.time.common),
                "T_receive": unix_to_utc(event.time.common),
                "TravelTime": float(reply.range),
                "dbV": float(reply.diag.dbv),
                "acc": int(reply.diag.xc),
                "aSNR": float(reply.diag.snr),
                "tat": float(reply.tat),  # already seconds (model converts from ms)
            }
        )
    return rows


def _enu_sigmas_to_ecef(
    lat_deg: float, lon_deg: float, sig_e: float, sig_n: float, sig_u: float
) -> tuple[float, float, float]:
    """Rotate diagonal ENU std devs to ECEF std devs (correlations dropped)."""
    lat, lon = math.radians(lat_deg), math.radians(lon_deg)
    sl, cl, so, co = math.sin(lat), math.cos(lat), math.sin(lon), math.cos(lon)
    # Rows: ECEF X/Y/Z; columns: E/N/U unit vectors.
    rotation = (
        (-so, -sl * co, cl * co),
        (co, -sl * so, cl * so),
        (0.0, cl, sl),
    )
    variances = (sig_e**2, sig_n**2, sig_u**2)
    return tuple(
        math.sqrt(sum(r**2 * v for r, v in zip(row, variances))) for row in rotation
    )


def antenna_position_rows(events: Iterable[SV3Event]) -> list[dict[str, Any]]:
    """Extract one row per distinct GNSS fix time from the events' GNSS blocks."""
    rows: dict[Any, dict[str, Any]] = {}
    for event in events:
        gnss = event.observations.GNSS
        if gnss is None or gnss.time.common in rows:
            continue
        lat, lon, hae = float(gnss.latitude), float(gnss.longitude), float(gnss.hae)
        x, y, z = pm.geodetic2ecef(lat, lon, hae)
        if None in (gnss.sdx, gnss.sdy, gnss.sdz):
            sigmas = (None, None, None)
        else:
            sigmas = _enu_sigmas_to_ecef(
                lat, lon, float(gnss.sdx), float(gnss.sdy), float(gnss.sdz)
            )
        rows[gnss.time.common] = {
            "time": unix_to_utc(gnss.time.common),
            "ant_x": x,
            "ant_y": y,
            "ant_z": z,
            "ant_sigx": sigmas[0],
            "ant_sigy": sigmas[1],
            "ant_sigz": sigmas[2],
            "q": gnss.q.value,
        }
    return list(rows.values())


def _float_or_none(value: Any) -> float | None:
    return None if value is None else float(value)


def ahrs_rows(events: Iterable[SV3Event]) -> list[dict[str, Any]]:
    """Extract one row per distinct AHRS time from the events' AHRS blocks."""
    rows: dict[Any, dict[str, Any]] = {}
    for event in events:
        ahrs = event.observations.AHRS
        if ahrs is None or ahrs.time.common in rows:
            continue
        rows[ahrs.time.common] = {
            "time": unix_to_utc(ahrs.time.common),
            "time_instrument": float(ahrs.time.instrument),
            "time_start_count": ahrs.time.start_count,
            "time_status": ahrs.time.status,
            **{
                k: _float_or_none(getattr(ahrs, k))
                for k in ("acx", "acy", "acz", "h", "h_mag", "p", "r")
            },
        }
    return list(rows.values())


# ---------------------------------------------------------------------------
# File -> Arrow
# ---------------------------------------------------------------------------


def dfop00_to_travel_times(source: str | Path, scope: Scope) -> pa.Table:
    return TRAVEL_TIMES.to_arrow(
        travel_time_rows(read_dfop00_events(source)), scope, "DFOP00"
    )


def qcpin_to_travel_times(source: str | Path, scope: Scope) -> pa.Table:
    return TRAVEL_TIMES.to_arrow(
        travel_time_rows(read_qcpin_events(source)), scope, "QCPIN"
    )


def dfop00_to_gnss_antenna_positions(source: str | Path, scope: Scope) -> pa.Table:
    return GNSS_ANTENNA_POSITIONS.to_arrow(
        antenna_position_rows(read_dfop00_events(source)),
        scope,
        "DFOP00",
    )


def qcpin_to_gnss_antenna_positions(source: str | Path, scope: Scope) -> pa.Table:
    return GNSS_ANTENNA_POSITIONS.to_arrow(
        antenna_position_rows(read_qcpin_events(source)),
        scope,
        "QCPIN",
    )


def dfop00_to_attitude_ahrs(source: str | Path, scope: Scope) -> pa.Table:
    return ATTITUDE_AHRS.to_arrow(
        ahrs_rows(read_dfop00_events(source)), scope, "DFOP00"
    )


def qcpin_to_attitude_ahrs(source: str | Path, scope: Scope) -> pa.Table:
    return ATTITUDE_AHRS.to_arrow(ahrs_rows(read_qcpin_events(source)), scope, "QCPIN")
