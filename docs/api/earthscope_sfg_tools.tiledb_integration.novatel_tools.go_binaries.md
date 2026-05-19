# go_binaries

`earthscope_sfg_tools.tiledb_integration.novatel_tools.go_binaries`

TileDB-specific Go binary wrappers for NovAtel data ingestion and export.

## `nov0002tile(input_files: 'list[str] | str', tdb_path: 'str', logger: 'logging.Logger' = <Logger earthscope_sfg_tools.tiledb_integration.novatel_tools.go_binaries (WARNING)>) -> 'subprocess.CompletedProcess'`

Convert NovAtel NOV000 binary logs to a TileDB array.

Parameters
----------
input_files : str or list[str]
    NovAtel NOV000 (``.bin``) input file(s).
tdb_path : str
    S3 or local path to the target TileDB array.

Returns
-------
subprocess.CompletedProcess

Raises
------
BinaryNotFoundError
    If the nov0002tile binary cannot be found.

## `nova2tile(input_files: 'list[str] | str', tdb_path: 'str', num_procs: 'int' = 10, logger: 'logging.Logger' = <Logger earthscope_sfg_tools.tiledb_integration.novatel_tools.go_binaries (WARNING)>) -> 'subprocess.CompletedProcess'`

Convert NovAtel ASCII logs to a TileDB array.

Parameters
----------
input_files : str or list[str]
    NovAtel ASCII input file(s).
tdb_path : str
    S3 or local path to the target TileDB array.
num_procs : int, optional
    Number of parallel goroutines. Defaults to 10.

Returns
-------
subprocess.CompletedProcess

Raises
------
BinaryNotFoundError
    If the nova2tile binary cannot be found.

## `novb2tile(input_files: 'list[str] | str', tdb_path: 'str', num_procs: 'int' = 10, logger: 'logging.Logger' = <Logger earthscope_sfg_tools.tiledb_integration.novatel_tools.go_binaries (WARNING)>) -> 'subprocess.CompletedProcess'`

Convert NovAtel NOV770 binary logs to a TileDB array.

Parameters
----------
input_files : str or list[str]
    NovAtel NOV770 (``.raw``) input file(s).
tdb_path : str
    S3 or local path to the target TileDB array.
num_procs : int, optional
    Number of parallel goroutines. Defaults to 10.

Returns
-------
subprocess.CompletedProcess

Raises
------
BinaryNotFoundError
    If the novb2tile binary cannot be found.

## `tdb2rnx(tdb_path: 'str', settings_file: 'str', time_interval: 'int' = 1, processing_year: 'int' = 0, modulo_millis: 'int' = 0, logger: 'logging.Logger' = <Logger earthscope_sfg_tools.tiledb_integration.novatel_tools.go_binaries (WARNING)>) -> 'subprocess.CompletedProcess'`

Convert a TileDB GNSS observation array to RINEX files.

Parameters
----------
tdb_path : str
    S3 or local path to the TileDB array.
settings_file : str
    Path to JSON settings file for RINEX metadata.
time_interval : int, optional
    Hours of data loaded per query batch. Defaults to 1.
processing_year : int, optional
    If non-zero, restrict output to this year only.
modulo_millis : int, optional
    Decimation interval in milliseconds (0 = no decimation).

Returns
-------
subprocess.CompletedProcess

Raises
------
BinaryNotFoundError
    If the tdb2rnx binary cannot be found.
