import re
import subprocess
import logging
import warnings
from pathlib import Path
import logging

from .system_utils import raise_exception, get_system_architecture

logger = logging.getLogger(__name__)

GO_BINARY_BUILD_DIR = Path(__file__).parent.parent.parent / "go" / "build"

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

    # Check package's go/build/ directory first
    if GO_BINARY_BUILD_DIR.exists():
        binary_path = GO_BINARY_BUILD_DIR / binary_name
        if binary_path.exists() and binary_path.is_file():
            return binary_path
    else:
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


def parse_cli_logs(result: subprocess.CompletedProcess, logger: logging.Logger):
    """Parse and forward stdout/stderr from a Go binary subprocess.

    Strips ANSI escape codes, then routes lines to the appropriate log
    level.  Lines containing ``'Processed'`` or ``'Created'`` are logged
    at INFO; everything else at DEBUG.  Lines matching a known exception
    pattern raise that exception.  ``stderr`` lines containing ``'error'``
    are logged at ERROR; others at WARNING.

    Args:
        result: Completed subprocess whose stdout/stderr are plain text.
        logger: Logger to receive the forwarded messages.

    Raises:
        Exception: Any exception class returned by :func:`raise_exception`
            when its trigger string is found in stdout or stderr.
    """
    if result.stdout:
        stdout_cleaned = remove_ansi_escape(result.stdout)
        logger.debug(stdout_cleaned)
        result_message = stdout_cleaned.split("msg=")
        for log_line in result_message:
            message = log_line.split("\n")[0]
            if "Processed" in message or "Created" in message:
                logger.info(message)
            if (exception := raise_exception(message)) is not None:
                raise exception
    if result.stderr:
        stderr_cleaned = remove_ansi_escape(result.stderr)
        if "error" in stderr_cleaned.lower():
            logger.error(stderr_cleaned)
            if (warning := parse_error(stderr_cleaned)) is not None:
                logger.warning(warning.message)
                warnings.warn(warning.message, warning, 3)
        else:
            logger.warning(stderr_cleaned)

        result_message = stderr_cleaned.split("msg=")
        for log_line in result_message:
            message = log_line.split("\n")[0]
            if "Processing" in message or "Created" in message:
                logger.info(message)
            if (exception := raise_exception(message)) is not None:
                raise exception
