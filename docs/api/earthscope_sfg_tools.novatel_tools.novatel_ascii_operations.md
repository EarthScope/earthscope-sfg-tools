# novatel_ascii_operations

`earthscope_sfg_tools.novatel_tools.novatel_ascii_operations`

_No docstring._

## `nova2rnx(input_files: list[str] | str, settings_file: str | pathlib.Path, output_dir: pathlib.Path | str | None = None, modulo: int | None = None, logger: logging.Logger = <Logger earthscope_sfg_tools.novatel_tools.novatel_ascii_operations (WARNING)>) -> subprocess.CompletedProcess`

Convert NovAtel ASCII logs to RINEX files using the Go utility.

## `novatel_ascii_2rinex(files: list[pathlib.Path | str] | pathlib.Path | str, writedir: pathlib.Path = None, site: str = 'SIT1', metadata: dict | earthscope_sfg_tools.novatel_tools.rinex_metadata.RinexMetadata | pathlib.Path | str = None, modulo_millis: int = 0, logger: logging.Logger = <Logger earthscope_sfg_tools.novatel_tools.novatel_ascii_operations (WARNING)>, *kwargs) -> list[pathlib.Path]`

Convert a NovAtel ASCII file to a daily RINEX file using nova2rnxo.
This function wraps the external `nova2rnxo` binary to convert a NovAtel ASCII
observation file into a daily RINEX file. Metadata describing the site and
receiver can be supplied directly or generated automatically from a site code.
If `metadata` is a mapping or `MetadataModel`, it is validated and written
to a JSON file in `writedir`. If it is a path (or string path), it is checked
for existence and validated as JSON metadata. If `metadata` is not provided,
a metadata JSON is generated using `get_metadatav2` with the given `site`
code and a random serial number.
The resulting RINEX file produced by `nova2rnxo` is moved from a temporary
working directory into `writedir` and its final path is returned.

Parameters
----------
files : list[pathlib.Path or str] or pathlib.Path or str
    Paths to the input NovAtel ASCII files to convert.
writedir : pathlib.Path or str, optional
    Directory where the output RINEX (and metadata JSON, if created) will
    be written. Defaults to the parent directory of the first file in `files`.
site : str, optional
    Four-character site code used when generating metadata automatically,
    required if `metadata` is not provided. Must be exactly 4 characters.
metadata : dict, MetadataModel, pathlib.Path or str, optional
    Site/receiver metadata. May be:
      * A dict containing metadata fields compatible with `MetadataModel`.
      * A `MetadataModel` instance.
      * A path (or string path) to a JSON file with metadata.
    If provided, it is validated against `MetadataModel`. If provided as a
    dict or `MetadataModel`, a JSON metadata file is written to `writedir`.
modulo_millis : int, optional
    Decimation modulo in milliseconds (e.g., 1000 for 1 Hz, 15000 for 15s
    intervals). If 0, no decimation is applied. Loss-of-lock indicators
    from skipped epochs are propagated to the next written epoch. Default
    is 0 (no decimation).
**kwargs
    Currently unused; accepted for future extensibility.

Returns
-------
List[pathlib.Path]
    Paths to the generated daily RINEX files in `writedir`.

Raises
------
AssertionError
    If `metadata` is None and `site` is missing, not a string, or not
    exactly 4 characters long, or if `metadata` is not a path by the time
    it is passed to `nova2rnxo`.
ValueError
    If the metadata dictionary, model, or JSON file fails validation
    against `MetadataModel`, or if `metadata` is of an unsupported type.
FileNotFoundError
    If the provided metadata file path does not exist.
subprocess.CalledProcessError
    If the `nova2rnxo` subprocess fails.

## `qcpin_to_novatelpin(source: str | pathlib.Path, writedir: pathlib.Path) -> pathlib.Path`

Convert a QCPIN JSON file to a Novatel PIN text file.
Args:
    source (str|Path): Path to the QCPIN JSON file.
    writedir (Path): Directory where the Novatel PIN text file will be saved.
Returns:
    Path: Path to the generated Novatel PIN text file.
