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
| `time` | `<pandera.common.AnnotationInfo object at 0x156e6f9d0>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x156e6de50>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x156e6fa70>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x156e6fb10>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x156e6fbb0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x156e6fc50>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x156e6fcf0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x156e6fd90>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x156e6fe30>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x156e6fed0>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x156e6ff70>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x156ea0050>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x156ea00f0>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x156ea0190>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x156ea0230>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x156ea02d0>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x156ea0370>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x156ea0410>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x156ea04b0>` |  |

## class `INSPVAASchema`

Schema for the INSPVAA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSPVA.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x156ea0d70>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x156ea0e10>` |  |
| `lat` | `<pandera.common.AnnotationInfo object at 0x156ea0eb0>` |  |
| `lon` | `<pandera.common.AnnotationInfo object at 0x156ea0f50>` |  |
| `alt` | `<pandera.common.AnnotationInfo object at 0x156ea0ff0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x156ea1090>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x156ea1130>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x156ea11d0>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x156ea1270>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x156ea1310>` |  |
| `heading` | `<pandera.common.AnnotationInfo object at 0x156ea13b0>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x156ea1450>` |  |

## class `INSSTDEVSchema`

Schema for the INSSTDEVA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel_std`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSSTDEV.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x156ea1c70>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x156ea1d10>` |  |
| `lat_sig` | `<pandera.common.AnnotationInfo object at 0x156ea1db0>` |  |
| `lon_sig` | `<pandera.common.AnnotationInfo object at 0x156ea1e50>` |  |
| `alt_sig` | `<pandera.common.AnnotationInfo object at 0x156ea1ef0>` |  |
| `north_sig` | `<pandera.common.AnnotationInfo object at 0x156ea1f90>` |  |
| `east_sig` | `<pandera.common.AnnotationInfo object at 0x156ea2030>` |  |
| `up_sig` | `<pandera.common.AnnotationInfo object at 0x156ea20d0>` |  |
| `cov_rr` | `<pandera.common.AnnotationInfo object at 0x156ea2170>` |  |
| `cov_pp` | `<pandera.common.AnnotationInfo object at 0x156ea2210>` |  |
| `cov_hh` | `<pandera.common.AnnotationInfo object at 0x156ea22b0>` |  |
| `ext_sol_stat` | `<pandera.common.AnnotationInfo object at 0x156ea2350>` |  |
| `time_since_update` | `<pandera.common.AnnotationInfo object at 0x156ea23f0>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x156ea2490>` |  |
