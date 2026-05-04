"""Go binary execution layer — hides subprocess orchestration from callers."""

from __future__ import annotations

import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Callable, Protocol, runtime_checkable
import logging

from .go_utils import BinaryNotFoundError, find_binary, parse_cli_logs  # noqa: F401

logger = logging.getLogger(__name__)


@runtime_checkable
class SubprocessRunner(Protocol):
    """Anything that runs a command and returns a CompletedProcess.

    Inject a fake implementation in tests to avoid hitting real binaries.
    """

    def __call__(
        self,
        cmd: list[str],
        *,
        cwd: Path,
        capture_output: bool,
        text: bool,
    ) -> subprocess.CompletedProcess: ...


def _real_runner(
    cmd: list[str],
    *,
    cwd: Path,
    capture_output: bool = True,
    text: bool = True,
) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, cwd=cwd, capture_output=capture_output, text=text)


class GoBinaryRunner:
    """Run a single Go binary that consumes input files and emits output files.

    Hides: binary discovery, temp-workdir lifecycle, subprocess execution,
    stdout/stderr log parsing, non-zero exit handling, output file collection,
    and shutil.move into the destination directory.

    Parameters
    ----------
    binary_name:
        Logical binary name (e.g. ``"nov0002rnx"``).  ``find_binary()``
        resolves the platform-specific path at construction time so
        ``BinaryNotFoundError`` surfaces immediately.
    output_glob:
        Shell glob used to discover output files inside the temp workdir
        after the binary exits.  Pass a callable ``(site: str) -> str``
        for patterns that embed the site code.  Default ``"*"`` collects
        every file (useful for QC-style binaries via ``run_raw``).
    runner:
        Injected subprocess implementation.  ``None`` uses the real
        ``subprocess.run``.  Pass a ``SubprocessRunner`` in tests.
    log:
        Logger to use.  Defaults to the module logger.
    """

    def __init__(
        self,
        binary_name: str,
        output_glob: str | Callable[[str], str] = "*",
        *,
        binary_path: Path | None = None,
        runner: SubprocessRunner | None = None,
        log: logging.Logger | None = None,
    ) -> None:
        self._binary: Path = binary_path if binary_path is not None else find_binary(binary_name)
        self._output_glob = output_glob
        self._runner: SubprocessRunner = runner or _real_runner
        self._log = log or logger

    # ------------------------------------------------------------------
    # Primary interface
    # ------------------------------------------------------------------

    def run(
        self,
        input_files: list[Path | str] | Path | str,
        output_dir: Path | str,
        site: str,
        *,
        setup_fn: Callable[[Path], list[str]] | None = None,
        extra_flags: list[str] | None = None,
    ) -> list[Path]:
        """Convert *input_files* and return paths of output files in *output_dir*.

        Parameters
        ----------
        input_files:
            One or more input file paths.
        output_dir:
            Destination directory.  Must already exist.  Output files are
            atomically moved here from a private temp workdir.
        site:
            4-character site code used for glob-based output discovery when
            ``output_glob`` is a callable.
        setup_fn:
            Called with the temp workdir ``Path`` before the binary runs.
            Returns a list of CLI flags to prepend (e.g.
            ``["-settings", "/tmp/x/meta.json"]``).  Use it to write
            auxiliary files (like the metadata JSON) into the workdir.
        extra_flags:
            Static CLI flags appended after ``setup_fn`` flags and before
            file arguments.

        Raises
        ------
        FileNotFoundError
            If any input file does not exist, or ``output_dir`` does not exist.
        RuntimeError
            If the binary exits with a non-zero return code.
        """
        files = _coerce_files(input_files)
        out_dir = Path(output_dir)

        if not out_dir.exists():
            raise FileNotFoundError(f"output_dir does not exist: {out_dir}")
        for f in files:
            if not f.exists():
                raise FileNotFoundError(f"Input file not found: {f}")

        glob_pat = (
            self._output_glob(site)
            if callable(self._output_glob)
            else self._output_glob
        )

        with tempfile.TemporaryDirectory() as _tmp:
            workdir = Path(_tmp)

            dynamic_flags = setup_fn(workdir) if setup_fn else []
            all_flags = dynamic_flags + (extra_flags or [])

            cmd = [str(self._binary)] + all_flags + [str(f) for f in files]
            self._log.info("Running: %s (cwd=%s)", " ".join(cmd), workdir)

            result = self._runner(cmd, cwd=workdir, capture_output=True, text=True)
            parse_cli_logs(result, self._log)

            if result.returncode != 0:
                raise RuntimeError(
                    f"{self._binary.name} exited {result.returncode}.\n"
                    f"stdout: {result.stdout}\nstderr: {result.stderr}"
                )

            return _collect_outputs(workdir, glob_pat, out_dir, self._log)

    def run_raw(
        self,
        input_files: list[Path | str] | Path | str,
        *,
        extra_flags: list[str] | None = None,
        cwd: Path | None = None,
    ) -> subprocess.CompletedProcess:
        """Run the binary without output-file collection.

        No temp workdir is created.  Intended for QC tools whose useful
        output is stdout/stderr rather than produced files.

        Args:
            input_files: One or more input file paths.
            extra_flags: Optional CLI flags inserted before file arguments.
            cwd: Working directory for the subprocess. Defaults to ``'.'``.

        Returns:
            The raw ``subprocess.CompletedProcess`` from the binary.
        """
        files = _coerce_files(input_files)
        cmd = [str(self._binary)] + (extra_flags or []) + [str(f) for f in files]
        return self._runner(cmd, cwd=cwd or Path("."), capture_output=True, text=True)

    # ------------------------------------------------------------------
    # Introspection
    # ------------------------------------------------------------------

    @property
    def binary_path(self) -> Path:
        """Resolved filesystem path to the Go binary."""
        return self._binary

    @classmethod
    def available_binaries(cls, names: list[str]) -> dict[str, Path | None]:
        """Probe binary names and return a mapping of name to resolved path.

        Useful for health-checks and CI pre-flight checks.

        Args:
            names: List of logical binary names to probe (e.g.
                ``['nova2rnx', 'nov0002rnx']``).

        Returns:
            Dict mapping each name to its resolved ``Path``, or ``None``
            if the binary is not found.
        """
        result: dict[str, Path | None] = {}
        for name in names:
            try:
                result[name] = find_binary(name)
            except BinaryNotFoundError:
                result[name] = None
        return result


# ---------------------------------------------------------------------------
# Private helpers
# ---------------------------------------------------------------------------

def _coerce_files(raw: list[Path | str] | Path | str) -> list[Path]:
    if isinstance(raw, (str, Path)):
        raw = [raw]
    return [Path(f) for f in raw]


def _collect_outputs(
    workdir: Path,
    glob: str,
    dest: Path,
    log: logging.Logger,
) -> list[Path]:
    found = [p for p in workdir.rglob(glob) if p.is_file()]
    if not found:
        log.warning("Binary produced no output files matching %r in %s", glob, workdir)
        return []
    out: list[Path] = []
    for src in found:
        dst = dest / src.name
        if dst.exists():
            log.warning("Overwriting existing file: %s", dst)
        shutil.move(str(src), dst)
        out.append(dst)
    return out
