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


def parse_seabird_lines(
    lines: list[str],
    logger: logging.Logger = logger,
) -> "DataFrame[SoundVelocityDataFrame] | None":
    """Parse Seabird CTD SVP lines into a validated DataFrame.

    Pure function — no filesystem access. Pass the output of
    ``f.readlines()`` or any list of strings.

    Args:
        lines: Text lines from a Seabird SVP file, including the
            header section that ends with ``*END*``.
        logger: For info/error messages.

    Returns:
        Validated ``SoundVelocityDataFrame`` with ``depth`` and
        ``speed`` columns, or ``None`` if no data rows follow
        ``*END*``.
    """
    data = []
    data_start = re.compile(r"\*END\*")
    remaining = list(lines)

    while remaining:
        line = remaining.pop(0)
        if data_start.match(line):
            break

    if not remaining:
        logger.error("No data found in the sound speed profile lines")
        return None

    for line in remaining:
        values = line.split()
        data.append({"depth": float(values[0]), "speed": float(values[5])})

    df = pd.DataFrame(data)
    logger.info(
        f"Found SS data down to max depth of {df['depth'].max()} m\n"
        f"SS ranges from {df['speed'].min()} to {df['speed'].max()} m/s"
    )
    return SoundVelocityDataFrame(df, lazy=True)


@pa.check_types(lazy=True)
def seabird_to_soundvelocity(
    source: str | Path,
    logger: logging.Logger = logger,
) -> DataFrame[SoundVelocityDataFrame]:
    """Parse a Seabird CTD sound velocity profile file into a DataFrame.

    Thin I/O wrapper around :func:`parse_seabird_lines`.

    Args:
        source: Path to the Seabird SVP file.
        logger: Logger for info/error messages.

    Returns:
        Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
        ``speed`` (m/s) columns, or ``None`` if no data rows are
        found after ``*END*``.

    Example:
        >>> df = seabird_to_soundvelocity("cast_001.cnv")
        >>> df["depth"].max()
        500.0
    """
    with open(source) as f:
        lines = f.readlines()
    return parse_seabird_lines(lines, logger)


@pa.check_types(lazy=True)
def ctd_to_svp_v1(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    """Load a comma-separated CTD SVP file and extend it via interpolation.

    Negates depth values to convert from negative-down to positive-down
    convention, then calls :func:`interpolate_svp` to append 200 m of
    extrapolated speeds beyond the maximum measured depth.

    Args:
        source: Path to the comma-separated file with ``depth`` and
            ``speed`` columns (no header).

    Returns:
        Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
        ``speed`` (m/s) columns.
    """
    df = pd.read_csv(source, names=["depth", "speed"])
    df["depth"] = -df["depth"]
    df = interpolate_svp(df, additional_depth=200.0)
    return SoundVelocityDataFrame(df, lazy=True)


@pa.check_types(lazy=True)
def ctd_to_svp_v2(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    """Load a whitespace-separated CTD SVP file and extend it via interpolation.

    Negates depth values to convert from negative-down to positive-down
    convention. Adds a sub-micrometre random jitter to speed values to
    prevent exact duplicates, then calls :func:`interpolate_svp` to
    append 200 m of extrapolated speeds beyond the maximum measured depth.

    Args:
        source: Path to the whitespace-delimited file with ``depth``
            and ``speed`` columns (no header).

    Returns:
        Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
        ``speed`` (m/s) columns.
    """
    df = pd.read_csv(source, sep=r"\s+", names=["depth", "speed"])
    df["depth"] = -df["depth"]
    df["speed"] = df["speed"] + np.random.randn(len(df)) * 1e-6
    df = interpolate_svp(df, additional_depth=200.0)
    return SoundVelocityDataFrame(df, lazy=True)


def interpolate_svp(svp: pd.DataFrame, additional_depth: float = 200.0) -> pd.DataFrame:
    """Extend a sound velocity profile by linear extrapolation.

    Appends integer-metre depth rows from ``max_depth + 1`` to
    ``max_depth + additional_depth`` using :func:`numpy.interp`,
    allowing acoustic ray-tracing to continue past the deepest
    measured sample.

    Args:
        svp: DataFrame with ``depth`` (m) and ``speed`` (m/s) columns.
        additional_depth: Metres to append beyond the current maximum
            depth. Defaults to 200.0.

    Returns:
        Input DataFrame concatenated with the extrapolated rows,
        index reset to be contiguous.
    """
    max_depth = svp["depth"].max()
    new_depths = np.arange(max_depth + 1, max_depth + additional_depth)
    new_speeds = np.interp(new_depths, svp["depth"].to_numpy(), svp["speed"].to_numpy())
    new_svp = pd.DataFrame({"depth": new_depths, "speed": new_speeds})
    svp = pd.concat([svp, new_svp], ignore_index=True)
    logger.info(f"Extended SVP to {additional_depth} m depth with interpolation.")
    return svp
