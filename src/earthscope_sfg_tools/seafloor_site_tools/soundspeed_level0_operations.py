"""
Level 1 parsers for sound speed profiles (CTD casts and Sea-Bird files).

Each parser returns a ``pyarrow.Table`` matching the ``sound_speed`` Iceberg
table definition.

Neither file format carries a reliable cast time, so the caller supplies
``cast_time``; every row of a profile gets the same time.
"""

from datetime import datetime
from pathlib import Path

import pandas as pd
import pyarrow as pa

from ..datamodels.iceberg.schemas import SOUND_SPEED, Scope
from .soundspeed_operations import seabird_to_soundvelocity


def _profile_rows(df: pd.DataFrame, cast_time: datetime) -> list[dict]:
    if cast_time.tzinfo is None:
        raise ValueError("cast_time must be timezone-aware (UTC)")
    return [
        {"time": cast_time, "depth": float(d), "speed": float(s)}
        for d, s in zip(df["depth"], df["speed"])
    ]


def seabird_to_sound_speed(
    source: str | Path, scope: Scope, cast_time: datetime
) -> pa.Table:
    """Parse a Sea-Bird SVP file (data after ``*END*``) into the sound_speed table."""
    df = seabird_to_soundvelocity(source)
    rows = [] if df is None else _profile_rows(df, cast_time)
    return SOUND_SPEED.to_arrow(rows, scope, "SEABIRD")


def ctd_to_sound_speed(
    source: str | Path, scope: Scope, cast_time: datetime
) -> pa.Table:
    """Parse a headerless two-column CTD file (comma- or whitespace-separated).

    Depths are logged negative-down and flipped to positive-down. Unlike
    :func:`.soundspeed_operations.ctd_to_svp_v1`
    / ``_v2``, the profile is not extrapolated or jittered: Level 1 stores
    the cast as measured.
    """
    df = pd.read_csv(source, sep=r"[\s,]+", names=["depth", "speed"], engine="python")
    df["depth"] = -df["depth"]
    return SOUND_SPEED.to_arrow(_profile_rows(df, cast_time), scope, "CTD")
