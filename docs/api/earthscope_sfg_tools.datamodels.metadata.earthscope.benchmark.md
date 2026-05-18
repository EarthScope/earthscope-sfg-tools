# benchmark

`earthscope_sfg_tools.datamodels.metadata.earthscope.benchmark`

_No docstring._

## class `BatteryVoltage`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `date` | `datetime` | The date of the battery voltage reading |
| `voltage` | `float` | The battery voltage reading |

## class `Benchmark`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `str` | The name of the benchmark |
| `benchmarkID` | `Union` | The benchmark ID |
| `aPrioriLocation` | `Union` | The a priori location of the benchmark |
| `start` | `Union` | The start date of the benchmark |
| `end` | `Union` | The end date of the benchmark |
| `dropPointLocation` | `Union` | The drop point location of the benchmark |
| `transponders` | `Union` | The transponders attached to the benchmark |

**Methods**

### `Benchmark.get_transponder_by_datetime(self, dt: datetime.datetime) -> earthscope_sfg_tools.datamodels.metadata.earthscope.benchmark.Transponder | None`

Get the transponder for a given datetime.

Parameters
----------
dt : datetime
    The datetime to get the transponder for.

Returns
-------
Optional[Transponder]
    The transponder, or None if no transponder is found for the
    given datetime.


## class `ExtraSensors`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `type` | `str` | The type of the extra sensor |
| `serialNumber` | `str` | The serial number of the extra sensor |
| `model` | `str` | The model of the extra sensor |

## class `TAT`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `value` | `float` | Turn around time (TAT) in ms |
| `start` | `Union` | The start date of the TAT |
| `end` | `Union` | The end date of the TAT |

## class `Transponder`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `address` | `str` | The address of the transponder |
| `tat` | `list` | The turn around time (TAT) of the transponder |
| `start` | `Union` | The start date of the transponder |
| `end` | `Union` | The end date of the transponder (if removed) |
| `uid` | `Union` | The UID of the transponder |
| `model` | `Union` | The model of the transponder |
| `serialNumber` | `Union` | The serial number of the transponder |
| `batteryCapacity` | `Union` | The battery capacity of the transponder, e.g 4 Ah |
| `notes` | `Union` | Additional notes about the transponder or deployment |
| `batteryVoltage` | `Union` | The battery voltage of the transponder, including date and voltage |
| `extraSensors` | `Union` | Extra sensors attached to the transponder |

**Methods**

### `Transponder.get_tat_by_datetime(self, dt: datetime.datetime) -> float | None`

Get the turn around time (TAT) for a given datetime.

Parameters
----------
dt : datetime
    The datetime to get the TAT for.

Returns
-------
Optional[float]
    The TAT value, or None if no TAT is found for the given
    datetime.

