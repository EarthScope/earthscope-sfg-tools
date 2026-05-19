# go_runner

`earthscope_sfg_tools.utils.go_runner`

Go binary execution layer — hides subprocess orchestration from callers.

## class `GoBinaryRunner`

Run a subcommand of the ``sfg`` Go binary.

Hides: binary discovery, temp-workdir lifecycle, subprocess execution,
stdout/stderr log parsing, non-zero exit handling, output file collection,
and shutil.move into the destination directory.

Parameters
----------
subcommand:
    The ``sfg`` subcommand to invoke (e.g. ``"nov0002rnx"``).
    The binary is located via ``find_binary()`` at construction time so
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

**Methods**

### `GoBinaryRunner.run(self, input_files: 'list[Path | str] | Path | str', output_dir: 'Path | str', site: 'str', *, setup_fn: 'Callable[[Path], list[str]] | None' = None, extra_flags: 'list[str] | None' = None) -> 'list[Path]'`

Convert *input_files* and return paths of output files in *output_dir*.

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

### `GoBinaryRunner.run_raw(self, input_files: 'list[Path | str] | Path | str', *, extra_flags: 'list[str] | None' = None, cwd: 'Path | None' = None) -> 'subprocess.CompletedProcess'`

Run the binary without output-file collection.

No temp workdir is created.  Intended for QC tools whose useful
output is stdout/stderr rather than produced files.

Args:
    input_files: One or more input file paths.
    extra_flags: Optional CLI flags inserted before file arguments.
    cwd: Working directory for the subprocess. Defaults to ``'.'``.

Returns:
    The raw ``subprocess.CompletedProcess`` from the binary.


## class `SubprocessRunner`

Anything that runs a command and returns a CompletedProcess.

Inject a fake implementation in tests to avoid hitting real binaries.
