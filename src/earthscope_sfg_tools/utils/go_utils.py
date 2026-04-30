import re
import subprocess
import logging
import warnings
from pathlib import Path
import logging

from .system_utils import raise_exception ,get_system_architecture

logger = logging.getLogger(__name__)

GO_BINARY_BUILD_DIR = Path(__file__).parent.parent / "go" / "build"

WARNINGS_DICT = {}


class BinaryNotFoundError(Exception):
    """Raised when a required Go binary cannot be found."""

    pass


def find_binary(name: str) -> Path:
    """Find a Go binary by name.

    Parameters
    ----------
    name : str
        Binary name (e.g., 'nova2rnx')

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
    os_name, arch = get_system_architecture()

    binary_name = f"{name}_{os_name}_{arch}"

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
        f"Please ensure Go tools are built and available in PATH or {GO_BINARY_BUILD_DIR}"
    )


def parse_error(string: str) -> Warning | None:
    for key, warning_ in WARNINGS_DICT.items():
        if key in string:
            return warning_
    return None


def remove_ansi_escape(text):
    ansi_escape = re.compile(r"\x1B\[[0-?]*[ -/]*[@-~]")
    return ansi_escape.sub("", text)


def parse_cli_logs(result: subprocess.CompletedProcess, logger: logging.Logger):
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
