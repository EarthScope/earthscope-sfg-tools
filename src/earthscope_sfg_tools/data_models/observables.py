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


class KinPositionDataFrame(pa.DataFrameModel):
    """Kinematic GNSS position solution stored in TileDB KinPosition arrays.

    Columns match KinPositionArraySchema; time is the sparse dimension index.
    """

    time: Series[pd.DatetimeTZDtype] = pa.Field(
        dtype_kwargs={"unit": "ms", "tz": "UTC"},
        description="Observation timestamp [datetime64[ms, UTC]]",
    )
    latitude: Series[float] = pa.Field(ge=-90.0, le=90.0, description="Latitude [deg]")
    longitude: Series[float] = pa.Field(ge=-180.0, le=180.0, description="Longitude [deg]")
    height: Series[float] = pa.Field(description="Ellipsoidal height [m]")
    east: Series[float] = pa.Field(description="East displacement [m]")
    north: Series[float] = pa.Field(description="North displacement [m]")
    up: Series[float] = pa.Field(description="Up displacement [m]")
    number_of_satellites: Series[int] = pa.Field(ge=0, description="Number of tracked satellites")
    pdop: Series[float] = pa.Field(ge=0.0, description="Position dilution of precision")
    wrms: Series[float] = pa.Field(ge=0.0, description="Weighted RMS of position residuals [m]")

    class Config:
        coerce = True
        strict = False


class IMUPositionDataFrame(pa.DataFrameModel):
    """IMU/INS position and orientation solution stored in TileDB IMUPosition arrays.

    Columns match IMUPositionArraySchema; time is the sparse dimension index.
    Std-dev fields are nullable because they may not be populated by all receivers.
    """

    time: Series[pd.DatetimeTZDtype] = pa.Field(
        dtype_kwargs={"unit": "ms", "tz": "UTC"},
        description="Observation timestamp [datetime64[ms, UTC]]",
    )
    azimuth: Series[float] = pa.Field(ge=0.0, le=360.0, description="Heading / azimuth [deg]")
    pitch: Series[float] = pa.Field(ge=-90.0, le=90.0, description="Pitch angle [deg]")
    roll: Series[float] = pa.Field(ge=-180.0, le=180.0, description="Roll angle [deg]")
    latitude: Series[float] = pa.Field(ge=-90.0, le=90.0, description="Latitude [deg]")
    longitude: Series[float] = pa.Field(ge=-180.0, le=180.0, description="Longitude [deg]")
    height: Series[float] = pa.Field(description="Ellipsoidal height [m]")
    northVelocity: Series[float] = pa.Field(description="North velocity [m/s]")
    eastVelocity: Series[float] = pa.Field(description="East velocity [m/s]")
    upVelocity: Series[float] = pa.Field(description="Up velocity [m/s]")
    latitude_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Latitude std dev [deg]")
    longitude_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Longitude std dev [deg]")
    height_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Height std dev [m]")
    northVelocity_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="North velocity std dev [m/s]")
    eastVelocity_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="East velocity std dev [m/s]")
    upVelocity_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Up velocity std dev [m/s]")
    roll_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Roll std dev [deg]")
    pitch_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Pitch std dev [deg]")
    azimuth_std: Series[float] = pa.Field(ge=0.0, nullable=True, description="Azimuth std dev [deg]")

    class Config:
        coerce = True
        strict = False
