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
| `time` | `<pandera.common.AnnotationInfo object at 0x168d6f9d0>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x168d6de50>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x168d6fa70>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x168d6fb10>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x168d6fbb0>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x168d6fc50>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x168d6fcf0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x168d6fd90>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x168d6fe30>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x168d6fed0>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x168d6ff70>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x168da0050>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x168da00f0>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x168da0190>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x168da0230>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x168da02d0>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x168da0370>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x168da0410>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x168da04b0>` |  |

## class `INSPVAASchema`

Schema for the INSPVAA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSPVA.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x168da0d70>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x168da0e10>` |  |
| `lat` | `<pandera.common.AnnotationInfo object at 0x168da0eb0>` |  |
| `lon` | `<pandera.common.AnnotationInfo object at 0x168da0f50>` |  |
| `alt` | `<pandera.common.AnnotationInfo object at 0x168da0ff0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x168da1090>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x168da1130>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x168da11d0>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x168da1270>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x168da1310>` |  |
| `heading` | `<pandera.common.AnnotationInfo object at 0x168da13b0>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x168da1450>` |  |

## class `INSSTDEVSchema`

Schema for the INSSTDEVA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel_std`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSSTDEV.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x168da1c70>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x168da1d10>` |  |
| `lat_sig` | `<pandera.common.AnnotationInfo object at 0x168da1db0>` |  |
| `lon_sig` | `<pandera.common.AnnotationInfo object at 0x168da1e50>` |  |
| `alt_sig` | `<pandera.common.AnnotationInfo object at 0x168da1ef0>` |  |
| `north_sig` | `<pandera.common.AnnotationInfo object at 0x168da1f90>` |  |
| `east_sig` | `<pandera.common.AnnotationInfo object at 0x168da2030>` |  |
| `up_sig` | `<pandera.common.AnnotationInfo object at 0x168da20d0>` |  |
| `cov_rr` | `<pandera.common.AnnotationInfo object at 0x168da2170>` |  |
| `cov_pp` | `<pandera.common.AnnotationInfo object at 0x168da2210>` |  |
| `cov_hh` | `<pandera.common.AnnotationInfo object at 0x168da22b0>` |  |
| `ext_sol_stat` | `<pandera.common.AnnotationInfo object at 0x168da2350>` |  |
| `time_since_update` | `<pandera.common.AnnotationInfo object at 0x168da23f0>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x168da2490>` |  |
