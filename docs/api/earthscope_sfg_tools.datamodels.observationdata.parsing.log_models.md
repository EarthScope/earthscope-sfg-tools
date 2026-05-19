# log_models

`earthscope_sfg_tools.datamodels.observationdata.parsing.log_models`

Pydantic models for parsed SV3 interrogation/reply records.

## class `SV3InterrogationData`

GARPOS-formatted interrogation record for one acoustic ping.

Holds the ECEF position and attitude of the vessel transducer at
ping time, position standard deviations, and the outgoing ping
timestamp in GPS seconds.

**Fields**

| Name | Type | Description |
|---|---|---|
| `head0` | `Decimal` |  |
| `pitch0` | `Decimal` |  |
| `roll0` | `Decimal` |  |
| `east0` | `Decimal` |  |
| `north0` | `Decimal` |  |
| `up0` | `Decimal` |  |
| `east_std0` | `decimal.Decimal \| None` |  |
| `north_std0` | `decimal.Decimal \| None` |  |
| `up_std0` | `decimal.Decimal \| None` |  |
| `pingTime` | `Decimal` |  |

## class `SV3ReplyData`

GARPOS-formatted reply record for one acoustic return.

Extends the interrogation geometry with the receive-epoch ECEF
position and attitude, transponder ID, acoustic diagnostics (SNR,
dBV, cross-correlation), hardware turnaround time (TAT), one-way
travel time, and return timestamp in GPS seconds.

**Fields**

| Name | Type | Description |
|---|---|---|
| `head1` | `Decimal` |  |
| `pitch1` | `Decimal` |  |
| `roll1` | `Decimal` |  |
| `east1` | `Decimal` |  |
| `north1` | `Decimal` |  |
| `up1` | `Decimal` |  |
| `transponderID` | `str` |  |
| `dbv` | `Decimal` |  |
| `snr` | `Decimal` |  |
| `xc` | `Decimal` |  |
| `tt` | `Decimal` |  |
| `tat` | `Decimal` |  |
| `returnTime` | `Decimal` |  |
| `east_std1` | `decimal.Decimal \| None` |  |
| `north_std1` | `decimal.Decimal \| None` |  |
| `up_std1` | `decimal.Decimal \| None` |  |
