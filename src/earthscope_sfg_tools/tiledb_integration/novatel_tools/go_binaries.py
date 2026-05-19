"""TileDB-specific Go binary wrappers for NovAtel data ingestion and export."""

from __future__ import annotations

import subprocess
import logging
from typing import Optional

from ...utils.go_utils import BinaryNotFoundError, find_binary, parse_cli_logs  # noqa: F401
from ...utils.misc import listify

logger = logging.getLogger(__name__)


def nova2tile(
    input_files: list[str] | str,
    tdb_path: str,
    num_procs: int = 10,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel ASCII logs to a TileDB array.

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
    """
    binary = find_binary()

    files = [str(f) for f in listify(input_files)]
    cmd = [str(binary), "nova2tile", "--tdb", str(tdb_path), "--procs", str(num_procs)]
    cmd.extend(files)

    logger.info(f"Running sfg nova2tile: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    parse_cli_logs(result, logger)
    return result


def novb2tile(
    input_files: list[str] | str,
    tdb_path: str,
    num_procs: int = 10,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel NOV770 binary logs to a TileDB array.

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
    """
    binary = find_binary()

    files = [str(f) for f in listify(input_files)]
    cmd = [str(binary), "novab2tile", "--tdb", str(tdb_path), "--procs", str(num_procs)]
    cmd.extend(files)

    logger.info(f"Running sfg novab2tile: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    parse_cli_logs(result, logger)
    return result


def nov0002tile(
    input_files: list[str] | str,
    tdb_path: str,
    tdb_position: Optional[str] = None,
    num_procs: int = 10,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel NOV000 binary logs to a TileDB array.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel NOV000 (``.bin``) input file(s).
    tdb_path : str
        S3 or local path to the target TileDB GNSS observation array.
    tdb_position : str, optional
        Path to a secondary TileDB array for INS / position records
        (``--tdbpos``). When ``None`` the flag is omitted.
    num_procs : int, optional
        Number of parallel goroutines (``--procs``). Defaults to 10.

    Returns
    -------
    subprocess.CompletedProcess

    Raises
    ------
    BinaryNotFoundError
        If the nov0002tile binary cannot be found.
    """
    binary = find_binary()

    files = [str(f) for f in listify(input_files)]
    cmd = [
        str(binary),
        "nov0002tile",
        "--tdb",
        str(tdb_path),
        "--procs",
        str(num_procs),
    ]
    if tdb_position:
        cmd.extend(["--tdbpos", str(tdb_position)])
    cmd.extend(files)

    logger.info(f"Running sfg nov0002tile: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    parse_cli_logs(result, logger)
    return result


def tdb2rnx(
    tdb_path: str,
    settings_file: str,
    time_interval: int = 1,
    processing_year: int = 0,
    modulo_millis: int = 0,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert a TileDB GNSS observation array to RINEX files.

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
    """
    binary = find_binary()

    cmd = [
        str(binary),
        "tdb2rnx",
        "--tdb",
        str(tdb_path),
        "--settings",
        str(settings_file),
        "--timeint",
        str(time_interval),
        "--year",
        str(processing_year),
    ]
    if modulo_millis > 0:
        cmd.extend(["--modulo", str(modulo_millis)])

    logger.info(f"Running sfg tdb2rnx: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    parse_cli_logs(result, logger)
    return result
