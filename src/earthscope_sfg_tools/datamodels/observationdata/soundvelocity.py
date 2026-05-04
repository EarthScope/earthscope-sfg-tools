from pandera.typing import Series
import pandera.pandas as pa


class SoundVelocityDataFrame(pa.DataFrameModel):
    """Pandera schema for a sound velocity profile DataFrame.

    Enforces non-negative depth (0–10 000 m), physically plausible speed
    values (0–3 800 m/s), and uniqueness of speed values to prevent
    duplicate rows from breaking interpolation.
    """
    depth: Series[float] = pa.Field(ge=0, le=10000, coerce=True)
    speed: Series[float] = pa.Field(unique=True, ge=0, le=3800, coerce=True)

    class Config:
        coerce = True
        drop_invalid_rows = True
