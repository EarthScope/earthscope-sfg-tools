# soundspeed_operations

`earthscope_sfg_tools.seafloor_site_tools.soundspeed_operations`

Functions for processing sound velocity profile (SVP) data.

## `CTD_to_svp_v1(source: 'str | Path') -> 'DataFrame[SoundVelocityDataFrame]'`

Load a comma-separated CTD SVP file and extend it via interpolation.

Negates depth values to convert from negative-down to positive-down
convention, then calls :func:`interpolate_svp` to append 200 m of
extrapolated speeds beyond the maximum measured depth.

Args:
    source: Path to the comma-separated file with ``depth`` and
        ``speed`` columns (no header).

Returns:
    Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
    ``speed`` (m/s) columns.

## `CTD_to_svp_v2(source: 'str | Path') -> 'DataFrame[SoundVelocityDataFrame]'`

Load a whitespace-separated CTD SVP file and extend it via interpolation.

Negates depth values to convert from negative-down to positive-down
convention. Adds a sub-micrometre random jitter to speed values to
prevent exact duplicates, then calls :func:`interpolate_svp` to
append 200 m of extrapolated speeds beyond the maximum measured depth.

Args:
    source: Path to the whitespace-delimited file with ``depth``
        and ``speed`` columns (no header).

Returns:
    Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
    ``speed`` (m/s) columns.

## `ctd_to_svp_v1(source: 'str | Path') -> 'DataFrame[SoundVelocityDataFrame]'`

Load a comma-separated CTD SVP file and extend it via interpolation.

Negates depth values to convert from negative-down to positive-down
convention, then calls :func:`interpolate_svp` to append 200 m of
extrapolated speeds beyond the maximum measured depth.

Args:
    source: Path to the comma-separated file with ``depth`` and
        ``speed`` columns (no header).

Returns:
    Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
    ``speed`` (m/s) columns.

## `ctd_to_svp_v2(source: 'str | Path') -> 'DataFrame[SoundVelocityDataFrame]'`

Load a whitespace-separated CTD SVP file and extend it via interpolation.

Negates depth values to convert from negative-down to positive-down
convention. Adds a sub-micrometre random jitter to speed values to
prevent exact duplicates, then calls :func:`interpolate_svp` to
append 200 m of extrapolated speeds beyond the maximum measured depth.

Args:
    source: Path to the whitespace-delimited file with ``depth``
        and ``speed`` columns (no header).

Returns:
    Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
    ``speed`` (m/s) columns.

## `interpolate_svp(svp: 'pd.DataFrame', additional_depth: 'float' = 200.0) -> 'pd.DataFrame'`

Extend a sound velocity profile by linear extrapolation.

Appends integer-metre depth rows from ``max_depth + 1`` to
``max_depth + additional_depth`` using :func:`numpy.interp`,
allowing acoustic ray-tracing to continue past the deepest
measured sample.

Args:
    svp: DataFrame with ``depth`` (m) and ``speed`` (m/s) columns.
    additional_depth: Metres to append beyond the current maximum
        depth. Defaults to 200.0.

Returns:
    Input DataFrame concatenated with the extrapolated rows,
    index reset to be contiguous.

## `parse_seabird_lines(lines: 'list[str]', logger: 'logging.Logger' = <Logger earthscope_sfg_tools.seafloor_site_tools.soundspeed_operations (WARNING)>) -> "'DataFrame[SoundVelocityDataFrame] | None'"`

Parse Seabird CTD SVP lines into a validated DataFrame.

Pure function — no filesystem access. Pass the output of
``f.readlines()`` or any list of strings.

Args:
    lines: Text lines from a Seabird SVP file, including the
        header section that ends with ``*END*``.
    logger: For info/error messages.

Returns:
    Validated ``SoundVelocityDataFrame`` with ``depth`` and
    ``speed`` columns, or ``None`` if no data rows follow
    ``*END*``.

## `seabird_to_soundvelocity(source: 'str | Path', logger: 'logging.Logger' = <Logger earthscope_sfg_tools.seafloor_site_tools.soundspeed_operations (WARNING)>) -> 'DataFrame[SoundVelocityDataFrame]'`

Parse a Seabird CTD sound velocity profile file into a DataFrame.

Thin I/O wrapper around :func:`parse_seabird_lines`.

Args:
    source: Path to the Seabird SVP file.
    logger: Logger for info/error messages.

Returns:
    Validated ``SoundVelocityDataFrame`` with ``depth`` (m) and
    ``speed`` (m/s) columns, or ``None`` if no data rows are
    found after ``*END*``.

Example:
    >>> df = seabird_to_soundvelocity("cast_001.cnv")
    >>> df["depth"].max()
    500.0
