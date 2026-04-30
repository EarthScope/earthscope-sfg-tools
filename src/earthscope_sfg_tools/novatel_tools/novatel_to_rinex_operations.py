import json
import logging
import os
import shutil
import subprocess
import tempfile
import uuid
from collections import defaultdict
from pathlib import Path

from ..utils.go_utils import BinaryNotFoundError, find_binary, parse_cli_logs
from .utils import MetadataModel, check_metadata, check_metadata_path, get_metadatav2

os.environ["DYLD_LIBRARY_PATH"] = os.environ.get("CONDA_PREFIX", "") + "/lib"
os.environ["LD_LIBRARY_PATH"] = os.environ.get("CONDA_PREFIX", "") + "/lib"

# Don't use basicConfig as it configures the root logger with default format
logger = logging.getLogger("ES_SFGTools.NovatelToRinex")
logger.setLevel(logging.INFO)


def _novatel_2rinex_wrapper(
    files: list[Path] | list[str],
    writedir: Path,
    metadata: dict | Path | str,
    binary_path: Path,
    modulo_millis: int = 0,
    num_routines: int = 1,
) -> list[Path]:
    """Internal helper to call a NovAtel-to-RINEX Go binary on a batch of files.

    Parameters
    ----------
    files
        Input NOV000 / NOV770 files to convert.
    writedir
        Directory where output RINEX files will be moved.
    metadata
        Metadata dictionary or path to a JSON metadata file.
    binary_path
        Path to the nov0002rnx / novb2rnxo binary.
    modulo_millis
        Decimation modulo in milliseconds. If 0, no decimation is applied.
    num_routines
        Number of concurrent goroutines. Defaults to 1.

    Returns
    -------
    List[pathlib.Path]
        Paths to the RINEX files moved into ``writedir``.

    Raises
    ------
    ValueError
        If ``files`` is empty or ``metadata`` cannot be interpreted.
    FileNotFoundError
        If a provided metadata path does not exist.
    RuntimeError
        If the underlying Go binary exits with a non-zero return code.
    """

    if not files:
        raise ValueError("No input files provided to _novatel_2rinex_wrapper")

    file_paths: list[Path] = [Path(f) for f in files]

    with tempfile.TemporaryDirectory(dir="/tmp/") as workdir_str:
        workdir = Path(workdir_str)

        if isinstance(metadata, dict):
            metadata_dict = metadata
        elif isinstance(metadata, (Path, str)):
            metadata_path = Path(metadata)
            if not metadata_path.exists():
                raise FileNotFoundError(f"Metadata file not found: {metadata_path}")
            if not metadata_path.is_file():
                raise ValueError(f"Metadata path is not a file: {metadata_path}")
            if (suffix := metadata_path.suffix.lower()) != ".json":
                raise ValueError(f"Metadata file must be a JSON file, got {suffix}")
            with open(metadata_path) as f:
                metadata_dict = json.load(f)
        else:
            raise ValueError(f"Metadata must be a dict or path to JSON file, got {type(metadata)}")

        site = metadata_dict.get("marker_name", "SIT1")

        metadata_tmp_path = workdir / f"{site}_metadata.json"
        with open(metadata_tmp_path, "w") as f:
            json.dump(metadata_dict, f, indent=4)

        cmd = [str(binary_path), "-settings", str(metadata_tmp_path)]
        if modulo_millis > 0:
            cmd.extend(["-modulo", str(modulo_millis)])
        if num_routines > 1:
            cmd.extend(["-numroutines", str(num_routines)])
        cmd.extend([str(p) for p in file_paths])

        logger.info(f"Running {' '.join(cmd)} in {workdir}", stacklevel=2)
        result = subprocess.run(cmd, capture_output=True, cwd=workdir, text=True)

        if result.returncode != 0:
            print(
                {
                    "returncode": result.returncode,
                    "stdout": result.stdout,
                    "stderr": result.stderr,
                }
            )
            raise RuntimeError(
                f"{binary_path.name} failed with return code {result.returncode}. "
                "See logs for stdout/stderr."
            )

        parse_cli_logs(result, logger)

        rinex_file_paths = [x for x in workdir.rglob(f"*{site}*") if x.suffix != ".json"]
        logger.info(
            f"Converted {len(file_paths)} input files to {len(rinex_file_paths)} Daily RINEX files",
            stacklevel=2,
        )
        outpaths: list[Path] = []
        for rinex_file in rinex_file_paths:
            logger.debug(f"RINEX file: {rinex_file}", stacklevel=2)
            new_rinex_path = writedir / rinex_file.name
            if new_rinex_path.exists():
                logger.warning(f"RINEX file {new_rinex_path} already exists and will be overwritten.")
            shutil.move(src=rinex_file, dst=new_rinex_path)
            logger.info(f"Generated Daily RINEX file {new_rinex_path}", stacklevel=2)
            outpaths.append(new_rinex_path)

    if not outpaths:
        logger.warning(f"No RINEX files were generated from files: {file_paths}", stacklevel=2)
    return outpaths


def novatel_binary_2rinex(
    files: list[Path] | list[str] | str | Path,
    writedir: Path | str | None = None,
    site: str | None = None,
    metadata: dict | MetadataModel | Path | str | None = None,
    modulo_millis: int = 0,
    num_routines: int = 1,
    **kwargs,
) -> list[Path]:
    """Convert NovAtel NOV000 / NOV770 binary files to daily RINEX.

    Parameters
    ----------
    files : List[pathlib.Path] | List[str] | str | pathlib.Path
        Input NOV000/NOV770 files to convert.
    writedir : Optional[pathlib.Path | str], optional
        Directory where output RINEX files will be written.
    site : Optional[str], optional
        4-character site code. Required if ``metadata`` is not provided.
    metadata : Optional[dict | MetadataModel | pathlib.Path | str], optional
        Metadata for the site.
    modulo_millis : int, optional
        Decimation modulo in milliseconds. Default is 0 (no decimation).
    num_routines : int, optional
        Number of concurrent goroutines. Defaults to 1.

    Returns
    -------
    List[pathlib.Path]
        List of generated daily RINEX file paths.

    Notes
    -----
    NOV000.bin files are processed before NOV770.raw files to prevent
    lower-frequency data from overwriting higher-frequency RINEX output
    when ``writedir`` is shared.
    """

    if metadata is not None:
        if isinstance(metadata, (str, Path)):
            metadata = check_metadata_path(metadata)
        elif isinstance(metadata, (dict, MetadataModel)):
            metadata = check_metadata(metadata)
        else:
            raise ValueError(
                f"Metadata must be a dict, MetadataModel, or path to a JSON file, got {type(metadata)}"
            )
    else:
        if site is None:
            raise ValueError("Either metadata or site must be provided")
        if not isinstance(site, str):
            raise ValueError(f"Site must be a string, got {type(site)}")
        if len(site) != 4:
            raise ValueError(f"Site must be 4 characters long, got {site}")
        metadata = get_metadatav2(site, serialNumber=uuid.uuid4().hex[:10])

    if isinstance(files, (str, Path)):
        file_paths: list[Path] = [Path(files)]
    else:
        file_paths = [Path(f) for f in files]

    if not file_paths:
        raise ValueError("No input files provided to novatel_2rinex")

    for file in file_paths:
        if not file.exists():
            raise FileNotFoundError(f"File not found: {file}")

    bin_files: list[Path] = []
    raw_files: list[Path] = []

    for file in file_paths:
        suffix = file.suffix.lower()
        match suffix:
            case ".bin":
                bin_files.append(file)
            case ".raw":
                raw_files.append(file)
            case _:
                raise ValueError(f"Unsupported file extension: {suffix} for file {file}")

    all_rinex_paths: list[Path] = []

    if bin_files:
        binary_path = find_binary("nov0002rnx")
        if writedir is None:
            write_dirs: dict[Path, list[Path]] = defaultdict(list)
            for file in bin_files:
                write_dirs[file.parent].append(file)
            logger.info(
                "writedir is None, processing NOV000.bin files grouped by directory: "
                f"{list(write_dirs.keys())}"
            )
        else:
            write_dirs = {Path(writedir): bin_files}
            logger.info(f"Processing NOV000.bin files to writedir: {Path(writedir)}")
        for write_dir, files_to_process in write_dirs.items():
            rinex_paths = _novatel_2rinex_wrapper(
                files=files_to_process,
                writedir=write_dir,
                metadata=metadata,
                binary_path=binary_path,
                modulo_millis=modulo_millis,
                num_routines=num_routines,
            )
            logger.info(
                f"Converted {len(files_to_process)} NOV000.bin files to "
                f"{len(rinex_paths)} RINEX files in {write_dir}"
            )
            all_rinex_paths.extend(rinex_paths)

    if raw_files:
        binary_path = find_binary("novb2rnxo")
        if writedir is None:
            write_dirs: dict[Path, list[Path]] = defaultdict(list)
            for file in raw_files:
                write_dirs[file.parent].append(file)
            logger.info(
                "writedir is None, processing NOV770.raw files grouped by directory: "
                f"{list(write_dirs.keys())}"
            )
        else:
            write_dirs = {Path(writedir): raw_files}
            logger.info(f"Processing NOV770.raw files to writedir: {Path(writedir)}")
        for write_dir, files_to_process in write_dirs.items():
            rinex_paths = _novatel_2rinex_wrapper(
                files=files_to_process,
                writedir=write_dir,
                metadata=metadata,
                binary_path=binary_path,
                modulo_millis=modulo_millis,
                num_routines=num_routines,
            )
            logger.info(
                f"Converted {len(files_to_process)} NOV770.raw files to "
                f"{len(rinex_paths)} RINEX files in {write_dir}"
            )
            all_rinex_paths.extend(rinex_paths)

    counted_paths: dict[Path, int] = defaultdict(int)
    for rinex_path in all_rinex_paths:
        counted_paths[rinex_path] += 1
    overlapping = "\n".join(str(p) for p, count in counted_paths.items() if count > 1)
    if overlapping:
        logger.warning(
            f"The following RINEX files were generated multiple times:\n{overlapping}"
        )

    return all_rinex_paths
