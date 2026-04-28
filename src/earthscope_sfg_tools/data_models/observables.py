"""Pandera dataframe schemas used by migrated parsing workflows."""

from __future__ import annotations

import pandas as pd
import pandera.pandas as pa
from pandera.typing import Series

from .constants import GNSS_START_TIME, LEAP_SECONDS


class AcousticDataFrame(pa.DataFrameModel):
    transponderID: Series[str] = pa.Field(description="Unique identifier", coerce=True)
    pingTime: Series[float] = pa.Field(
        ge=GNSS_START_TIME.timestamp() - LEAP_SECONDS,
        coerce=True,
        description="Ping send time in GPS time [s]",
    )
    returnTime: Series[float] = pa.Field(
        ge=GNSS_START_TIME.timestamp() - LEAP_SECONDS,
        coerce=True,
        description="Return time in GPS time [s]",
    )
    tt: Series[float] = pa.Field(ge=0.0, le=600, coerce=True)
    dbv: Series[int] = pa.Field(coerce=True)
    xc: Series[int] = pa.Field(ge=0, le=100, coerce=True)
    snr: Series[float] = pa.Field(ge=-100, le=100.0, coerce=True, default=0, nullable=True)
    tat: Series[float] = pa.Field(ge=0, le=10, coerce=True, default=0, nullable=True)

    class Config:
        coerce = True
        add_missing_columns = True


class ShotDataFrame(AcousticDataFrame):
    head0: Series[float]
    pitch0: Series[float]
    roll0: Series[float]
    east0: Series[float]
    north0: Series[float]
    up0: Series[float]
    head1: Series[float]
    pitch1: Series[float]
    roll1: Series[float]
    east1: Series[float]
    north1: Series[float]
    up1: Series[float]
    east_std0: Series[float] | None = pa.Field(nullable=True)
    north_std0: Series[float] | None = pa.Field(nullable=True)
    up_std0: Series[float] | None = pa.Field(nullable=True)
    east_std1: Series[float] | None = pa.Field(nullable=True)
    north_std1: Series[float] | None = pa.Field(nullable=True)
    up_std1: Series[float] | None = pa.Field(nullable=True)

    class Config:
        add_missing_columns = True
        coerce = True
        drop_invalid_rows = True


class SoundVelocityDataFrame(pa.DataFrameModel):
    depth: Series[float] = pa.Field(ge=0, le=10000, coerce=True)
    speed: Series[float] = pa.Field(unique=True, ge=0, le=3800, coerce=True)

    class Config:
        coerce = True
        drop_invalid_rows = True
