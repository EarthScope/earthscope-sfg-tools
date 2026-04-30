"""Wrappers for compiled Go utilities for RINEX conversion and QC."""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import Optional
import logging

from ..utils.go_utils import BinaryNotFoundError, find_binary, parse_cli_logs  # noqa: F401

logger = logging.getLogger(__name__)


def _listify(value):
    if isinstance(value, list):
        return value
    return [value]


def nova2rnx(
    input_files: list[str] | str,
    settings_file: str | Path,
    output_dir: Optional[Path | str] = None,
    modulo: Optional[int] = None,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel ASCII logs to RINEX files.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel input file(s).
    settings_file : str or Path
        Path to JSON settings file for RINEX metadata.
    output_dir : Path or str, optional
        Directory to save the output files.
    modulo : int, optional
        Decimation modulo in milliseconds (e.g., 1000 for 1 Hz).

    Returns
    -------
    subprocess.CompletedProcess

    Raises
    ------
    BinaryNotFoundError
        If the nova2rnx binary cannot be found.
    """
    binary = find_binary("nova2rnx")

    settings_file = Path(settings_file)
    assert settings_file.exists(), f"Settings file {settings_file} does not exist."
    if output_dir is not None:
        output_dir = Path(output_dir)
        assert output_dir.exists(), f"Output directory {output_dir} does not exist."

    files = _listify(input_files)
    for f in files:
        assert Path(f).exists(), f"Input file {f} does not exist."

    cmd = [str(binary), "-settings", str(settings_file)]
    if modulo is not None:
        cmd.extend(["-modulo", str(modulo)])
    cmd.extend([str(f) for f in files])

    logger.info(f"Running nova2rnx: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False, cwd=output_dir)
    parse_cli_logs(result, logger)
    return result


def nov0002rnx(
    input_files: list[str] | str,
    settings_file: str | Path,
    output_dir: Optional[Path | str] = None,
    modulo: Optional[int] = None,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel NOV000 binary logs to RINEX files.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel binary input file(s).
    settings_file : str or Path
        Path to JSON settings file for RINEX metadata.
    output_dir : Path or str, optional
        Directory to save the output files.
    modulo : int, optional
        Decimation modulo in milliseconds.

    Returns
    -------
    subprocess.CompletedProcess

    Raises
    ------
    BinaryNotFoundError
        If the nov0002rnx binary cannot be found.
    """
    binary = find_binary("nov0002rnx")

    files = _listify(input_files)
    for f in files:
        assert Path(f).exists(), f"Input file {f} does not exist."
    settings_file = Path(settings_file)
    assert settings_file.exists(), f"Settings file {settings_file} does not exist."
    if output_dir is not None:
        output_dir = Path(output_dir)
        assert output_dir.exists(), f"Output directory {output_dir} does not exist."

    cmd = [str(binary), "-settings", str(settings_file)]
    if modulo is not None:
        cmd.extend(["-modulo", str(modulo)])
    cmd.extend([str(f) for f in files])

    logger.info(f"Running nov0002rnx: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False, cwd=output_dir)
    parse_cli_logs(result, logger)
    return result


def rnxqc(
    input_file: str,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Perform RINEX quality control checks.

    Parameters
    ----------
    input_file : str
        Path to RINEX file.

    Returns
    -------
    subprocess.CompletedProcess

    Raises
    ------
    BinaryNotFoundError
        If the rnxqc binary cannot be found.
    """
    binary = find_binary("rnxqc")
    cmd = [str(binary), input_file]
    logger.info(f"Running rnxqc: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    return result
