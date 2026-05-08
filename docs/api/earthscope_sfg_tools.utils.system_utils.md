# system_utils

`earthscope_sfg_tools.utils.system_utils`

_No docstring._

## class `DYLDLibraryException`

Exception raised when the DYLD_LIBRARY_PATH environment variable is not set.

## class `LDLibraryException`

Exception raised when the LD_LIBRARY_PATH environment variable is not set.

## `get_system_architecture() -> tuple[str, str]`

Return the normalised OS name and CPU architecture.

Maps Python's ``platform`` output to the naming convention used by
the Go binary build system (e.g. ``x86_64`` → ``amd64``).

Returns:
    A ``(system, arch)`` tuple such as ``('darwin', 'arm64')`` or
    ``('linux', 'amd64')``.

Raises:
    ValueError: If the platform or architecture is not supported.

## `raise_exception(string: str) -> Exception | None`

Return the exception class matching an error string, or ``None``.

Looks up the stripped input against the platform-appropriate
exceptions dictionary.  The caller is responsible for raising the
returned class.

Args:
    string: Error text to test, typically from subprocess stderr.

Returns:
    The matching exception class (e.g. ``DYLDLibraryException``),
    or ``None`` if no pattern matches.
