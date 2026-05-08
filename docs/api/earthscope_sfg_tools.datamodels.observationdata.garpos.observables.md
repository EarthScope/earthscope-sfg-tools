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
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x156ea2c10>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x156ea2cb0>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x156ea2d50>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x156ea2df0>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x156ea2e90>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x156ea2f30>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x156ea2fd0>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x156ea3070>` |  |

## class `GARPOSShotDataFrame`

Pandera schema for a GARPOS-ready shot DataFrame.

Extends :class:`AcousticDataFrame` with full ECEF position and
attitude columns for both the transmit (``*0``) and receive (``*1``)
epochs, plus optional position standard deviations.

**Fields**

| Name | Type | Description |
|---|---|---|
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x156ea3750>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x156ea37f0>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x156ea3890>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x156ea3930>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x156ea39d0>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x156ea3a70>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x156ea3b10>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x156ea3bb0>` |  |
| `head0` | `<pandera.common.AnnotationInfo object at 0x156ea3c50>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x156ea3cf0>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x156ea3d90>` |  |
| `east0` | `<pandera.common.AnnotationInfo object at 0x156ea3e30>` |  |
| `north0` | `<pandera.common.AnnotationInfo object at 0x156ea3ed0>` |  |
| `up0` | `<pandera.common.AnnotationInfo object at 0x156ea3f70>` |  |
| `head1` | `<pandera.common.AnnotationInfo object at 0x156ec4050>` |  |
| `pitch1` | `<pandera.common.AnnotationInfo object at 0x156ec40f0>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x156ec4190>` |  |
| `east1` | `<pandera.common.AnnotationInfo object at 0x156ec4230>` |  |
| `north1` | `<pandera.common.AnnotationInfo object at 0x156ec42d0>` |  |
| `up1` | `<pandera.common.AnnotationInfo object at 0x156ec4370>` |  |
| `east_std0` | `<pandera.common.AnnotationInfo object at 0x156ec4410>` |  |
| `north_std0` | `<pandera.common.AnnotationInfo object at 0x156ec44b0>` |  |
| `up_std0` | `<pandera.common.AnnotationInfo object at 0x156ec4550>` |  |
| `east_std1` | `<pandera.common.AnnotationInfo object at 0x156ec45f0>` |  |
| `north_std1` | `<pandera.common.AnnotationInfo object at 0x156ec4690>` |  |
| `up_std1` | `<pandera.common.AnnotationInfo object at 0x156ec4730>` |  |

## class `IMUPositionDataFrame`

IMU/INS position and orientation solution stored in TileDB IMUPosition arrays.

Columns match IMUPositionArraySchema; time is the sparse dimension index.
Std-dev fields are nullable because they may not be populated by all receivers.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x156e6f930>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x156ec4af0>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x156ec4b90>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x156ec4c30>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x156ec4cd0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x156ec4d70>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x156ec4e10>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x156ec4eb0>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x156ec4f50>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x156ec4ff0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x156ec5090>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x156ec5130>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x156ec51d0>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x156ec5270>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x156ec5310>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x156ec53b0>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x156ec5450>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x156ec54f0>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x156ec5590>` |  |
