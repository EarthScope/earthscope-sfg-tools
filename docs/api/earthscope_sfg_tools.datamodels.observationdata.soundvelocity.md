# soundvelocity

`earthscope_sfg_tools.datamodels.observationdata.soundvelocity`

_No docstring._

## class `SoundVelocityDataFrame`

Pandera schema for a sound velocity profile DataFrame.

Enforces non-negative depth (0–10 000 m), physically plausible speed
values (0–3 800 m/s), and uniqueness of speed values to prevent
duplicate rows from breaking interpolation.

**Fields**

| Name | Type | Description |
|---|---|---|
| `depth` | `<pandera.common.AnnotationInfo object at 0x168dc2df0>` |  |
| `speed` | `<pandera.common.AnnotationInfo object at 0x168dc23f0>` |  |
