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
| `MT_ID` | `<pandera.common.AnnotationInfo object at 0x7ff6a7638a40>` |  |
| `TravelTime` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b36b40>` |  |
| `T_transmit` | `<pandera.common.AnnotationInfo object at 0x7ff6a7d29880>` |  |
| `X_transmit` | `<pandera.common.AnnotationInfo object at 0x7ff6a5b11f70>` |  |
| `Y_transmit` | `<pandera.common.AnnotationInfo object at 0x7ff6a6e4e4e0>` |  |
| `Z_transmit` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7650>` |  |
| `T_receive` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7320>` |  |
| `X_receive` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc75f0>` |  |
| `Y_receive` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc76b0>` |  |
| `Z_receive` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7740>` |  |
| `TDC_ID` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc77a0>` |  |
| `aSNR` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7800>` |  |
| `acc` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7860>` |  |
| `dbV` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7830>` |  |
| `quality_flag` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc6c90>` |  |
| `ant_X0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7890>` |  |
| `ant_Y0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc78c0>` |  |
| `ant_Z0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc78f0>` |  |
| `ant_sigX0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7920>` |  |
| `ant_sigY0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7950>` |  |
| `ant_sigZ0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7980>` |  |
| `ant_X1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc79b0>` |  |
| `ant_Y1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc79e0>` |  |
| `ant_Z1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7a40>` |  |
| `ant_sigX1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7a70>` |  |
| `ant_sigY1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7aa0>` |  |
| `ant_sigZ1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7ad0>` |  |
| `heading0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7b00>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7b30>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7b60>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x7ff6a5bc7b90>` |  |
