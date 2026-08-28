"""Helpers for locating bundled Go binaries and parsing their CLI log output."""

import re
import subprocess
import logging
import warnings
from pathlib import Path
import logging

from .system_utils import raise_exception, get_system_architecture

logger = logging.getLogger(__name__)

# When installed as a wheel: site-packages/earthscope_sfg_tools/go/build
# When installed editable/dev: src/earthscope_sfg_tools/../go/build → src/go/build
_pkg_root = Path(__file__).parent.parent
GO_BINARY_BUILD_DIR = _pkg_root / "go" / "build"
GO_BINARY_BUILD_DIR_SOURCE = _pkg_root.parent / "go" / "build"

WARNINGS_DICT = {}


class BinaryNotFoundError(Exception):
    """Raised when a required Go binary cannot be found."""

    pass


def find_binary(name: str = "sfg") -> Path:
    """Find the ``sfg`` Go binary.

    Parameters
    ----------
    name : str
        Ignored — retained for backward compatibility.  All subcommands
        are now part of the single ``sfg`` binary.

    Returns
    -------
    Path
        Full path to the ``sfg`` binary.

    Raises
    ------
    BinaryNotFoundError
        If the binary cannot be found.
    """
    os_name, arch = get_system_architecture()
    binary_name = f"sfg_{os_name}_{arch}"

    # Check installed wheel path then source/editable path
    for build_dir in (GO_BINARY_BUILD_DIR, GO_BINARY_BUILD_DIR_SOURCE):
        if build_dir.exists():
            binary_path = build_dir / binary_name
            if binary_path.exists() and binary_path.is_file():
                return binary_path

    if not GO_BINARY_BUILD_DIR.exists() and not GO_BINARY_BUILD_DIR_SOURCE.exists():
        logger.warning(
            f"Go binary build directory {GO_BINARY_BUILD_DIR} does not exist. "
            f"Please ensure Go tools are built and available in PATH or {GO_BINARY_BUILD_DIR}",
            stacklevel=2,
        )

    # Try to find in PATH
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
        f"Please ensure Go tools are built and available in PATH or {GO_BINARY_BUILD_DIR}"
    )


def parse_error(string: str) -> Warning | None:
    """Return the warning class matching an error string, or ``None``.

    Args:
        string: Text to test against the ``WARNINGS_DICT`` keys.

    Returns:
        The matching ``Warning`` subclass, or ``None``.
    """
    for key, warning_ in WARNINGS_DICT.items():
        if key in string:
            return warning_
    return None


def remove_ansi_escape(text):
    """Strip ANSI escape sequences from a string.

    Args:
        text: Raw text that may contain terminal colour/control codes.

    Returns:
        The input string with all ANSI escape sequences removed.
    """
    ansi_escape = re.compile(r"\x1B\[[0-?]*[ -/]*[@-~]")
    return ansi_escape.sub("", text)


# Go binaries log via logrus in the form: time="..." level=info msg="..."
_LOGRUS_LEVEL = re.compile(r"\blevel=(\w+)")
_LEVEL_MAP = {
    "trace": logging.DEBUG,
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warning": logging.WARNING,
    "warn": logging.WARNING,
    "error": logging.ERROR,
    "fatal": logging.CRITICAL,
    "panic": logging.CRITICAL,
}


def _forward_logrus(stream_text: str, logger: logging.Logger, default_level: int):
    """Forward each Go log line at its own logrus level.

    Each line is emitted as its own record at the Python level that maps to
    the line's ``level=`` field (info→INFO, warning→WARNING, error→ERROR,
    etc.).  Lines with no recognizable ``level=`` field fall back to
    ``default_level``.  Known warning/exception strings in the ``msg=``
    payload are preserved as ``warnings.warn`` calls / raised exceptions.

    Args:
        stream_text: Decoded, ANSI-stripped stdout or stderr text.
        logger: Logger to receive the forwarded messages.
        default_level: Level used for lines lacking a ``level=`` field.

    Raises:
        Exception: Any exception class returned by :func:`raise_exception`
            when its trigger string is found in a line's ``msg=`` payload.
    """
    for line in stream_text.splitlines():
        line = line.strip()
        if not line:
            continue
        match = _LOGRUS_LEVEL.search(line)
        level = (
            _LEVEL_MAP.get(match.group(1).lower(), default_level)
            if match
            else default_level
        )
        logger.log(level, line)

        # Preserve existing side effects on the msg= payload.
        message = line.split("msg=", 1)[-1]
        if (warning := parse_error(message)) is not None:
            logger.warning(warning.message)
            warnings.warn(warning.message, warning, 3)
        if (exception := raise_exception(message)) is not None:
            raise exception


def parse_cli_logs(result: subprocess.CompletedProcess, logger: logging.Logger):
    """Parse and forward stdout/stderr from a Go binary subprocess.

    Strips ANSI escape codes, then forwards each line at its own logrus
    ``level=`` (see :func:`_forward_logrus`), so ordinary
    ``logger.setLevel`` calls filter the Go output as expected.  Lines with
    no ``level=`` field default to DEBUG on stdout and WARNING on stderr.
    Lines matching a known exception pattern raise that exception.

    Args:
        result: Completed subprocess whose stdout/stderr are plain text.
        logger: Logger to receive the forwarded messages.

    Raises:
        Exception: Any exception class returned by :func:`raise_exception`
            when its trigger string is found in stdout or stderr.
    """
    if result.stdout:
        _forward_logrus(
            remove_ansi_escape(result.stdout), logger, default_level=logging.DEBUG
        )
    if result.stderr:
        _forward_logrus(
            remove_ansi_escape(result.stderr), logger, default_level=logging.WARNING
        )
