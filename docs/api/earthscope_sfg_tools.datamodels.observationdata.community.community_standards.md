# community_standards

`earthscope_sfg_tools.datamodels.observationdata.community.community_standards`

Community-standard data models for GNSS-A acoustic exchange.

## class `SFGDSTFSeafloorAcousticData`

Pandera schema for the SFG DSTF seafloor acoustic exchange format.

Defines the required columns (MT ID, travel time, transmit/receive
timestamps and ECEF positions) and optional diagnostic/quality
columns (SNR, dBV, cross-correlation, attitude, position
uncertainties) for data exchange between GNSS-A processing systems.

**Fields**

| Name | Type | Description |
|---|---|---|
| `MT_ID` | `<pandera.common.AnnotationInfo object at 0x156d72900>` |  |
| `TravelTime` | `<pandera.common.AnnotationInfo object at 0x156e8c050>` |  |
| `T_transmit` | `<pandera.common.AnnotationInfo object at 0x156e8c190>` |  |
| `X_transmit` | `<pandera.common.AnnotationInfo object at 0x156debe10>` |  |
| `Y_transmit` | `<pandera.common.AnnotationInfo object at 0x156e90050>` |  |
| `Z_transmit` | `<pandera.common.AnnotationInfo object at 0x156de2f90>` |  |
| `T_receive` | `<pandera.common.AnnotationInfo object at 0x10304a7a0>` |  |
| `X_receive` | `<pandera.common.AnnotationInfo object at 0x156e517b0>` |  |
| `Y_receive` | `<pandera.common.AnnotationInfo object at 0x156e74a50>` |  |
| `Z_receive` | `<pandera.common.AnnotationInfo object at 0x156e74b50>` |  |
| `TDC_ID` | `<pandera.common.AnnotationInfo object at 0x156e49e50>` |  |
| `aSNR` | `<pandera.common.AnnotationInfo object at 0x156e49f40>` |  |
| `acc` | `<pandera.common.AnnotationInfo object at 0x156e31d30>` |  |
| `dbV` | `<pandera.common.AnnotationInfo object at 0x156e33310>` |  |
| `quality_flag` | `<pandera.common.AnnotationInfo object at 0x13332ce20>` |  |
| `ant_X0` | `<pandera.common.AnnotationInfo object at 0x156dba150>` |  |
| `ant_Y0` | `<pandera.common.AnnotationInfo object at 0x156dba210>` |  |
| `ant_Z0` | `<pandera.common.AnnotationInfo object at 0x1030fd020>` |  |
| `ant_sigX0` | `<pandera.common.AnnotationInfo object at 0x1030fcec0>` |  |
| `ant_sigY0` | `<pandera.common.AnnotationInfo object at 0x156e6dd10>` |  |
| `ant_sigZ0` | `<pandera.common.AnnotationInfo object at 0x156e6dc70>` |  |
| `ant_X1` | `<pandera.common.AnnotationInfo object at 0x156e6def0>` |  |
| `ant_Y1` | `<pandera.common.AnnotationInfo object at 0x156e6ddb0>` |  |
| `ant_Z1` | `<pandera.common.AnnotationInfo object at 0x156e6df90>` |  |
| `ant_sigX1` | `<pandera.common.AnnotationInfo object at 0x156e6e030>` |  |
| `ant_sigY1` | `<pandera.common.AnnotationInfo object at 0x156e6e0d0>` |  |
| `ant_sigZ1` | `<pandera.common.AnnotationInfo object at 0x156e6e170>` |  |
| `heading0` | `<pandera.common.AnnotationInfo object at 0x156e6e210>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x156e6e2b0>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x156e6e350>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x156e6e3f0>` |  |
