# site

`earthscope_sfg_tools.datamodels.metadata.earthscope.site`

_No docstring._

## class `ReferenceFrame`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `str` | The name of the reference frame |
| `start` | `Union` | The start date of the reference frame used for the site |
| `end` | `Union` | The end date of the reference frame used for the site |

## class `Site`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `names` | `list` | The names of the site, including the 4 character ID |
| `networks` | `list` | A list networks the site is part of |
| `timeOrigin` | `datetime` | The time origin of the site |
| `localGeoidHeight` | `Union` | The local geoid height of the site |
| `arrayCenter` | `Union` | The array center of the site |
| `campaigns` | `list` | The campaigns associated with the site |
| `benchmarks` | `list` | The benchmarks associated with the site |
| `referenceFrames` | `list` | The reference frames used for the site |

**Methods**

### `Site.export_site(self, filepath: str)`

_No docstring._

### `Site.print_json(self)`

_No docstring._

### `Site.return_tats_for_campaign(self, campaign_name: str) -> list[dict[str, Any]] | None`

Return all TATs for a given campaign

Args:
    campaign_name (str): The name of the campaign
Returns:
    List[Dict[str, Any]]: A list of dictionaries containing Benchmark name, Transponder address, and TAT

### `Site.run_component(self, component_type: earthscope_sfg_tools.datamodels.metadata.earthscope.site.TopLevelSiteGroups, component_metadata: dict, add_new: bool = False, update: bool = False, delete: bool = False)`

Generic add, update or delete equipment for the site

### `Site.run_sub_component(self, component_type: earthscope_sfg_tools.datamodels.metadata.earthscope.site.TopLevelSiteGroups, component_name: str, sub_component_type: earthscope_sfg_tools.datamodels.metadata.earthscope.site.SubLevelSiteGroups, sub_component_metadata: dict, add_new: bool = False, update: bool = False, delete: bool = False)`

Generic add, update or delete sub-components (e.g Transponder attached to Benchmark, Survey to campaign)
for the site.

### `Site.validate_components(self)`

If there are no benchmarks, transponders, campaigns, or surveys, print a warning.


## class `SubLevelSiteGroups`

_No docstring._

## class `TopLevelSiteGroups`

_No docstring._

## `import_site(filepath: str)`

Import site data from a JSON file.
