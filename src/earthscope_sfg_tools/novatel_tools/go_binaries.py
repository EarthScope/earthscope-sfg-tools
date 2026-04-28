"""Wrappers for calling compiled Go utilities.

These are optional tools that provide high-performance RINEX and TileDB operations.
They require pre-compiled Go binaries to be available in the system PATH or in a
standard location (e.g., `<package>/go/build/`).

If binaries are not available, functions will raise BinaryNotFoundError.
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path
from typing import Optional
import platform

from ..logging import get_logger


class BinaryNotFoundError(Exception):
    """Raised when a required Go binary cannot be found."""

    pass


def _find_binary(name: str, search_paths: Optional[list[str]] = None) -> Path:
    """Find a Go binary by name.

    Parameters
    ----------
    name : str
        Binary name (e.g., 'nova2rnx')
    search_paths : list[str], optional
        Additional paths to search. Will check package's go/build/ directory first.

    Returns
    -------
    Path
        Full path to the binary.

    Raises
    ------
    BinaryNotFoundError
        If binary cannot be found.
    """
    # Determine platform-specific binary name
    os_name = platform.system().lower()
    arch = platform.machine().lower()

    # Map architecture names
    arch_map = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}
    arch = arch_map.get(arch, arch)

    # Map OS names
    os_map = {"darwin": "darwin", "linux": "linux", "windows": "windows"}
    os_name = os_map.get(os_name, os_name)

    binary_name = f"{name}_{os_name}_{arch}"

    # Check package's go/build/ directory first
    package_go_build = Path(__file__).parent.parent.parent.parent.parent / "go" / "build"
    if package_go_build.exists():
        binary_path = package_go_build / binary_name
        if binary_path.exists() and binary_path.is_file():
            return binary_path

    # Check provided search paths
    if search_paths:
        for search_path in search_paths:
            binary_path = Path(search_path) / binary_name
            if binary_path.exists() and binary_path.is_file():
                return binary_path

    # Try to find in PATH (plain name)
    try:
        result = subprocess.run(
            ["which", binary_name],
            capture_output=True,
            text=True,
            timeout=5,
        )
        if result.returncode == 0:
            return Path(result.stdout.strip())
    except (subprocess.TimeoutExpired, FileNotFoundError):
        pass

    raise BinaryNotFoundError(
        f"Binary '{binary_name}' not found. "
        f"Please ensure Go tools are built and available in PATH or {package_go_build}"
    )


def nova2rnx(
    input_files: list[str] | str,
    settings_file: str,
    modulo: Optional[int] = None,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Convert NovAtel ASCII logs to RINEX files.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel input file(s).
    settings_file : str
        Path to JSON settings file for RINEX metadata.
    modulo : int, optional
        Decimation modulo in milliseconds (e.g., 1000 for 1 Hz). If None, no decimation.
    search_paths : list[str], optional
        Additional paths to search for the binary.

    Returns
    -------
    int
        Subprocess exit code.

    Raises
    ------
    BinaryNotFoundError
        If the nova2rnx binary cannot be found.
    subprocess.CalledProcessError
        If the subprocess fails.
    """
    binary = _find_binary("nova2rnx", search_paths)

    files = [input_files] if isinstance(input_files, str) else input_files

    cmd = [str(binary), "-settings", settings_file]
    if modulo is not None:
        cmd.extend(["-modulo", str(modulo)])
    cmd.extend(files)

    get_logger().loginfo(f"Running nova2rnx: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    return result.returncode


def nova2tile(
    input_files: list[str] | str,
    tdb_path: str,
    num_procs: int = 10,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Convert NovAtel ASCII logs to TileDB array.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel input file(s).
    tdb_path : str
        S3 or local path to TileDB array.
    num_procs : int, optional
        Number of concurrent processes. Default is 10.
    search_paths : list[str], optional
        Additional paths to search for the binary.

    Returns
    -------
    int
        Subprocess exit code.

    Raises
    ------
    BinaryNotFoundError
        If the nova2tile binary cannot be found.
    subprocess.CalledProcessError
        If the subprocess fails.
    """
    binary = _find_binary("nova2tile", search_paths)

    files = [input_files] if isinstance(input_files, str) else input_files

    cmd = [str(binary), "-tdb", tdb_path, "-procs", str(num_procs)]
    cmd.extend(files)

    get_logger().loginfo(f"Running nova2tile: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    return result.returncode


def nov0002rnx(
    input_files: list[str] | str,
    settings_file: str,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Convert NovAtel binary logs to RINEX files.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel binary input file(s).
    settings_file : str
        Path to JSON settings file for RINEX metadata.
    search_paths : list[str], optional
        Additional paths to search for the binary.

    Returns
    -------
    int
        Subprocess exit code.

    Raises
    ------
    BinaryNotFoundError
        If the nov0002rnx binary cannot be found.
    """
    binary = _find_binary("nov0002rnx", search_paths)

    files = [input_files] if isinstance(input_files, str) else input_files
    cmd = [str(binary), "-settings", settings_file]
    cmd.extend(files)

    get_logger().loginfo(f"Running nov0002rnx: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    return result.returncode


def nov0002tile(
    input_files: list[str] | str,
    tdb_path: str,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Convert NovAtel binary logs to TileDB array.

    Parameters
    ----------
    input_files : str or list[str]
        NovAtel binary input file(s).
    tdb_path : str
        S3 or local path to TileDB array.
    search_paths : list[str], optional
        Additional paths to search for the binary.

    Returns
    -------
    int
        Subprocess exit code.

    Raises
    ------
    BinaryNotFoundError
        If the nov0002tile binary cannot be found.
    """
    binary = _find_binary("nov0002tile", search_paths)

    files = [input_files] if isinstance(input_files, str) else input_files
    cmd = [str(binary), "-tdb", tdb_path]
    cmd.extend(files)

    get_logger().loginfo(f"Running nov0002tile: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    return result.returncode


def tdb2rnx(
    tdb_path: str,
    settings_file: str,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Convert TileDB array to RINEX files.

    Parameters
    ----------
    tdb_path : str
        S3 or local path to TileDB array.
    settings_file : str
        Path to JSON settings file for RINEX metadata.
    search_paths : list[str], optional
        Additional paths to search for the binary.

    Returns
    -------
    int
        Subprocess exit code.

    Raises
    ------
    BinaryNotFoundError
        If the tdb2rnx binary cannot be found.
    """
    binary = _find_binary("tdb2rnx", search_paths)

    cmd = [str(binary), "-tdb", tdb_path, "-settings", settings_file]

    get_logger().loginfo(f"Running tdb2rnx: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    return result.returncode


def rnxqc(
    input_file: str,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Perform RINEX quality control checks.

    Parameters
    ----------
    input_file : str
        Path to RINEX file.
    search_paths : list[str], optional
        Additional paths to search for the binary.

    Returns
    -------
    int
        Subprocess exit code.

    Raises
    ------
    BinaryNotFoundError
        If the rnxqc binary cannot be found.
    """
    binary = _find_binary("rnxqc", search_paths)

    cmd = [str(binary), input_file]

    get_logger().loginfo(f"Running rnxqc: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    return result.returncode
