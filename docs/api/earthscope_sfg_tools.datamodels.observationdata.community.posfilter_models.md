# posfilter_models

`earthscope_sfg_tools.datamodels.observationdata.community.posfilter_models`

Pandera DataFrame schemas for the GNSS-A Kalman filter position filter inputs.

Column names follow the gnatss constants convention:
  - time        : J2000 epoch seconds (TIME_J2000 / GPS_TIME = "time")
  - east/north/up : local ENU tangent frame (GPS_LOCAL_TANGENT)
  - ant_x/y/z   : ECEF antenna position (ANT_GPS_GEOCENTRIC)
  - ant_sigx/y/z: ECEF antenna position std dev (ANT_GPS_GEOCENTRIC_STD)

## class `IMUPositionDataFrame`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `time` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b56030>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7bc0>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7ef0>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b12a80>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a277d0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27860>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27890>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a241a0>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27920>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a278f0>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a278c0>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27950>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27980>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a279b0>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a279e0>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27a10>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27a40>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27a70>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27aa0>` |  |

## class `INSPVAASchema`

Schema for the INSPVAA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSPVA.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x7ff6a6aa90d0>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b80b90>` |  |
| `lat` | `<pandera.common.AnnotationInfo object at 0x7ff6a7dd9400>` |  |
| `lon` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a27c80>` |  |
| `alt` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2dfa0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e030>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e060>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2d970>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e0f0>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e0c0>` |  |
| `heading` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e090>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e120>` |  |

## class `INSSTDEVSchema`

Schema for the INSSTDEVA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel_std`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSSTDEV.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x7ff6a6dccce0>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e540>` |  |
| `lat_sig` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e150>` |  |
| `lon_sig` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a2e510>` |  |
| `alt_sig` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19160>` |  |
| `north_sig` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a191f0>` |  |
| `east_sig` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19220>` |  |
| `up_sig` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a18a40>` |  |
| `cov_rr` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a192b0>` |  |
| `cov_pp` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19280>` |  |
| `cov_hh` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19250>` |  |
| `ext_sol_stat` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a192e0>` |  |
| `time_since_update` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19310>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x7ff6a5a19340>` |  |
