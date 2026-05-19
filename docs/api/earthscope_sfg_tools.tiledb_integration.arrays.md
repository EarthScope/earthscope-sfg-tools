# arrays

`earthscope_sfg_tools.tiledb_integration.arrays`

TileDB array base class and typed subclasses for seafloor geodesy data.

## class `TBDArray`

A base class for interacting with a TileDB array.

This class provides common functionality for creating, reading, and writing
pandas DataFrames to and from a TileDB array. It is intended to be subclassed
for specific data types and schemas.

Attributes:
    dataframe_schema: A pandera schema for validating DataFrames.
    array_schema: A tiledb.ArraySchema for creating the array.
    name (str): A human-readable name for the array type.
    uri (str): The URI of the TileDB array.

**Methods**

### `TBDArray.consolidate(self)`

Consolidates and vacuums the TileDB array to improve performance.

### `TBDArray.get_unique_dates(self, field: str) -> numpy.ndarray`

Gets the unique dates from a specified datetime field in the array.

Args:
    field (str): The name of the datetime field to query.

Returns:
    np.ndarray: An array of unique dates, or None if an error occurs.

### `TBDArray.read_df(self, start: datetime.datetime | numpy.datetime64, end: datetime.datetime | numpy.datetime64 = None, validate: bool = True, **kwargs) -> pandas.DataFrame`

Read a DataFrame from the array between a start and end date.

Args:
    start (datetime.datetime | np.datetime64): The start date for the
        data slice.
    end (datetime.datetime | np.datetime64, optional): The end date for
        the data slice. If None, defaults to one day after start.
        Defaults to None.
    validate (bool, optional): Whether to validate the returned
        DataFrame. Defaults to True.

Returns:
    pd.DataFrame: A DataFrame containing the data for the specified
    date range. Returns an empty DataFrame if no data is found or
    on error.

### `TBDArray.view(self, network: str = '', station: str = '')`

Generates a plot showing the dates for which data is available.

Args:
    network (str, optional): Network name to display in the title.
        Defaults to "".
    station (str, optional): Station name to display in the title.
        Defaults to "".

Raises:
    ValueError: If no data is found in the array.

### `TBDArray.write_df(self, df: pandas.DataFrame, validate: bool = True)`

Write a pandas DataFrame to the array.

The DataFrame is validated against the class's `dataframe_schema`
before being written.

Args:
    df (pd.DataFrame): The DataFrame to write.
    validate (bool, optional): Whether to validate the DataFrame.
        Defaults to True.


## class `TDBAcousticArray`

Handles TileDB storage for acoustic ranging data.

**Methods**

### `TDBAcousticArray.consolidate(self)`

Consolidates and vacuums the TileDB array to improve performance.

### `TDBAcousticArray.get_unique_dates(self, field='triggerTime') -> numpy.ndarray`

Gets unique dates from the 'triggerTime' field.

### `TDBAcousticArray.read_df(self, start: <module 'datetime' from '/Users/terry/repos/earthscope-sfg-tools/.pixi/envs/tiledb/lib/python3.14/datetime.py'>, end: <module 'datetime' from '/Users/terry/repos/earthscope-sfg-tools/.pixi/envs/tiledb/lib/python3.14/datetime.py'> = None, **kwargs) -> pandas.DataFrame`

Reads acoustic data for a given time range.

### `TDBAcousticArray.view(self, network: str = '', station: str = '')`

Generates a plot showing the dates for which data is available.

Args:
    network (str, optional): Network name to display in the title.
        Defaults to "".
    station (str, optional): Station name to display in the title.
        Defaults to "".

Raises:
    ValueError: If no data is found in the array.

### `TDBAcousticArray.write_df(self, df: pandas.DataFrame)`

Writes an acoustic data DataFrame to the array.


## class `TDBGNSSObsArray`

Handles TileDB storage for GNSS observation data.

**Methods**

### `TDBGNSSObsArray.consolidate(self)`

Consolidates and vacuums the TileDB array to improve performance.

### `TDBGNSSObsArray.get_unique_dates(self, field: str = 'time') -> numpy.ndarray`

Gets unique dates from a specified datetime field in the array.

Args:
    field (str, optional): The name of the datetime field to query.
        Defaults to "time".

Returns:
    np.ndarray: An array of unique dates, or None if an error occurs.

### `TDBGNSSObsArray.read_df(self, start: datetime.datetime | numpy.datetime64, end: datetime.datetime | numpy.datetime64 = None, validate: bool = True, **kwargs) -> pandas.DataFrame`

Read a DataFrame from the array between a start and end date.

Args:
    start (datetime.datetime | np.datetime64): The start date for the
        data slice.
    end (datetime.datetime | np.datetime64, optional): The end date for
        the data slice. If None, defaults to one day after start.
        Defaults to None.
    validate (bool, optional): Whether to validate the returned
        DataFrame. Defaults to True.

Returns:
    pd.DataFrame: A DataFrame containing the data for the specified
    date range. Returns an empty DataFrame if no data is found or
    on error.

### `TDBGNSSObsArray.view(self, network: str = '', station: str = '')`

Generates a plot showing the dates for which data is available.

Args:
    network (str, optional): Network name to display in the title.
        Defaults to "".
    station (str, optional): Station name to display in the title.
        Defaults to "".

Raises:
    ValueError: If no data is found in the array.

### `TDBGNSSObsArray.write_df(self, df: pandas.DataFrame, validate: bool = True)`

Write a pandas DataFrame to the array.

The DataFrame is validated against the class's `dataframe_schema`
before being written.

Args:
    df (pd.DataFrame): The DataFrame to write.
    validate (bool, optional): Whether to validate the DataFrame.
        Defaults to True.

### `TDBGNSSObsArray.write_epochs(self, epochs: list[earthscope_sfg_tools.novatel_tools.rangea_parser.GNSSEpoch], region: str = 'us-east-2') -> int`

Write GNSS observation epochs to this TileDB array.

This method is the Python equivalent of the Go WriteObsV3Array function.
It flattens the hierarchical epoch/satellite/observation structure into
columnar buffers and writes them to the TileDB array.

Array Schema (dimensions):
    - time (int64): UTC timestamp in milliseconds since Unix epoch
    - sys (uint8): GNSS system identifier
    - sat (uint8): Satellite PRN/slot number
    - obs (uint16): Observation type code (2-char code as uint16)

Array Schema (attributes):
    - range (float64): Pseudorange measurement in meters
    - phase (float64): Carrier phase in cycles
    - doppler (float64): Doppler frequency in Hz
    - snr (float32): Signal-to-noise ratio in dB-Hz
    - slip (uint16): Lock time / slip counter
    - flags (uint16): Observation flags
    - fcn (int8): GLONASS frequency channel number

### `TDBGNSSObsArray.write_rangea_strings(self, rangea_strings: list[str], verbose: bool = False) -> int`

Write GNSS observation epochs to this TileDB array from RINEX 3.05
observation file lines.

Parameters
----------
    rangea_strings (List[str])
        A list of strings
    verbose (bool, optional)
        Whether to print verbose output during processing. Defaults to False.


## class `TDBIMUPositionArray`

Handles TileDB storage for IMU position and orientation data.

**Methods**

### `TDBIMUPositionArray.consolidate(self)`

Consolidates and vacuums the TileDB array to improve performance.

### `TDBIMUPositionArray.get_unique_dates(self, field='time') -> numpy.ndarray`

Gets unique dates from the 'time' field.

### `TDBIMUPositionArray.read_df(self, start: datetime.datetime | numpy.datetime64, end: datetime.datetime | numpy.datetime64 = None, validate: bool = True, **kwargs) -> pandas.DataFrame`

Read a DataFrame from the array between a start and end date.

Args:
    start (datetime.datetime | np.datetime64): The start date for the
        data slice.
    end (datetime.datetime | np.datetime64, optional): The end date for
        the data slice. If None, defaults to one day after start.
        Defaults to None.
    validate (bool, optional): Whether to validate the returned
        DataFrame. Defaults to True.

Returns:
    pd.DataFrame: A DataFrame containing the data for the specified
    date range. Returns an empty DataFrame if no data is found or
    on error.

### `TDBIMUPositionArray.view(self, network: str = '', station: str = '')`

Generates a plot showing the dates for which data is available.

Args:
    network (str, optional): Network name to display in the title.
        Defaults to "".
    station (str, optional): Station name to display in the title.
        Defaults to "".

Raises:
    ValueError: If no data is found in the array.

### `TDBIMUPositionArray.write_df(self, df: pandas.DataFrame, validate: bool = True)`

Write a pandas DataFrame to the array.

The DataFrame is validated against the class's `dataframe_schema`
before being written.

Args:
    df (pd.DataFrame): The DataFrame to write.
    validate (bool, optional): Whether to validate the DataFrame.
        Defaults to True.


## class `TDBKinPositionArray`

Handles TileDB storage for kinematic GNSS position data.

**Methods**

### `TDBKinPositionArray.consolidate(self)`

Consolidates and vacuums the TileDB array to improve performance.

### `TDBKinPositionArray.get_unique_dates(self, field='time') -> numpy.ndarray`

Gets unique dates from the 'time' field.

### `TDBKinPositionArray.read_df(self, start: datetime.datetime | numpy.datetime64, end: datetime.datetime | numpy.datetime64 = None, validate: bool = True, **kwargs) -> pandas.DataFrame`

Read a DataFrame from the array between a start and end date.

Args:
    start (datetime.datetime | np.datetime64): The start date for the
        data slice.
    end (datetime.datetime | np.datetime64, optional): The end date for
        the data slice. If None, defaults to one day after start.
        Defaults to None.
    validate (bool, optional): Whether to validate the returned
        DataFrame. Defaults to True.

Returns:
    pd.DataFrame: A DataFrame containing the data for the specified
    date range. Returns an empty DataFrame if no data is found or
    on error.

### `TDBKinPositionArray.view(self, network: str = '', station: str = '')`

Generates a plot showing the dates for which data is available.

Args:
    network (str, optional): Network name to display in the title.
        Defaults to "".
    station (str, optional): Station name to display in the title.
        Defaults to "".

Raises:
    ValueError: If no data is found in the array.

### `TDBKinPositionArray.write_df(self, df: pandas.DataFrame, validate: bool = True)`

Write a pandas DataFrame to the array.

The DataFrame is validated against the class's `dataframe_schema`
before being written.

Args:
    df (pd.DataFrame): The DataFrame to write.
    validate (bool, optional): Whether to validate the DataFrame.
        Defaults to True.


## class `TDBShotDataArray`

Handles TileDB storage for processed shot data.

**Methods**

### `TDBShotDataArray.consolidate(self)`

Consolidates and vacuums the TileDB array to improve performance.

### `TDBShotDataArray.get_unique_dates(self, field='pingTime') -> numpy.ndarray`

Gets unique dates from the 'pingTime' field.

### `TDBShotDataArray.read_df(self, start: <module 'datetime' from '/Users/terry/repos/earthscope-sfg-tools/.pixi/envs/tiledb/lib/python3.14/datetime.py'>, end: <module 'datetime' from '/Users/terry/repos/earthscope-sfg-tools/.pixi/envs/tiledb/lib/python3.14/datetime.py'> = None, **kwargs) -> pandas.DataFrame`

Read a DataFrame from the array between the start and end dates.

Args:
    start (datetime.datetime): The start date.
    end (datetime.datetime, optional): The end date. Defaults to None.

Returns:
    pd.DataFrame: A DataFrame of shot data, or None on error.

### `TDBShotDataArray.view(self, network: str = '', station: str = '')`

Generates a plot showing the dates for which data is available.

Args:
    network (str, optional): Network name to display in the title.
        Defaults to "".
    station (str, optional): Station name to display in the title.
        Defaults to "".

Raises:
    ValueError: If no data is found in the array.

### `TDBShotDataArray.write_df(self, df: pandas.DataFrame, validate: bool = True)`

Write a shot data DataFrame to the array.

Handles conversion of timestamp columns from float or datetime objects
to the required nanosecond-precision numpy datetime64 format.

Args:
    df (pd.DataFrame): The dataframe to write.
    validate (bool, optional): Whether to validate the dataframe.
        Defaults to True.


## `as_py_datetime_object_col(s: pandas.Series) -> pandas.Series`

Convert a pandas Series of datetime-like objects to Python datetime objects.

Parameters
----------
    s (pd.Series): A pandas Series containing datetime-like objects.

Returns
-------
    pd.Series: A pandas Series with Python datetime objects.
