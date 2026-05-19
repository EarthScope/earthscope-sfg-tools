# novatel_to_rinex_operations

`earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations`

_No docstring._

## `nov0002rnx(input_files: list[str] | str, settings_file: str | pathlib.Path, output_dir: pathlib.Path | str | None = None, modulo: int | None = None, logger: logging.Logger = <Logger ES_SFGTools.NovatelToRinex (INFO)>) -> subprocess.CompletedProcess`

Convert NovAtel NOV000 binary logs to RINEX files using the Go utility.

## `novatel_binary_2rinex(files: list[pathlib.Path] | list[str] | str | pathlib.Path, writedir: pathlib.Path | str | None = None, site: str | None = None, metadata: dict | earthscope_sfg_tools.novatel_tools.rinex_metadata.RinexMetadata | pathlib.Path | str | None = None, modulo_millis: int = 0, num_routines: int = 1, antindex: int = 0, **kwargs) -> list[pathlib.Path]`

Convert NovAtel NOV000 / NOV770 binary files to daily RINEX.
