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
    """Run RINEX quality-control checks using the Go ``rnxqc`` binary.

    Args:
        input_file: Path to the RINEX file to inspect.

    Returns:
        The raw ``subprocess.CompletedProcess``; stdout/stderr contain
        the quality-control report produced by the binary.

    Raises:
        FileNotFoundError: If ``input_file`` does not exist.
    """
    input_file = Path(input_file)
    if not input_file.exists():
        raise FileNotFoundError(f"Input file {input_file} does not exist.")
    return _runner.run_raw(input_file)
