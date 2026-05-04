"""RINEX quality-control helpers.

This module intentionally re-exports :func:`rnxqc` from the NovAtel RINEX
operations module so there is a single implementation for invoking the QC
binary while preserving the historical import path.
"""

from pathlib import Path
import subprocess
import logging

from earthscope_sfg_tools.utils.go_utils import find_binary

logger = logging.getLogger(__name__)

def rnxqc(
    input_file: str | Path,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Perform RINEX quality-control checks using the Go utility."""
    binary = find_binary("rnxqc")
    input_file = Path(input_file)
    assert input_file.exists(), f"Input file {input_file} does not exist."
    cmd = [str(binary), str(input_file)]
    logger.info(f"Running rnxqc: {' '.join(cmd)}")
    return subprocess.run(cmd, capture_output=True, text=True, check=False)

