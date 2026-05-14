# observables

`earthscope_sfg_tools.datamodels.observationdata.garpos.observables`

Pandera dataframe schemas used by migrated parsing workflows.

## class `AcousticDataFrame`

Base Pandera schema for raw acoustic ranging data.

Defines the minimal set of columns required by all acoustic shot
DataFrames: transponder ID, ping and return timestamps (GPS seconds),
one-way travel time, signal diagnostics, TAT, and SNR.

**Fields**

| Name | Type | Description |
|---|---|---|
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x13eadedf0>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x13eadee90>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x13eadef30>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x13eadefd0>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x13eadf070>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x13eadf110>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x13eadf1b0>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x13eadf250>` |  |

## class `GARPOSShotDataFrame`

Pandera schema for a GARPOS-ready shot DataFrame.

Extends :class:`AcousticDataFrame` with full ECEF position and
attitude columns for both the transmit (``*0``) and receive (``*1``)
epochs, plus optional position standard deviations.

**Fields**

| Name | Type | Description |
|---|---|---|
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x13eadf930>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x13eadf9d0>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x13eadfa70>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x13eadfb10>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x13eadfbb0>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x13eadfc50>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x13eadfcf0>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x13eadfd90>` |  |
| `head0` | `<pandera.common.AnnotationInfo object at 0x13eadfe30>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x13eadfed0>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x13eadff70>` |  |
| `east0` | `<pandera.common.AnnotationInfo object at 0x13eaf4050>` |  |
| `north0` | `<pandera.common.AnnotationInfo object at 0x13eaf40f0>` |  |
| `up0` | `<pandera.common.AnnotationInfo object at 0x13eaf4190>` |  |
| `head1` | `<pandera.common.AnnotationInfo object at 0x13eaf4230>` |  |
| `pitch1` | `<pandera.common.AnnotationInfo object at 0x13eaf42d0>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x13eaf4370>` |  |
| `east1` | `<pandera.common.AnnotationInfo object at 0x13eaf4410>` |  |
| `north1` | `<pandera.common.AnnotationInfo object at 0x13eaf44b0>` |  |
| `up1` | `<pandera.common.AnnotationInfo object at 0x13eaf4550>` |  |
| `east_std0` | `<pandera.common.AnnotationInfo object at 0x13eaf45f0>` |  |
| `north_std0` | `<pandera.common.AnnotationInfo object at 0x13eaf4690>` |  |
| `up_std0` | `<pandera.common.AnnotationInfo object at 0x13eaf4730>` |  |
| `east_std1` | `<pandera.common.AnnotationInfo object at 0x13eaf47d0>` |  |
| `north_std1` | `<pandera.common.AnnotationInfo object at 0x13eaf4870>` |  |
| `up_std1` | `<pandera.common.AnnotationInfo object at 0x13eaf4910>` |  |

## class `IMUPositionDataFrame`

IMU/INS position and orientation solution stored in TileDB IMUPosition arrays.

Columns match IMUPositionArraySchema; time is the sparse dimension index.
Std-dev fields are nullable because they may not be populated by all receivers.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x13ea47b10>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x13eaf4cd0>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x13eaf4d70>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x13eaf4e10>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x13eaf4eb0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x13eaf4f50>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x13eaf4ff0>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x13eaf5090>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x13eaf5130>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x13eaf51d0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x13eaf5270>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x13eaf5310>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x13eaf53b0>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x13eaf5450>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x13eaf54f0>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x13eaf5590>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x13eaf5630>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x13eaf56d0>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x13eaf5770>` |  |
