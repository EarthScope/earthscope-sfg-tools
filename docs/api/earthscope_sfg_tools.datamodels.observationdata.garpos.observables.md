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
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x168da2c10>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x168da2cb0>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x168da2d50>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x168da2df0>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x168da2e90>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x168da2f30>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x168da2fd0>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x168da3070>` |  |

## class `GARPOSShotDataFrame`

Pandera schema for a GARPOS-ready shot DataFrame.

Extends :class:`AcousticDataFrame` with full ECEF position and
attitude columns for both the transmit (``*0``) and receive (``*1``)
epochs, plus optional position standard deviations.

**Fields**

| Name | Type | Description |
|---|---|---|
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x168da3750>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x168da37f0>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x168da3890>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x168da3930>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x168da39d0>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x168da3a70>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x168da3b10>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x168da3bb0>` |  |
| `head0` | `<pandera.common.AnnotationInfo object at 0x168da3c50>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x168da3cf0>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x168da3d90>` |  |
| `east0` | `<pandera.common.AnnotationInfo object at 0x168da3e30>` |  |
| `north0` | `<pandera.common.AnnotationInfo object at 0x168da3ed0>` |  |
| `up0` | `<pandera.common.AnnotationInfo object at 0x168da3f70>` |  |
| `head1` | `<pandera.common.AnnotationInfo object at 0x168dc0050>` |  |
| `pitch1` | `<pandera.common.AnnotationInfo object at 0x168dc00f0>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x168dc0190>` |  |
| `east1` | `<pandera.common.AnnotationInfo object at 0x168dc0230>` |  |
| `north1` | `<pandera.common.AnnotationInfo object at 0x168dc02d0>` |  |
| `up1` | `<pandera.common.AnnotationInfo object at 0x168dc0370>` |  |
| `east_std0` | `<pandera.common.AnnotationInfo object at 0x168dc0410>` |  |
| `north_std0` | `<pandera.common.AnnotationInfo object at 0x168dc04b0>` |  |
| `up_std0` | `<pandera.common.AnnotationInfo object at 0x168dc0550>` |  |
| `east_std1` | `<pandera.common.AnnotationInfo object at 0x168dc05f0>` |  |
| `north_std1` | `<pandera.common.AnnotationInfo object at 0x168dc0690>` |  |
| `up_std1` | `<pandera.common.AnnotationInfo object at 0x168dc0730>` |  |

## class `IMUPositionDataFrame`

IMU/INS position and orientation solution stored in TileDB IMUPosition arrays.

Columns match IMUPositionArraySchema; time is the sparse dimension index.
Std-dev fields are nullable because they may not be populated by all receivers.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x168d6f930>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x168dc0af0>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x168dc0b90>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x168dc0c30>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x168dc0cd0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x168dc0d70>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x168dc0e10>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x168dc0eb0>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x168dc0f50>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x168dc0ff0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x168dc1090>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x168dc1130>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x168dc11d0>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x168dc1270>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x168dc1310>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x168dc13b0>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x168dc1450>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x168dc14f0>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x168dc1590>` |  |
