"""Functions for processing sound velocity profile (SVP) data."""

from __future__ import annotations

import logging
import re
from pathlib import Path

import numpy as np
import pandas as pd
import pandera.pandas as pa
from pandera.typing import DataFrame

from ..datamodels.observationdata.soundvelocity import SoundVelocityDataFrame

logger = logging.getLogger(__name__)    

@pa.check_types(lazy=True)
def seabird_to_soundvelocity(source: str | Path,
                             logger: logging.Logger = logger) -> DataFrame[SoundVelocityDataFrame]:
    """Parse a Seabird CTD sound velocity profile file into a validated DataFrame.

    Reads lines from a Seabird-formatted file, skipping the header section that
    ends with ``*END*``, then extracts depth (column 0) and sound speed (column 5)
    from each data row.

    Args:
        source: Path to the Seabird SVP file.
        logger: Logger instance to use for info/error messages.

    Returns:
        A validated :class:`SoundVelocityDataFrame` with ``depth`` (m) and
        ``speed`` (m/s) columns, or ``None`` if no data rows are found.
    """
    with open(source) as f:
        lines = f.readlines()
        data = []
        data_start = re.compile(r"\*END\*")
        while lines:
            line = lines.pop(0)
            if data_start.match(line):
                break

        if not lines:
            logger.error(f"No data found in the sound speed profile file {source}")
            return None

        for line in lines:
            values = line.split()
            data.append({"depth": float(values[0]), "speed": float(values[5])})

    df = pd.DataFrame(data)
    logger.info(
        f"Found SS data down to max depth of {df['depth'].max()} m\n"
        f"SS ranges from {df['speed'].min()} to {df['speed'].max()} m/s"
    )
    return SoundVelocityDataFrame(df, lazy=True)


@pa.check_types(lazy=True)
def ctd_to_svp_v1(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    """Load a CTD sound velocity profile (v1 format) and extend it via interpolation.

    Reads a comma-separated file with ``depth`` and ``speed`` columns, negates
    depth values to convert from negative-down to positive-down convention, then
    calls :func:`interpolate_svp` to extend the profile by 200 m.

    Args:
        source: Path to the CSV file containing depth and speed columns.

    Returns:
        A validated :class:`SoundVelocityDataFrame` with ``depth`` (m) and
        ``speed`` (m/s) columns.
    """
    df = pd.read_csv(source, names=["depth", "speed"])
    df["depth"] = -df["depth"]
    df = interpolate_svp(df, additional_depth=200.0)
    return SoundVelocityDataFrame(df, lazy=True)


@pa.check_types(lazy=True)
def ctd_to_svp_v2(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    """Load a CTD sound velocity profile (v2 format) and extend it via interpolation.

    Reads a whitespace-separated file with ``depth`` and ``speed`` columns, negates
    depth values to convert from negative-down to positive-down convention, adds a
    tiny random jitter to speed values to break exact duplicates, then calls
    :func:`interpolate_svp` to extend the profile by 200 m.

    Args:
        source: Path to the whitespace-delimited file containing depth and speed columns.

    Returns:
        A validated :class:`SoundVelocityDataFrame` with ``depth`` (m) and
        ``speed`` (m/s) columns.
    """
    df = pd.read_csv(source, sep=r"\s+", names=["depth", "speed"])
    df["depth"] = -df["depth"]
    df["speed"] = df["speed"] + np.random.randn(len(df)) * 1e-6
    df = interpolate_svp(df, additional_depth=200.0)
    return SoundVelocityDataFrame(df, lazy=True)


def interpolate_svp(svp: pd.DataFrame, additional_depth: float = 200.0) -> pd.DataFrame:
    """Extend a sound velocity profile by linearly interpolating beyond its maximum depth.

    Appends rows from ``max_depth + 1`` to ``max_depth + additional_depth`` using
    :func:`numpy.interp` based on the existing profile, allowing acoustic ray-tracing
    to continue past the last measured depth.

    Args:
        svp: DataFrame with ``depth`` (m) and ``speed`` (m/s) columns.
        additional_depth: Number of metres to extend the profile beyond its current
            maximum depth. Defaults to 200.0 m.

    Returns:
        The input DataFrame concatenated with the interpolated extension rows,
        with the index reset.
    """
    max_depth = svp["depth"].max()
    new_depths = np.arange(max_depth + 1, max_depth + additional_depth)
    new_speeds = np.interp(new_depths, svp["depth"].to_numpy(), svp["speed"].to_numpy())
    new_svp = pd.DataFrame({"depth": new_depths, "speed": new_speeds})
    svp = pd.concat([svp, new_svp], ignore_index=True)
    logger.info(f"Extended SVP to {additional_depth} m depth with interpolation.")
    return svp
