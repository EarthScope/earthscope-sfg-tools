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
| `benchmarkID` | `str \| None` | The benchmark ID |
| `aPrioriLocation` | `earthscope_sfg_tools.datamodels.metadata.earthscope.utils.Location \| None` | The a priori location of the benchmark |
| `start` | `datetime.datetime \| None` | The start date of the benchmark |
| `end` | `datetime.datetime \| None` | The end date of the benchmark |
| `dropPointLocation` | `earthscope_sfg_tools.datamodels.metadata.earthscope.utils.Location \| None` | The drop point location of the benchmark |
| `transponders` | `list[earthscope_sfg_tools.datamodels.metadata.earthscope.benchmark.Transponder] \| None` | The transponders attached to the benchmark |

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
| `start` | `datetime.datetime \| None` | The start date of the TAT |
| `end` | `datetime.datetime \| None` | The end date of the TAT |

## class `Transponder`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `address` | `str` | The address of the transponder |
| `tat` | `list` | The turn around time (TAT) of the transponder |
| `start` | `datetime.datetime \| None` | The start date of the transponder |
| `end` | `datetime.datetime \| None` | The end date of the transponder (if removed) |
| `uid` | `str \| None` | The UID of the transponder |
| `model` | `str \| None` | The model of the transponder |
| `serialNumber` | `str \| None` | The serial number of the transponder |
| `batteryCapacity` | `str \| None` | The battery capacity of the transponder, e.g 4 Ah |
| `notes` | `str \| None` | Additional notes about the transponder or deployment |
| `batteryVoltage` | `list[earthscope_sfg_tools.datamodels.metadata.earthscope.benchmark.BatteryVoltage] \| None` | The battery voltage of the transponder, including date and voltage |
| `extraSensors` | `list[earthscope_sfg_tools.datamodels.metadata.earthscope.benchmark.ExtraSensors] \| None` | Extra sensors attached to the transponder |

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

