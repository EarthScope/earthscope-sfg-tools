# quality_control

`earthscope_sfg_tools.rinex_tools.quality_control`

RINEX quality-control helpers.

## `rnxqc(input_file: str | pathlib.Path) -> subprocess.CompletedProcess`

Run RINEX quality-control checks using the Go ``rnxqc`` binary.

Args:
    input_file: Path to the RINEX file to inspect.

Returns:
    The raw ``subprocess.CompletedProcess``; stdout/stderr contain
    the quality-control report produced by the binary.

Raises:
    FileNotFoundError: If ``input_file`` does not exist.
