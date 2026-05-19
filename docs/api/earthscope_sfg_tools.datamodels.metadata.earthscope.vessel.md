# vessel

`earthscope_sfg_tools.datamodels.metadata.earthscope.vessel`

_No docstring._

## class `AcousticTransceiver`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `type` | `str` |  |
| `serialNumber` | `str` |  |
| `frequency` | `str` | The frequency of the transceiver, e.g MF/LMF |
| `start` | `datetime` |  |
| `triggerDelay` | `float \| None` | The trigger delay in seconds |
| `delayIncludedInTWTT` | `bool \| None` | Whether the delay is included in the TWTT |
| `end` | `datetime.datetime \| None` | The end date of the transceiver usage |

## class `AcousticTransducer`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `type` | `str` |  |
| `serialNumber` | `str` |  |
| `frequency` | `str` |  |
| `start` | `datetime` |  |
| `end` | `datetime.datetime \| None` |  |

## class `AtdOffset`

The offset of the transducer from the GNSS antenna

**Fields**

| Name | Type | Description |
|---|---|---|
| `serialNumber` | `str \| None` | The transducer serial number associated with the offset |
| `x` | `float` | X offset: Relative position of the transducer to the GNSS antenna |
| `y` | `float` | Y offset: Relative position of the transducer to the GNSS antenna |
| `z` | `float` | Z offset: Relative position of the transducer to the GNSS antenna |

## class `EquipmentType`

_No docstring._

## class `GnssAntenna`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `type` | `str` |  |
| `serialNumber` | `str` |  |
| `start` | `datetime` |  |
| `order` | `str \| None` |  |
| `model` | `str \| None` |  |
| `radomeSerialNumber` | `str \| None` |  |
| `end` | `datetime.datetime \| None` |  |

## class `GnssReceiver`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `type` | `str` |  |
| `serialNumber` | `str` |  |
| `start` | `datetime` |  |
| `model` | `str \| None` | The model of the receiver |
| `firmwareVersion` | `str \| None` | The firmware version of the receiver |
| `end` | `datetime.datetime \| None` |  |

## class `ImuSensor`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `type` | `str` |  |
| `serialNumber` | `str` |  |
| `start` | `datetime` |  |
| `model` | `str \| None` |  |
| `end` | `datetime.datetime \| None` |  |

## class `Vessel`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `str` | The 4 digit name of the vessel |
| `type` | `str` | The type of the vessel. e.g. waveglider |
| `model` | `str` | The model of the vessel |
| `serialNumber` | `str \| None` |  |
| `start` | `datetime.datetime \| None` |  |
| `end` | `datetime.datetime \| None` |  |
| `imuSensors` | `list` |  |
| `atdOffsets` | `list` |  |
| `gnssAntennas` | `list` |  |
| `gnssReceivers` | `list` |  |
| `acousticTransducers` | `list` |  |
| `acousticTransceivers` | `list` |  |

**Methods**

### `Vessel.export_vessel(self, filepath: str)`

Export vessel data to a JSON file.

Parameters
----------
filepath : str
    The path to the JSON file.

### `Vessel.print_json(self)`

Print the vessel data as a JSON string.

### `Vessel.run_equipment(self, serial_number: str, equipment_type: earthscope_sfg_tools.datamodels.metadata.earthscope.vessel.EquipmentType, equipment_metadata: dict, add_new: bool = False, update: bool = False, delete: bool = False)`

Add, update, or delete a survey vessel equipment.

Parameters
----------
serial_number : str
    The serial number of the equipment.
equipment_type : EquipmentType
    The type of the equipment.
equipment_metadata : dict
    The metadata of the equipment.
add_new : bool, optional
    Whether to add a new equipment, by default False.
update : bool, optional
    Whether to update an existing equipment, by default False.
delete : bool, optional
    Whether to delete an existing equipment, by default False.


## `import_vessel(filepath: str) -> earthscope_sfg_tools.datamodels.metadata.earthscope.vessel.Vessel`

Import vessel data from a JSON file.

Parameters
----------
filepath : str
    The path to the JSON file.

Returns
-------
Vessel
    The vessel object.
