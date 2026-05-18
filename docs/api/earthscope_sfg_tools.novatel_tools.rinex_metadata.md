# rinex_metadata

`earthscope_sfg_tools.novatel_tools.rinex_metadata`

Single-class metadata management for RINEX conversion workflows.

## class `RinexMetadata`

Validated, immutable metadata for any NovAtel-to-RINEX conversion.

Use ``RinexMetadata.load()`` as the single entry point regardless of
whether the source is a dict, a JSON file path, an existing instance,
or just a site code.  Call ``write()`` explicitly when a JSON file is
needed on disk (e.g. to pass to a Go binary via ``-settings``).

**Fields**

| Name | Type | Description |
|---|---|---|
| `marker_name` | `str` | Site name |
| `rinex_version` | `Union` |  |
| `rinex_type` | `Union` |  |
| `rinex_system` | `Union` |  |
| `marker_number` | `Union` |  |
| `marker_type` | `Union` |  |
| `observer` | `Union` |  |
| `agency` | `Union` |  |
| `program` | `Union` |  |
| `run_by` | `Union` |  |
| `date` | `Union` |  |
| `receiver_model` | `Union` |  |
| `receiver_serial` | `Union` |  |
| `receiver_firmware` | `Union` |  |
| `antenna_model` | `Union` |  |
| `antenna_serial` | `Union` |  |
| `antenna_position` | `Union` |  |
| `antenna_offsetHEN` | `Union` |  |

**Methods**

### `RinexMetadata.write(self, path: 'Path | str') -> 'Path'`

Serialise to JSON and return the resolved path.

