# ppp

`earthscope_sfg_tools.datamodels.observationdata.parsing.ppp`

_No docstring._

## class `KinPositionDataFrame`

Kinematic GNSS position solution stored in TileDB KinPosition arrays.

Columns match KinPositionArraySchema; time is the sparse dimension index.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x168dc2350>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x168dc22b0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x168dc2490>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x168dc2530>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x168dc25d0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x168dc2670>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x168dc2710>` |  |
| `number_of_satellites` | `<pandera.common.AnnotationInfo object at 0x168dc27b0>` |  |
| `pdop` | `<pandera.common.AnnotationInfo object at 0x168dc2850>` |  |
| `wrms` | `<pandera.common.AnnotationInfo object at 0x168dc28f0>` |  |
