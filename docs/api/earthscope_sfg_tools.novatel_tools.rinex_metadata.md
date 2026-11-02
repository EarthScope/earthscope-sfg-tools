# rinex_metadata

`earthscope_sfg_tools.novatel_tools.rinex_metadata`

Single-class metadata management for RINEX conversion workflows.

## class `RinexMetadata`

Validated, immutable metadata for any NovAtel-to-RINEX conversion.

Use ``RinexMetadata.load()`` as the single entry point regardless of
whether the source is a dict, a JSON file path, an existing instance,
or just a site code.  Call ``write()`` explicitly when a JSON file is
needed on disk (e.g. to pass to a Go binary via ``-settings``).

Antenna types are serialized as a 20-character field: a model padded to
16 columns followed by a four-character radome. Legacy inputs such as
`NOV850 NONE` are normalized to `NOV850          NONE`; an omitted radome
becomes `NONE`. Existing correctly padded names and alternate radomes such
as `SCIT` are preserved. Oversized or ambiguous names are rejected rather
than silently truncated. This also applies to JSON settings passed directly
to `nova2rnx`, `novb2rnx`, `nov0002rnx`, and `tdb2rnx`.

**Fields**

| Name | Type | Description |
|---|---|---|
| `marker_name` | `str` | Site name |
| `rinex_version` | `str \| None` |  |
| `rinex_type` | `str \| None` |  |
| `rinex_system` | `str \| None` |  |
| `marker_number` | `str \| None` |  |
| `marker_type` | `str \| None` |  |
| `observer` | `str \| None` |  |
| `agency` | `str \| None` |  |
| `program` | `str \| None` |  |
| `run_by` | `str \| None` |  |
| `date` | `str \| None` |  |
| `receiver_model` | `str \| None` |  |
| `receiver_serial` | `str \| None` |  |
| `receiver_firmware` | `str \| None` |  |
| `antenna_model` | `str \| None` |  |
| `antenna_serial` | `str \| None` |  |
| `antenna_position` | `list[float] \| None` |  |
| `antenna_offsetHEN` | `list[float] \| None` |  |

**Methods**

### `RinexMetadata.write(self, path: 'Path | str') -> 'Path'`

Serialise to JSON and return the resolved path.

