# sv3_qc_operations

`earthscope_sfg_tools.sonardyne_tools.sv3_qc_operations`

_No docstring._

## `batch_qc_by_day(dataframes: 'list[pd.DataFrame]', date_column: 'str' = 'pingTime') -> 'dict[str, pd.DataFrame]'`

Group a list of shot-data DataFrames into per-day batches.

Converts the ``pingTime`` column (GPS seconds, stored as a float) to a UTC
date, groups rows by that date across all input DataFrames, and concatenates
them into a single DataFrame per day.

Args:
    dataframes: List of shot-data DataFrames, each expected to contain a
        column named ``date_column``.
    date_column: Name of the column holding the ping timestamp in GPS seconds.
        Defaults to ``'pingTime'``.

Returns:
    A dictionary mapping ISO-format date strings (``'YYYY-MM-DD'``) to
    concatenated DataFrames containing all shots from that day.

## `parse_qcjson_dict(raw: 'dict', logger: 'logging.Logger') -> "'DataFrame[GARPOSShotDataFrame] | None'"`

Parse a decoded QC JSON dict into a validated shot-data DataFrame.

Pure function: no filesystem access.  Pass the result of ``json.load()``.

Args:
    raw: Python dict decoded from a Sonardyne QC JSON file.
    logger: For missing-block and merge-failure messages.

Returns:
    Validated GARPOSShotDataFrame, or None if the interrogation block is
    missing or no valid range entries are found.

## `qcjson_to_shotdata(source: 'str | Path', logger: 'logging.Logger') -> "'DataFrame[GARPOSShotDataFrame] | None'"`

Parse a Sonardyne QC JSON file into a validated shot-data DataFrame.

Thin I/O wrapper around :func:`parse_qcjson_dict`.  Tries UTF-8 first,
falls back to latin-1.

Args:
    source: Path to the QC JSON file.
    logger: Logger instance for I/O errors and empty-result warnings.

Returns:
    A validated :class:`GARPOSShotDataFrame`, or ``None`` on read error or
    no valid pairs.
