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
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b81070>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b567b0>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1afc0>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19370>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1af30>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1b050>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1b080>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1ac00>` |  |

## class `GARPOSShotDataFrame`

Pandera schema for a GARPOS-ready shot DataFrame.

Extends :class:`AcousticDataFrame` with full ECEF position and
attitude columns for both the transmit (``*0``) and receive (``*1``)
epochs, plus optional position standard deviations.

**Fields**

| Name | Type | Description |
|---|---|---|
| `transponderID` | `<pandera.common.AnnotationInfo object at 0x7ff6a63a0d70>` |  |
| `pingTime` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1b110>` |  |
| `returnTime` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1b410>` |  |
| `tt` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a1b3e0>` |  |
| `dbv` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85100>` |  |
| `xc` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a84fb0>` |  |
| `snr` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85040>` |  |
| `tat` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85160>` |  |
| `head0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a851c0>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85220>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85280>` |  |
| `east0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a852e0>` |  |
| `north0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a852b0>` |  |
| `up0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a850a0>` |  |
| `head1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85310>` |  |
| `pitch1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85340>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85370>` |  |
| `east1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a853a0>` |  |
| `north1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a853d0>` |  |
| `up1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85400>` |  |
| `east_std0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85430>` |  |
| `north_std0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85460>` |  |
| `up_std0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85490>` |  |
| `east_std1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a854c0>` |  |
| `north_std1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a854f0>` |  |
| `up_std1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85520>` |  |

## class `IMUPositionDataFrame`

IMU/INS position and orientation solution stored in TileDB IMUPosition arrays.

Columns match IMUPositionArraySchema; time is the sparse dimension index.
Std-dev fields are nullable because they may not be populated by all receivers.

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85550>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a876b0>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85c10>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a85be0>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a87fb0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90110>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90140>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a900b0>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a901d0>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a901a0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90170>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90200>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90230>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90260>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90290>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a902c0>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a902f0>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90320>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a90350>` |  |
