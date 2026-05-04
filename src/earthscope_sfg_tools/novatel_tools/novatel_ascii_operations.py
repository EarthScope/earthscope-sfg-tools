# External imports
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

import logging

import numpy as np

from ..utils.go_utils import find_binary, parse_cli_logs
from ..utils.misc import listify
from .rinex_metadata import RinexMetadata
from .utils import MetadataModel  # backward-compat alias

logger = logging.getLogger(__name__)


def nova2rnx(
    input_files: list[str] | str,
    settings_file: str | Path,
    output_dir: Path | str | None = None,
    modulo: int | None = None,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel ASCII logs to RINEX files using the Go utility."""
    binary = find_binary("nova2rnx")

    settings_file = Path(settings_file)
    assert settings_file.exists(), f"Settings file {settings_file} does not exist."
    if output_dir is not None:
        output_dir = Path(output_dir)
        assert output_dir.exists(), f"Output directory {output_dir} does not exist."

    files = [Path(f) for f in listify(input_files)]
    for file in files:
        assert file.exists(), f"Input file {file} does not exist."

    cmd = [str(binary), "-settings", str(settings_file)]
    if modulo is not None:
        cmd.extend(["-modulo", str(modulo)])
    cmd.extend([str(file) for file in files])

    logger.info(f"Running nova2rnx: {' '.join(cmd)}")
    result = subprocess.run(
        cmd, capture_output=True, text=True, check=False, cwd=output_dir
    )
    parse_cli_logs(result, logger)
    return result


def novatel_ascii_2rinex(
    files: list[Path | str] | Path | str,
    writedir: Path = None,
    site: str = "SIT1",
    metadata: dict | MetadataModel | Path | str = None,
    modulo_millis: int = 0,
    logger: logging.Logger = logger,
    *kwargs,
) -> list[Path]:
    """Convert a NovAtel ASCII file to a daily RINEX file using nova2rnxo.
    This function wraps the external `nova2rnxo` binary to convert a NovAtel ASCII
    observation file into a daily RINEX file. Metadata describing the site and
    receiver can be supplied directly or generated automatically from a site code.
    If `metadata` is a mapping or `MetadataModel`, it is validated and written
    to a JSON file in `writedir`. If it is a path (or string path), it is checked
    for existence and validated as JSON metadata. If `metadata` is not provided,
    a metadata JSON is generated using `get_metadatav2` with the given `site`
    code and a random serial number.
    The resulting RINEX file produced by `nova2rnxo` is moved from a temporary
    working directory into `writedir` and its final path is returned.

    Parameters
    ----------
    files : list[pathlib.Path or str] or pathlib.Path or str
        Paths to the input NovAtel ASCII files to convert.
    writedir : pathlib.Path or str, optional
        Directory where the output RINEX (and metadata JSON, if created) will
        be written. Defaults to the parent directory of the first file in `files`.
    site : str, optional
        Four-character site code used when generating metadata automatically,
        required if `metadata` is not provided. Must be exactly 4 characters.
    metadata : dict, MetadataModel, pathlib.Path or str, optional
        Site/receiver metadata. May be:
          * A dict containing metadata fields compatible with `MetadataModel`.
          * A `MetadataModel` instance.
          * A path (or string path) to a JSON file with metadata.
        If provided, it is validated against `MetadataModel`. If provided as a
        dict or `MetadataModel`, a JSON metadata file is written to `writedir`.
    modulo_millis : int, optional
        Decimation modulo in milliseconds (e.g., 1000 for 1 Hz, 15000 for 15s
        intervals). If 0, no decimation is applied. Loss-of-lock indicators
        from skipped epochs are propagated to the next written epoch. Default
        is 0 (no decimation).
    **kwargs
        Currently unused; accepted for future extensibility.

    Returns
    -------
    List[pathlib.Path]
        Paths to the generated daily RINEX files in `writedir`.

    Raises
    ------
    AssertionError
        If `metadata` is None and `site` is missing, not a string, or not
        exactly 4 characters long, or if `metadata` is not a path by the time
        it is passed to `nova2rnxo`.
    ValueError
        If the metadata dictionary, model, or JSON file fails validation
        against `MetadataModel`, or if `metadata` is of an unsupported type.
    FileNotFoundError
        If the provided metadata file path does not exist.
    subprocess.CalledProcessError
        If the `nova2rnxo` subprocess fails.
    """

    files = listify(files)
    files = [Path(file) if isinstance(file, str) else file for file in files]

    if writedir is None:
        writedir = files[0].parent
    elif isinstance(writedir, str):
        writedir = Path(writedir)

    meta = RinexMetadata.load(metadata, site=site)
    binary_path = find_binary("nova2rnx")

    logger.info(f"Converting and merging {files} ascii Novatel to RINEX", stacklevel=2)

    with tempfile.TemporaryDirectory() as workdir:
        metadata_path = meta.write(Path(workdir) / f"{meta.marker_name}_metadata.json")
        cmd = [str(binary_path), "-settings", str(metadata_path)]
        if modulo_millis > 0:
            cmd.extend(["-modulo", str(modulo_millis)])
        for file in files:
            cmd.append(str(file))
        cmd_str = " ".join(cmd)
        logger.info(f" Running {cmd_str} in {workdir}", stacklevel=2)
        result = subprocess.run(cmd, check=True, capture_output=True, cwd=workdir)

        parse_cli_logs(result, logger)

        rinex_file_paths = list(Path(workdir).rglob(f"*{site}*"))
        logger.info(
            f"Converted {files} to {rinex_file_paths} Daily RINEX files", stacklevel=2
        )
        outpaths = []
        for rinex_file in rinex_file_paths:
            logger.debug(f" RINEX file: {str(rinex_file)}")
            new_rinex_path = writedir / rinex_file.name
            shutil.move(src=rinex_file, dst=new_rinex_path)
            logger.info(
                f"Generated Daily RINEX file {str(new_rinex_path)}", stacklevel=2
            )
            outpaths.append(new_rinex_path)

    return outpaths


def qcpin_to_novatelpin(source: str | Path, writedir: Path) -> Path:
    """Convert a QCPIN JSON file to a Novatel PIN text file.
    Args:
        source (str|Path): Path to the QCPIN JSON file.
        writedir (Path): Directory where the Novatel PIN text file will be saved.
    Returns:
        Path: Path to the generated Novatel PIN text file.
    """
    with open(source) as file:
        pin_data = json.load(file)

    range_headers = []
    time_stamps = []

    for data in pin_data.values():
        range_header = data.get("observations").get("NOV_RANGE")
        time_header = data.get("observations").get("NOV_INS").get("time").get("common")
        range_headers.append(range_header)
        time_stamps.append(time_header)

    time_sorted = np.argsort(time_stamps)
    range_headers = [range_headers[i] for i in time_sorted]

    file_path = writedir / (str(source.id) + "_novpin.txt")
    with tempfile.NamedTemporaryFile(mode="w+", delete=True) as temp_file:
        for header in range_headers:
            temp_file.write(header)
            temp_file.write("\n")
        temp_file.seek(0)
        shutil.copy(temp_file.name, file_path)
        novatel_pin = Path(file_path)

    return novatel_pin
