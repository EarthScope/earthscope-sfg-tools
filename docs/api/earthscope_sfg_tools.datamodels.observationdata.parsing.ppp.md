# ppp

`earthscope_sfg_tools.datamodels.observationdata.parsing.ppp`

_No docstring._

## class `KinPositionDataFrame`

Kinematic GNSS position solution stored in TileDB KinPosition arrays.

Columns match KinPositionArraySchema; time is the sparse dimension index.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x7ff6a5d964e0>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x7ff6a5beafc0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90380>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a908c0>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a927b0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a92510>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a904a0>` |  |
| `number_of_satellites` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a927e0>` |  |
| `pdop` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a92810>` |  |
| `wrms` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a92840>` |  |
