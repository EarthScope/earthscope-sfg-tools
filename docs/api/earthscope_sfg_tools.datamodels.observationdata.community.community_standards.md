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
| `MT_ID` | `<pandera.common.AnnotationInfo object at 0x13e9bdd30>` |  |
| `TravelTime` | `<pandera.common.AnnotationInfo object at 0x13eab82d0>` |  |
| `T_transmit` | `<pandera.common.AnnotationInfo object at 0x13eab8410>` |  |
| `X_transmit` | `<pandera.common.AnnotationInfo object at 0x13ea4d5b0>` |  |
| `Y_transmit` | `<pandera.common.AnnotationInfo object at 0x13ea4d480>` |  |
| `Z_transmit` | `<pandera.common.AnnotationInfo object at 0x13e9c1fd0>` |  |
| `T_receive` | `<pandera.common.AnnotationInfo object at 0x100bde7a0>` |  |
| `X_receive` | `<pandera.common.AnnotationInfo object at 0x13ea56360>` |  |
| `Y_receive` | `<pandera.common.AnnotationInfo object at 0x13eaa0c50>` |  |
| `Z_receive` | `<pandera.common.AnnotationInfo object at 0x13eaa0d50>` |  |
| `TDC_ID` | `<pandera.common.AnnotationInfo object at 0x13ea9c7d0>` |  |
| `aSNR` | `<pandera.common.AnnotationInfo object at 0x13ea9cc80>` |  |
| `acc` | `<pandera.common.AnnotationInfo object at 0x13ea7cf30>` |  |
| `dbV` | `<pandera.common.AnnotationInfo object at 0x13ea7ecf0>` |  |
| `quality_flag` | `<pandera.common.AnnotationInfo object at 0x13e90a5b0>` |  |
| `ant_X0` | `<pandera.common.AnnotationInfo object at 0x13e9dda90>` |  |
| `ant_Y0` | `<pandera.common.AnnotationInfo object at 0x13e9ddc10>` |  |
| `ant_Z0` | `<pandera.common.AnnotationInfo object at 0x13ea659c0>` |  |
| `ant_sigX0` | `<pandera.common.AnnotationInfo object at 0x100c914f0>` |  |
| `ant_sigY0` | `<pandera.common.AnnotationInfo object at 0x13ea45e50>` |  |
| `ant_sigZ0` | `<pandera.common.AnnotationInfo object at 0x13ea45ef0>` |  |
| `ant_X1` | `<pandera.common.AnnotationInfo object at 0x13ea460d0>` |  |
| `ant_Y1` | `<pandera.common.AnnotationInfo object at 0x13ea45f90>` |  |
| `ant_Z1` | `<pandera.common.AnnotationInfo object at 0x13ea46170>` |  |
| `ant_sigX1` | `<pandera.common.AnnotationInfo object at 0x13ea46210>` |  |
| `ant_sigY1` | `<pandera.common.AnnotationInfo object at 0x13ea462b0>` |  |
| `ant_sigZ1` | `<pandera.common.AnnotationInfo object at 0x13ea46350>` |  |
| `heading0` | `<pandera.common.AnnotationInfo object at 0x13ea463f0>` |  |
| `pitch0` | `<pandera.common.AnnotationInfo object at 0x13ea46490>` |  |
| `roll0` | `<pandera.common.AnnotationInfo object at 0x13ea46530>` |  |
| `roll1` | `<pandera.common.AnnotationInfo object at 0x13ea465d0>` |  |
