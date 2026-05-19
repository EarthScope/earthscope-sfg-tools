# utils

`earthscope_sfg_tools.datamodels.metadata.earthscope.utils`

_No docstring._

## class `AttributeUpdater`

_No docstring._

**Methods**

### `AttributeUpdater.set_value(self, key, value)`

Set the value of an attribute and update the class instance.

This also validates the updated instance.

Parameters
----------
key : str
    The key.
value : Any
    The value.

### `AttributeUpdater.update_attributes(self, additional_data: dict[str, typing.Any])`

Update the class attributes based on the provided dictionary.

This handles nested objects with the AttributeUpdater (e.g Location)
class. This class is helpful for the notebook where the user will be
passing empty strings if they don't want to update a field. This
function will only reset the value if not empty. If other keys are
provided in the dictionary, it will print a warning.

Parameters
----------
additional_data : Dict[str, Any]
    A dictionary of additional attributes to update.


## class `Location`

_No docstring._

**Fields**

| Name | Type | Description |
|---|---|---|
| `latitude` | `float \| None` | The latitude of the location. |
| `longitude` | `float \| None` | The longitude of the location. |
| `elevation` | `float \| None` | The elevation of the location. |

**Methods**

### `Location.set_value(self, key, value)`

Set the value of an attribute and update the class instance.

This also validates the updated instance.

Parameters
----------
key : str
    The key.
value : Any
    The value.

### `Location.update_attributes(self, additional_data: dict[str, typing.Any])`

Update the class attributes based on the provided dictionary.

This handles nested objects with the AttributeUpdater (e.g Location)
class. This class is helpful for the notebook where the user will be
passing empty strings if they don't want to update a field. This
function will only reset the value if not empty. If other keys are
provided in the dictionary, it will print a warning.

Parameters
----------
additional_data : Dict[str, Any]
    A dictionary of additional attributes to update.


## `check_dates(cls, end, values)`

Check that the end date is after the start date.

Parameters
----------
cls : class
    The class.
end : datetime
    The end date.
values : dict
    The values.

Returns
-------
datetime
    The end date.

## `check_fields_for_empty_strings(cls, value)`

Check if the field is an empty string and replace it with None.

Parameters
----------
cls : class
    The class.
value : str
    The value.

Returns
-------
str | None
    The value or None.

## `convert_custom_objects_to_dict(d: dict) -> dict`

Recursively convert custom objects in a dictionary to dictionaries.

Parameters
----------
d : dict
    The dictionary to update.

Returns
-------
dict
    The updated dictionary with custom objects converted to
    dictionaries.

## `convert_to_datetime(date_str: str | datetime.datetime) -> datetime.datetime`

Convert ISO string format to datetime if a string is provided.

Parameters
----------
date_str : Union[str, datetime]
    The date string or datetime object to convert.

Returns
-------
datetime
    The converted datetime object, always timezone-aware in UTC.

Raises
------
ValueError
    If the date string is not in a valid ISO format.

## `if_zero_than_none(cls, value)`

If the value is 0, return None.

Parameters
----------
cls : class
    The class.
value : int
    The value.

Returns
-------
int | None
    The value or None.

## `only_one_is_true(*args)`

Check that only one of the arguments is True.

Parameters
----------
*args
    The arguments to check.

Returns
-------
bool
    True if only one of the arguments is True, False otherwise.

## `parse_datetime(cls, value)`

Parse a datetime string.

Parameters
----------
cls : class
    The class.
value : str
    The datetime string.

Returns
-------
datetime
    The parsed datetime.
