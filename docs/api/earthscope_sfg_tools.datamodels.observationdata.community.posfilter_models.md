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
| `time` | `<pandera.common.AnnotationInfo object at 0x13ea47bb0>` |  |
| `azimuth` | `<pandera.common.AnnotationInfo object at 0x13ea46030>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x13ea47c50>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x13ea47cf0>` |  |
| `latitude` | `<pandera.common.AnnotationInfo object at 0x13ea47d90>` |  |
| `longitude` | `<pandera.common.AnnotationInfo object at 0x13ea47e30>` |  |
| `height` | `<pandera.common.AnnotationInfo object at 0x13ea47ed0>` |  |
| `latitude_std` | `<pandera.common.AnnotationInfo object at 0x13ea47f70>` |  |
| `longitude_std` | `<pandera.common.AnnotationInfo object at 0x13eadc050>` |  |
| `height_std` | `<pandera.common.AnnotationInfo object at 0x13eadc0f0>` |  |
| `northVelocity` | `<pandera.common.AnnotationInfo object at 0x13eadc190>` |  |
| `eastVelocity` | `<pandera.common.AnnotationInfo object at 0x13eadc230>` |  |
| `upVelocity` | `<pandera.common.AnnotationInfo object at 0x13eadc2d0>` |  |
| `northVelocity_std` | `<pandera.common.AnnotationInfo object at 0x13eadc370>` |  |
| `eastVelocity_std` | `<pandera.common.AnnotationInfo object at 0x13eadc410>` |  |
| `upVelocity_std` | `<pandera.common.AnnotationInfo object at 0x13eadc4b0>` |  |
| `roll_std` | `<pandera.common.AnnotationInfo object at 0x13eadc550>` |  |
| `pitch_std` | `<pandera.common.AnnotationInfo object at 0x13eadc5f0>` |  |
| `azimuth_std` | `<pandera.common.AnnotationInfo object at 0x13eadc690>` |  |

## class `INSPVAASchema`

Schema for the INSPVAA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSPVA.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x13eadcf50>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x13eadcff0>` |  |
| `lat` | `<pandera.common.AnnotationInfo object at 0x13eadd090>` |  |
| `lon` | `<pandera.common.AnnotationInfo object at 0x13eadd130>` |  |
| `alt` | `<pandera.common.AnnotationInfo object at 0x13eadd1d0>` |  |
| `north` | `<pandera.common.AnnotationInfo object at 0x13eadd270>` |  |
| `east` | `<pandera.common.AnnotationInfo object at 0x13eadd310>` |  |
| `up` | `<pandera.common.AnnotationInfo object at 0x13eadd3b0>` |  |
| `roll` | `<pandera.common.AnnotationInfo object at 0x13eadd450>` |  |
| `pitch` | `<pandera.common.AnnotationInfo object at 0x13eadd4f0>` |  |
| `heading` | `<pandera.common.AnnotationInfo object at 0x13eadd590>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x13eadd630>` |  |

## class `INSSTDEVSchema`

Schema for the INSSTDEVA (NovAtel Level-1) DataFrame.

Loaded via :func:`gnatss.loaders.load_novatel_std`.
Specification: https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSSTDEV.htm

**Fields**

| Name | Type | Description |
|---|---|---|
| `week` | `<pandera.common.AnnotationInfo object at 0x13eadde50>` |  |
| `seconds` | `<pandera.common.AnnotationInfo object at 0x13eaddef0>` |  |
| `lat_sig` | `<pandera.common.AnnotationInfo object at 0x13eaddf90>` |  |
| `lon_sig` | `<pandera.common.AnnotationInfo object at 0x13eade030>` |  |
| `alt_sig` | `<pandera.common.AnnotationInfo object at 0x13eade0d0>` |  |
| `north_sig` | `<pandera.common.AnnotationInfo object at 0x13eade170>` |  |
| `east_sig` | `<pandera.common.AnnotationInfo object at 0x13eade210>` |  |
| `up_sig` | `<pandera.common.AnnotationInfo object at 0x13eade2b0>` |  |
| `cov_rr` | `<pandera.common.AnnotationInfo object at 0x13eade350>` |  |
| `cov_pp` | `<pandera.common.AnnotationInfo object at 0x13eade3f0>` |  |
| `cov_hh` | `<pandera.common.AnnotationInfo object at 0x13eade490>` |  |
| `ext_sol_stat` | `<pandera.common.AnnotationInfo object at 0x13eade530>` |  |
| `time_since_update` | `<pandera.common.AnnotationInfo object at 0x13eade5d0>` |  |
| `time` | `<pandera.common.AnnotationInfo object at 0x13eade670>` |  |
