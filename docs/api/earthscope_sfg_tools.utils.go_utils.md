# go_utils

`earthscope_sfg_tools.utils.go_utils`

_No docstring._

## class `BinaryNotFoundError`

Raised when a required Go binary cannot be found.

## `find_binary(name: str = 'sfg') -> pathlib.Path`

Find the ``sfg`` Go binary.

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

## `parse_cli_logs(result: subprocess.CompletedProcess, logger: logging.Logger)`

Parse and forward stdout/stderr from a Go binary subprocess.

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

## `parse_error(string: str) -> Warning | None`

Return the warning class matching an error string, or ``None``.

Args:
    string: Text to test against the ``WARNINGS_DICT`` keys.

Returns:
    The matching ``Warning`` subclass, or ``None``.

## `remove_ansi_escape(text)`

Strip ANSI escape sequences from a string.

Args:
    text: Raw text that may contain terminal colour/control codes.

Returns:
    The input string with all ANSI escape sequences removed.
