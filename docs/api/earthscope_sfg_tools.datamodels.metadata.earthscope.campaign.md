# campaign

`earthscope_sfg_tools.datamodels.metadata.earthscope.campaign`

_No docstring._

## class `Campaign`

Represents a campaign, which is a collection of surveys.

**Fields**

| Name | Type | Description |
|---|---|---|
| `name` | `str` | The name of the campaign in the format YYYY_A_VVVV |
| `type` | `str` | The type of the campaign (deploy \| measure \| etc) |
| `vesselCode` | `str` | The 4 digit vessel code, associated with a vessel metadata file |
| `start` | `datetime` | The start date & time of the campaign |
| `end` | `datetime` | The end date & time of the campaign |
| `vessel` | `Union` | Instatiate Vessel object |
| `principalInvestigator` | `Union` |  |
| `launchVesselName` | `Union` |  |
| `recoveryVesselName` | `Union` |  |
| `cruiseName` | `Union` |  |
| `technicianName` | `Union` |  |
| `technicianContact` | `Union` |  |
| `surveys` | `list` |  |

**Methods**

### `Campaign.check_survey_times(self)`

Checks that survey times within the campaign do not overlap.

Raises
------
ValueError
    If any survey times overlap.

### `Campaign.get_survey_by_datetime(self, dt: datetime.datetime) -> earthscope_sfg_tools.datamodels.metadata.earthscope.campaign.Survey`

Returns the survey that encompasses the given datetime.

Parameters
----------
dt : datetime
    The datetime to check against surveys.

Returns
-------
Survey
    The Survey object that contains the given datetime.

Raises
------
ValueError
    If no survey is found for the given datetime.


## class `Survey`

Represents a single survey within a campaign.

**Fields**

| Name | Type | Description |
|---|---|---|
| `id` | `str` | The unique ID of the survey |
| `type` | `Union` | The type of the survey (e.g. circle \| fixed point \| mixed) |
| `benchmarkIDs` | `list` | Benchmark IDs associated with the survey |
| `start` | `datetime` | The start date & time of the survey |
| `end` | `datetime` | The end date & time of the survey |
| `notes` | `Union` | Any additional notes about the survey |
| `commands` | `Union` | Log of commands |

## class `SurveyType`

_No docstring._

## `campaign_checks(campaign_year, campaign_interval, vessel_code)`

Checks the campaign year, interval, and vessel code for validity.

Parameters
----------
campaign_year : str
    The campaign year (e.g., "2023").
campaign_interval : str
    The campaign interval (e.g., "A").
vessel_code : str
    The 4-character vessel code.

Returns
-------
Tuple[str, str]
    A tuple containing the formatted campaign name and the uppercase
    vessel code.

Raises
------
ValueError
    If the campaign year, interval, or vessel code are invalid.

## `classify_survey_type(survey_type: str) -> earthscope_sfg_tools.datamodels.metadata.earthscope.campaign.SurveyType`

Classifies the survey type based on the provided string.

Parameters
----------
survey_type : str
    The survey type as a string.

Returns
-------
SurveyType
    The classified SurveyType.

Raises
------
ValueError
    If the survey type is not recognized.
