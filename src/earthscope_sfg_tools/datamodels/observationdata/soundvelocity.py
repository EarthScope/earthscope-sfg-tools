from pandera.typing import Series
import pandera.pandas as pa


class SoundVelocityDataFrame(pa.DataFrameModel):
    depth: Series[float] = pa.Field(ge=0, le=10000, coerce=True)
    speed: Series[float] = pa.Field(unique=True, ge=0, le=3800, coerce=True)

    class Config:
        coerce = True
        drop_invalid_rows = True
