"""RINEX quality-control helpers."""

from pathlib import Path
import subprocess
import logging

from earthscope_sfg_tools.utils.go_runner import GoBinaryRunner

logger = logging.getLogger(__name__)

_runner = GoBinaryRunner("rnxqc", log=logger)


def rnxqc(
    input_file: str | Path,
) -> subprocess.CompletedProcess:
    """Perform RINEX quality-control checks using the Go utility."""
    input_file = Path(input_file)
    if not input_file.exists():
        raise FileNotFoundError(f"Input file {input_file} does not exist.")
    return _runner.run_raw(input_file)
