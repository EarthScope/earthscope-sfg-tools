# ppp

`earthscope_sfg_tools.datamodels.observationdata.parsing.ppp`

_No docstring._

## class `KinPositionDataFrame`

Kinematic GNSS position solution stored in TileDB KinPosition arrays.

Columns match KinPositionArraySchema; time is the sparse dimension index.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x156ec6350>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x156ec62b0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x156ec6490>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x156ec6530>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x156ec65d0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x156ec6670>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x156ec6710>` |  |
| `number_of_satellites` | `<pandera.common.AnnotationInfo object at 0x156ec67b0>` |  |
| `pdop` | `<pandera.common.AnnotationInfo object at 0x156ec6850>` |  |
| `wrms` | `<pandera.common.AnnotationInfo object at 0x156ec68f0>` |  |
