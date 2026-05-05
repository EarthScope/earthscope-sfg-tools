import logging
import subprocess
from collections import defaultdict
from pathlib import Path

from ..utils.go_runner import GoBinaryRunner
from ..utils.go_utils import (
    find_binary,
    parse_cli_logs,
)  # find_binary used in nov0002rnx()
from .rinex_metadata import RinexMetadata
from .utils import MetadataModel  # re-exported alias for backward compat

logger = logging.getLogger("ES_SFGTools.NovatelToRinex")
logger.setLevel(logging.INFO)


def nov0002rnx(
    input_files: list[str] | str,
    settings_file: str | Path,
    output_dir: Path | str | None = None,
    modulo: int | None = None,
    logger: logging.Logger = logger,
) -> subprocess.CompletedProcess:
    """Convert NovAtel NOV000 binary logs to RINEX files using the Go utility."""
    binary = find_binary()

    files = [
        Path(f)
        for f in ([input_files] if isinstance(input_files, str) else list(input_files))
    ]
    for file in files:
        assert file.exists(), f"Input file {file} does not exist."

    settings_file = Path(settings_file)
    assert settings_file.exists(), f"Settings file {settings_file} does not exist."
    if output_dir is not None:
        output_dir = Path(output_dir)
        assert output_dir.exists(), f"Output directory {output_dir} does not exist."

    cmd = [str(binary), "nov0002rnx", "--settings", str(settings_file)]
    if modulo is not None:
        cmd.extend(["--modulo", str(modulo)])
    cmd.extend([str(file) for file in files])

    logger.info(f"Running nov0002rnx: {' '.join(cmd)}")
    result = subprocess.run(
        cmd, capture_output=True, text=True, check=False, cwd=output_dir
    )
    parse_cli_logs(result, logger)
    return result


def _novatel_2rinex_wrapper(
    files: list[Path] | list[str],
    writedir: Path,
    metadata: RinexMetadata,
    subcommand: str,
    modulo_millis: int = 0,
    num_routines: int = 1,
    antindex: int = 0,
    logger: logging.Logger = logger,
) -> list[Path]:
    """Internal helper: run a NovAtel-to-RINEX sfg subcommand via GoBinaryRunner."""
    if not files:
        raise ValueError("No input files provided to _novatel_2rinex_wrapper")

    site = metadata.marker_name

    def _setup(workdir: Path) -> list[str]:
        meta_path = metadata.write(workdir / f"{site}_metadata.json")
        flags = ["--settings", str(meta_path)]
        if modulo_millis > 0:
            flags += ["--modulo", str(modulo_millis)]
        if num_routines > 1:
            flags += ["--numroutines", str(num_routines)]
        if antindex > 0:
            flags += ["--antindex", str(antindex)]
        return flags

    runner = GoBinaryRunner(
        subcommand,
        output_glob=lambda s: f"*{s}*",
        log=logger,
    )
    return runner.run(
        input_files=[Path(f) for f in files],
        output_dir=writedir,
        site=site,
        setup_fn=_setup,
    )


def novatel_binary_2rinex(
    files: list[Path] | list[str] | str | Path,
    writedir: Path | str | None = None,
    site: str | None = None,
    metadata: dict | MetadataModel | Path | str | None = None,
    modulo_millis: int = 0,
    num_routines: int = 1,
    antindex: int = 0,
    **kwargs,
) -> list[Path]:
    """Convert NovAtel NOV000 / NOV770 binary files to daily RINEX."""
    meta = RinexMetadata.load(metadata, site=site)

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
                raise ValueError(
                    f"Unsupported file extension: {suffix} for file {file}"
                )

    all_rinex_paths: list[Path] = []

    if bin_files:
        if writedir is None:
            write_dirs: dict[Path, list[Path]] = defaultdict(list)
            for file in bin_files:
                write_dirs[file.parent].append(file)
        else:
            write_dirs = {Path(writedir): bin_files}
        for write_dir, files_to_process in write_dirs.items():
            all_rinex_paths.extend(
                _novatel_2rinex_wrapper(
                    files=files_to_process,
                    writedir=write_dir,
                    metadata=meta,
                    subcommand="nov0002rnx",
                    modulo_millis=modulo_millis,
                    num_routines=num_routines,
                    antindex=antindex,
                    logger=logger,
                )
            )

    if raw_files:
        if writedir is None:
            write_dirs: dict[Path, list[Path]] = defaultdict(list)
            for file in raw_files:
                write_dirs[file.parent].append(file)
        else:
            write_dirs = {Path(writedir): raw_files}
        for write_dir, files_to_process in write_dirs.items():
            all_rinex_paths.extend(
                _novatel_2rinex_wrapper(
                    files=files_to_process,
                    writedir=write_dir,
                    metadata=meta,
                    subcommand="novb2rnx",
                    modulo_millis=modulo_millis,
                    num_routines=num_routines,
                    logger=logger,
                )
            )

    counted_paths: dict[Path, int] = defaultdict(int)
    for rinex_path in all_rinex_paths:
        counted_paths[rinex_path] += 1
    overlapping = "\n".join(str(p) for p, count in counted_paths.items() if count > 1)
    if overlapping:
        logger.warning(
            f"The following RINEX files were generated multiple times:\n{overlapping}"
        )

    return all_rinex_paths
