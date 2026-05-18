# ppp

`earthscope_sfg_tools.datamodels.observationdata.parsing.ppp`

_No docstring._

## class `KinPositionDataFrame`

Kinematic GNSS position solution stored in TileDB KinPosition arrays.

Columns match KinPositionArraySchema; time is the sparse dimension index.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x13eaf6530>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x13eaf6490>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x13eaf6670>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x13eaf6710>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x13eaf67b0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x13eaf6850>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x13eaf68f0>` |  |
| `number_of_satellites` | `<pandera.common.AnnotationInfo object at 0x13eaf6990>` |  |
| `pdop` | `<pandera.common.AnnotationInfo object at 0x13eaf6a30>` |  |
| `wrms` | `<pandera.common.AnnotationInfo object at 0x13eaf6ad0>` |  |
