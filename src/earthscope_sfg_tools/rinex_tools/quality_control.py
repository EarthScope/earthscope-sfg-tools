
from typing import Optional
import logging
import subprocess

from earthscope_sfg_tools.utils.go_utils import find_binary

logger = logging.getLogger(__name__)

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
        Subprocess result containing exit code and output.

    Raises
    ------
    BinaryNotFoundError
        If the rnxqc binary cannot be found.
    """
    binary = find_binary("rnxqc")

    cmd = [str(binary), input_file]

    logger.info(f"Running rnxqc: {' '.join(cmd)}", stacklevel=2)
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    return result
