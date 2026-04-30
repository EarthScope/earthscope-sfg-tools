"""Pandera DataFrame schemas for the GNSS-A Kalman filter position filter inputs.

Column names follow the gnatss constants convention:
  - time        : J2000 epoch seconds (TIME_J2000 / GPS_TIME = "time")
  - east/north/up : local ENU tangent frame (GPS_LOCAL_TANGENT)
  - ant_x/y/z   : ECEF antenna position (ANT_GPS_GEOCENTRIC)
  - ant_sigx/y/z: ECEF antenna position std dev (ANT_GPS_GEOCENTRIC_STD)
"""

from __future__ import annotations

import pandera.pandas as pa
from pandera.typing import Series


class INSPVAADataFrame(pa.DataFrameModel):
    """INSPVAA: INS+GNSS combined antenna solution with ENU velocity.

    Minimum required columns for kalman_filtering() inspvaa_df input.
    The function selects only time and the three velocity components;
    additional columns (lat, lon, alt, roll, pitch, heading) are allowed.
    """

    time: Series[float] = pa.Field(description="J2000 epoch time [s]")
    east: Series[float] = pa.Field(
        nullable=True,
        description="East velocity component [m/s]",
    )
    north: Series[float] = pa.Field(
        nullable=True,
        description="North velocity component [m/s]",
    )
    up: Series[float] = pa.Field(
        nullable=True,
        description="Up velocity component [m/s]",
    )

    class Config:
        coerce = True
        strict = False


class INSSTDEVADataFrame(pa.DataFrameModel):
    """INSSTDEVA: INS+GNSS ENU velocity standard deviations.

    Minimum required columns for kalman_filtering() insstdeva_df input.
    The function selects only time and the three ENU velocity sigma columns;
    additional columns (lat_sig, lon_sig, alt_sig, cov_rr, etc.) are allowed.
    """

    time: Series[float] = pa.Field(description="J2000 epoch time [s]")
    east_sig: Series[float] = pa.Field(
        ge=0.0,
        nullable=True,
        description="East velocity standard deviation [m/s]",
    )
    north_sig: Series[float] = pa.Field(
        ge=0.0,
        nullable=True,
        description="North velocity standard deviation [m/s]",
    )
    up_sig: Series[float] = pa.Field(
        ge=0.0,
        nullable=True,
        description="Up velocity standard deviation [m/s]",
    )

    class Config:
        coerce = True
        strict = False


class GPSPositionDataFrame(pa.DataFrameModel):
    """GPS ECEF antenna positions with standard deviations.

    Required columns for kalman_filtering() gps_df input.
    Position and sigma columns are nullable because the GPS solutions may not
    cover every timestamp in the merged filter dataset.
    """

    time: Series[float] = pa.Field(description="J2000 epoch time [s]")
    ant_x: Series[float] = pa.Field(
        nullable=True,
        description="Antenna ECEF X position [m]",
    )
    ant_y: Series[float] = pa.Field(
        nullable=True,
        description="Antenna ECEF Y position [m]",
    )
    ant_z: Series[float] = pa.Field(
        nullable=True,
        description="Antenna ECEF Z position [m]",
    )
    ant_sigx: Series[float] = pa.Field(
        ge=0.0,
        nullable=True,
        description="Antenna ECEF X standard deviation [m]",
    )
    ant_sigy: Series[float] = pa.Field(
        ge=0.0,
        nullable=True,
        description="Antenna ECEF Y standard deviation [m]",
    )
    ant_sigz: Series[float] = pa.Field(
        ge=0.0,
        nullable=True,
        description="Antenna ECEF Z standard deviation [m]",
    )

    class Config:
        coerce = True
        strict = False

