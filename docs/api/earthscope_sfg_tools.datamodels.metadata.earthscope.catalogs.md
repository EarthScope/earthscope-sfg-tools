# catalogs

`earthscope_sfg_tools.datamodels.metadata.earthscope.catalogs`

_No docstring._

## class `CatalogType`

_No docstring._

## class `MetaDataCatalog`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `Union` | The catalog name |
| `networks` | `dict` | Network catalog |
| `info` | `Union` | Optional catalog meta |
| `type` | `CatalogType` | Catalog Type (meta-data or data) |

**Methods**

### `MetaDataCatalog.serialize_type(self, value: earthscope_sfg_tools.datamodels.metadata.earthscope.catalogs.CatalogType) -> str`

_No docstring._

### `MetaDataCatalog.show(self)`

Displays an abridged structured representation of the catalog data.

This is displayed in JSON format.

The method organizes the data into a nested dictionary structure. The
output format depends on the catalog type:
- For "Meta-Data", it includes networks, stations, campaigns, and
  surveys.
- For "Data", it includes networks, stations, and their shotdata.

The resulting dictionary is serialized into a JSON string and printed
with indentation for readability.

Raises
------
AttributeError
    If the object structure does not match the expected attributes.

Examples
--------
>>> catalog_dir = Path("/path/to/catalog/directory")
>>> DATA = Catalog.load_metadata(data_path,name="sfg metadata",info="metadata for sfg")
{
"alaska-shumagins": {
    "IVB1": {
    "name": "2022_A_1049",
    "start": "2022-07-17T13:42:19.870000",
    "end": "2022-07-24T11:18:33.870000",
    "surveys": [
        {
        "survey_id": "2022_A_1049_1",
        "start": "2022-07-17T13:42:19.870000",
        "end": "2022-07-18T11:33:33.870000"
        },
        {
        "survey_id": "2022_A_1049_2",
        "start": "2022-07-18T13:42:19.870000",
        "end": "2022-07-21T11:18:33.870000"
        }
    ]
    },
}
>>>
Outputs the JSON representation of the catalog data to the console.


## class `NetworkData`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `str` | The network name |
| `stations` | `dict` | Stations in the network |

## class `StationData`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `str` | The station's name |
| `shotdata` | `str` | The station's shotdata TileDB URI |
| `shotdata_pre` | `str` | Pre-update shotdata |
| `kinpositiondata` | `str` | The station's RINEX derived position TileDB URI |
| `gnssobsdata` | `str` | The station's raw gnss observables TileDB URI |
| `gnssobsdata_secondary` | `str` | The station's secondary gnss observables TileDB URI |
| `imupositiondata` | `str` | The station's position data TileDB URI |
| `acousticdata` | `str` | The station's acoustic data TileDB URI |
