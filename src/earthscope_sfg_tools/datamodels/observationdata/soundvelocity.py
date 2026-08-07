"""Pandera schema for sound-velocity profile DataFrames."""

from pandera.typing import Series
import pandera.pandas as pa


class SoundVelocityDataFrame(pa.DataFrameModel):
    """Pandera schema for a sound velocity profile DataFrame.

    Enforces non-negative depth (0-10 000 m) and physically plausible speed
    values (0-3 800 m/s). Speed is not required to be unique: real CTD casts
    routinely have multiple depths with the same (rounded) sound speed, and
    that's not a data-quality problem - interpolation keys on depth, not
    speed, so a duplicate-speed constraint doesn't protect anything it claims
    to. With ``drop_invalid_rows = True``, a `unique` constraint here doesn't
    raise - it silently deletes every row in the duplicate group, which
    previously discarded genuine deep-water measurements (e.g. real CTD data
    reaching 1000 m getting truncated to 999 m) for no reason connected to
    interpolation safety.
    """

    depth: Series[float] = pa.Field(ge=0, le=10000, coerce=True)
    speed: Series[float] = pa.Field(ge=0, le=3800, coerce=True)

    class Config:
        coerce = True
        drop_invalid_rows = True
