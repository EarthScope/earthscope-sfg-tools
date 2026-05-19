"""Pandera DataFrame schemas for the GNSS-A Kalman filter position filter inputs.

Column names follow the gnatss constants convention:
  - time        : J2000 epoch seconds (TIME_J2000 / GPS_TIME = "time")
  - east/north/up : local ENU tangent frame (GPS_LOCAL_TANGENT)
  - ant_x/y/z   : ECEF antenna position (ANT_GPS_GEOCENTRIC)
  - ant_sigx/y/z: ECEF antenna position std dev (ANT_GPS_GEOCENTRIC_STD)
"""

from __future__ import annotations

import pandas as pd

from earthscope_sfg_tools.datamodels.observationdata.constants import GNSS_START_TIME
import pandera.pandas as pa
from pandera.typing import Series


class INSPVAASchema(pa.DataFrameModel):
    """Schema for the INSPVAA (NovAtel Level-1) DataFrame.

    Loaded via :func:`gnatss.loaders.load_novatel`.
    Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSPVA.htm
    """

    week: Series[int] = pa.Field(ge=0, description="GPS week number")
    seconds: Series[float] = pa.Field(ge=0.0, description="GPS seconds of week")
    lat: Series[float] = pa.Field(
        ge=-90.0, le=90.0, description="Geodetic latitude [deg.]"
    )
    lon: Series[float] = pa.Field(
        ge=-180.0, le=180.0, description="Geodetic longitude [deg.]"
    )
    alt: Series[float] = pa.Field(description="Geodetic altitude [m]")
    north: Series[float] = pa.Field(
        description="Velocity in local tangent North direction [m/s]"
    )
    east: Series[float] = pa.Field(
        description="Velocity in local tangent East direction [m/s]"
    )
    up: Series[float] = pa.Field(
        description="Velocity in local tangent Up direction [m/s]"
    )
    roll: Series[float] = pa.Field(ge=-180.0, le=180.0, description="Roll angle [deg.]")
    pitch: Series[float] = pa.Field(ge=-90.0, le=90.0, description="Pitch angle [deg.]")
    heading: Series[float] = pa.Field(
        ge=0.0, lt=360.0, description="Heading angle [deg.]"
    )
    time: Series[float] = pa.Field(
        description="J2000 time [sec since 2000-01-01 12:00:00]"
    )

    class Config:
        name = "INSPVAASchema"
        strict = False  # Allow extra columns added downstream
        coerce = False


class INSSTDEVSchema(pa.DataFrameModel):
    """Schema for the INSSTDEVA (NovAtel Level-1) DataFrame.

    Loaded via :func:`gnatss.loaders.load_novatel_std`.
    Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSSTDEV.htm
    """

    week: Series[int] = pa.Field(ge=0, description="GPS week number")
    seconds: Series[float] = pa.Field(ge=0.0, description="GPS seconds of week")
    lat_sig: Series[float] = pa.Field(
        ge=0.0, description="Latitude std deviation [deg.]"
    )
    lon_sig: Series[float] = pa.Field(
        ge=0.0, description="Longitude std deviation [deg.]"
    )
    alt_sig: Series[float] = pa.Field(ge=0.0, description="Altitude std deviation [m]")
    north_sig: Series[float] = pa.Field(
        ge=0.0, description="North velocity std deviation [m/s]"
    )
    east_sig: Series[float] = pa.Field(
        ge=0.0, description="East velocity std deviation [m/s]"
    )
    up_sig: Series[float] = pa.Field(
        ge=0.0, description="Up velocity std deviation [m/s]"
    )
    cov_rr: Series[float] = pa.Field(
        ge=0.0, description="Roll-Roll covariance diagonal [deg.^2]"
    )
    cov_pp: Series[float] = pa.Field(
        ge=0.0, description="Pitch-Pitch covariance diagonal [deg.^2]"
    )
    cov_hh: Series[float] = pa.Field(
        ge=0.0, description="Heading-Heading covariance diagonal [deg.^2]"
    )
    ext_sol_stat: Series[str] = pa.Field(description="Extended solution status")
    time_since_update: Series[str] = pa.Field(description="Time since last update")
    time: Series[float] = pa.Field(
        description="J2000 time [sec since 2000-01-01 12:00:00]"
    )

    class Config:
        name = "INSSTDEVSchema"
        strict = False
        coerce = False


class IMUPositionDataFrame(pa.DataFrameModel):
    """Combined IMU + GNSS position/velocity/attitude solution (community schema)."""

    time: Series[pd.Timestamp] = pa.Field(
        ge=GNSS_START_TIME.replace(tzinfo=None),
        coerce=True,
        description="Timestamp of the measurement in millisecond precision (UTC) [Y-M-D-H-M-S]",
    )
    azimuth: Series[float] = pa.Field(
        ge=-180,
        le=360,
        coerce=True,
        nullable=True,
        description="Heading/azimuth of the vessel at the time of the measurement [degrees]",
    )
    pitch: Series[float] = pa.Field(
        ge=-90,
        le=90,
        coerce=True,
        nullable=True,
        description="Pitch of the vessel at the time of the measurement [degrees]",
    )
    roll: Series[float] = pa.Field(
        ge=-180,
        le=180,
        coerce=True,
        nullable=True,
        description="Roll of the vessel at the time of the measurement [degrees]",
    )
    latitude: Series[float] = pa.Field(
        ge=-90,
        le=90,
        coerce=True,
        description="Latitude from the GNSS receiver (WGS84) [degrees]",
    )
    longitude: Series[float] = pa.Field(
        ge=-180,
        le=360,
        coerce=True,
        description="Longitude from the GNSS receiver (WGS84) [degrees]",
    )
    height: Series[float] = pa.Field(
        ge=-6378100,
        le=6378100,
        coerce=True,
        description="Height above ellipsoid [m]",
    )
    latitude_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of latitude [degrees]",
    )
    longitude_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of longitude [degrees]",
    )
    height_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of height [m]",
    )
    northVelocity: Series[float] = pa.Field(
        coerce=True,
        nullable=True,
        description="North velocity [m/s]",
    )
    eastVelocity: Series[float] = pa.Field(
        coerce=True,
        nullable=True,
        description="East velocity [m/s]",
    )
    upVelocity: Series[float] = pa.Field(
        coerce=True,
        nullable=True,
        description="Up velocity [m/s]",
    )
    northVelocity_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of north velocity [m/s]",
    )
    eastVelocity_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of east velocity [m/s]",
    )
    upVelocity_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of up velocity [m/s]",
    )
    roll_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of roll [degrees]",
    )
    pitch_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of pitch [degrees]",
    )
    azimuth_std: Series[float] = pa.Field(
        nullable=True,
        description="Standard deviation of azimuth/heading [degrees]",
    )

    class Config:
        coerce = True
        add_missing_columns = True
        drop_invalid_rows = True

    @pa.parser("time")
    def parse_time(cls, series: pd.Series) -> pd.Series:
        """Coerce integer/float epoch-millisecond ``time`` values to ``datetime64[ns]``."""
        return pd.to_datetime(series, unit="ms")
