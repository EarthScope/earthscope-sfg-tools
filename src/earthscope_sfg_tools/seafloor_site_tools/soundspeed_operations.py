"""Functions for processing sound velocity profile (SVP) data."""

from __future__ import annotations

import re
from pathlib import Path

import numpy as np
import pandas as pd
import pandera.pandas as pa
from pandera.typing import DataFrame

from ..data_models.observables import SoundVelocityDataFrame
from ..logging import get_logger


@pa.check_types(lazy=True)
def seabird_to_soundvelocity(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    with open(source) as f:
        lines = f.readlines()
        data = []
        data_start = re.compile(r"\*END\*")
        while lines:
            line = lines.pop(0)
            if data_start.match(line):
                break

        if not lines:
            get_logger().logerr(f"No data found in the sound speed profile file {source}")
            return None

        for line in lines:
            values = line.split()
            data.append({"depth": float(values[0]), "speed": float(values[5])})

    df = pd.DataFrame(data)
    get_logger().loginfo(
        f"Found SS data down to max depth of {df['depth'].max()} m\n"
        f"SS ranges from {df['speed'].min()} to {df['speed'].max()} m/s"
    )
    return SoundVelocityDataFrame(df, lazy=True)


@pa.check_types(lazy=True)
def ctd_to_svp_v1(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    df = pd.read_csv(source, names=["depth", "speed"])
    df["depth"] = -df["depth"]
    df = interpolate_svp(df, additional_depth=200.0)
    return SoundVelocityDataFrame(df, lazy=True)


@pa.check_types(lazy=True)
def ctd_to_svp_v2(source: str | Path) -> DataFrame[SoundVelocityDataFrame]:
    df = pd.read_csv(source, sep=r"\s+", names=["depth", "speed"])
    df["depth"] = -df["depth"]
    df["speed"] = df["speed"] + np.random.randn(len(df)) * 1e-6
    df = interpolate_svp(df, additional_depth=200.0)
    return SoundVelocityDataFrame(df, lazy=True)


def interpolate_svp(svp: pd.DataFrame, additional_depth: float = 200.0) -> pd.DataFrame:
    max_depth = svp["depth"].max()
    new_depths = np.arange(max_depth + 1, max_depth + additional_depth)
    new_speeds = np.interp(new_depths, svp["depth"].to_numpy(), svp["speed"].to_numpy())
    new_svp = pd.DataFrame({"depth": new_depths, "speed": new_speeds})
    svp = pd.concat([svp, new_svp], ignore_index=True)
    get_logger().loginfo(f"Extended SVP to {additional_depth} m depth with interpolation.")
    return svp
