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
| `MT_ID` | `<pandera.common.AnnotationInfo object at 0x168c767b0>` |  |
| `TravelTime` | `<pandera.common.AnnotationInfo object at 0x168d88050>` |  |
| `T_transmit` | `<pandera.common.AnnotationInfo object at 0x168d88190>` |  |
| `X_transmit` | `<pandera.common.AnnotationInfo object at 0x168ce7e10>` |  |
| `Y_transmit` | `<pandera.common.AnnotationInfo object at 0x168d8c050>` |  |
| `Z_transmit` | `<pandera.common.AnnotationInfo object at 0x168ce2f90>` |  |
| `T_receive` | `<pandera.common.AnnotationInfo object at 0x104b827a0>` |  |
| `X_receive` | `<pandera.common.AnnotationInfo object at 0x168d517b0>` |  |
| `Y_receive` | `<pandera.common.AnnotationInfo object at 0x168d74a50>` |  |
| `Z_receive` | `<pandera.common.AnnotationInfo object at 0x168d74b50>` |  |
| `TDC_ID` | `<pandera.common.AnnotationInfo object at 0x168d4dd60>` |  |
| `aSNR` | `<pandera.common.AnnotationInfo object at 0x168d4de50>` |  |
| `acc` | `<pandera.common.AnnotationInfo object at 0x168d31d30>` |  |
| `dbV` | `<pandera.common.AnnotationInfo object at 0x168d33310>` |  |
| `quality_flag` | `<pandera.common.AnnotationInfo object at 0x13ca28e20>` |  |
| `ant_X0` | `<pandera.common.AnnotationInfo object at 0x168cbe090>` |  |
| `ant_Y0` | `<pandera.common.AnnotationInfo object at 0x168cbe150>` |  |
| `ant_Z0` | `<pandera.common.AnnotationInfo object at 0x10515cec0>` |  |
| `ant_sigX0` | `<pandera.common.AnnotationInfo object at 0x10515cd60>` |  |
| `ant_sigY0` | `<pandera.common.AnnotationInfo object at 0x168d6dd10>` |  |
| `ant_sigZ0` | `<pandera.common.AnnotationInfo object at 0x168d6dc70>` |  |
| `ant_X1` | `<pandera.common.AnnotationInfo object at 0x168d6def0>` |  |
| `ant_Y1` | `<pandera.common.AnnotationInfo object at 0x168d6ddb0>` |  |
| `ant_Z1` | `<pandera.common.AnnotationInfo object at 0x168d6df90>` |  |
| `ant_sigX1` | `<pandera.common.AnnotationInfo object at 0x168d6e030>` |  |
| `ant_sigY1` | `<pandera.common.AnnotationInfo object at 0x168d6e0d0>` |  |
| `ant_sigZ1` | `<pandera.common.AnnotationInfo object at 0x168d6e170>` |  |
| `heading0` | `<pandera.common.AnnotationInfo object at 0x168d6e210>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x168d6e2b0>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x168d6e350>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x168d6e3f0>` |  |
