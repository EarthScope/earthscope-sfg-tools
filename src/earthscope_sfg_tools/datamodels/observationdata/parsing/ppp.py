from pandera.typing import Series
import pandera as pa
import pandas as pd


class KinPositionDataFrame(pa.DataFrameModel):
    """Kinematic GNSS position solution stored in TileDB KinPosition arrays.

    Columns match KinPositionArraySchema; time is the sparse dimension index.
    """

    time: Series[pd.DatetimeTZDtype] = pa.Field(
        dtype_kwargs={"unit": "ms", "tz": "UTC"},
        description="Observation timestamp [datetime64[ms, UTC]]",
    )
    latitude: Series[float] = pa.Field(ge=-90.0, le=90.0, description="Latitude [deg]")
    longitude: Series[float] = pa.Field(
        ge=-180.0, le=180.0, description="Longitude [deg]"
    )
    height: Series[float] = pa.Field(description="Ellipsoidal height [m]")
    east: Series[float] = pa.Field(description="East displacement [m]")
    north: Series[float] = pa.Field(description="North displacement [m]")
    up: Series[float] = pa.Field(description="Up displacement [m]")
    number_of_satellites: Series[int] = pa.Field(
        ge=0, description="Number of tracked satellites"
    )
    pdop: Series[float] = pa.Field(ge=0.0, description="Position dilution of precision")
    wrms: Series[float] = pa.Field(
        ge=0.0, description="Weighted RMS of position residuals [m]"
    )

    class Config:
        coerce = True
        strict = False
