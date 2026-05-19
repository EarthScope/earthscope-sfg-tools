"""Python wrappers around ``sfg crinex {compress,decompress}``.

Thin shims over the bundled Go binary that handle Hatanaka CRINEX
compression / decompression of RINEX observation files. See
``go/cmd/compress_rinex.go`` for the underlying CLI.
"""

from __future__ import annotations

import logging
import subprocess
from pathlib import Path

from ..utils.go_utils import find_binary, parse_cli_logs

logger = logging.getLogger(__name__)


def crinex_compress(
    input_path: str | Path,
    output_path: str | Path,
    gzip: bool = True,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Compress a RINEX observation file to CRINEX (Hatanaka).

    Parameters
    ----------
    input_path : str or Path
        Source RINEX file. ``.gz`` inputs are auto-detected.
    output_path : str or Path
        Destination path. When ``gzip=True``, should end in ``.gz``.
    gzip : bool, optional
        Gzip the CRINEX output. Defaults to True.

    Returns
    -------
    subprocess.CompletedProcess

    Raises
    ------
    RuntimeError
        If the underlying ``sfg crinex compress`` call exits non-zero.
    """
    binary = find_binary()
    cmd = [
        str(binary),
        "crinex",
        "compress",
        "--input",
        str(input_path),
        "--output",
        str(output_path),
    ]
    if gzip:
        cmd.append("-z")

    logger.info(f"Running sfg crinex compress: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    parse_cli_logs(result, logger)
    if result.returncode != 0:
        raise RuntimeError(
            f"sfg crinex compress failed (exit {result.returncode}): {result.stderr}"
        )
    return result


def crinex_decompress(
    input_path: str | Path,
    output_path: str | Path,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Decompress a CRINEX (Hatanaka) file back to RINEX observation text.

    Parameters
    ----------
    input_path : str or Path
        Source CRINEX file. ``.gz`` inputs are auto-detected.
    output_path : str or Path
        Destination RINEX path.

    Returns
    -------
    subprocess.CompletedProcess

    Raises
    ------
    RuntimeError
        If the underlying ``sfg crinex decompress`` call exits non-zero.
    """
    binary = find_binary()
    cmd = [
        str(binary),
        "crinex",
        "decompress",
        "--input",
        str(input_path),
        "--output",
        str(output_path),
    ]

    logger.info(f"Running sfg crinex decompress: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    parse_cli_logs(result, logger)
    if result.returncode != 0:
        raise RuntimeError(
            f"sfg crinex decompress failed (exit {result.returncode}): {result.stderr}"
        )
    return result
