import platform


def get_system_architecture() -> tuple[str, str]:
    """Return the normalised OS name and CPU architecture.

    Maps Python's ``platform`` output to the naming convention used by
    the Go binary build system (e.g. ``x86_64`` → ``amd64``).

    Returns:
        A ``(system, arch)`` tuple such as ``('darwin', 'arm64')`` or
        ``('linux', 'amd64')``.

    Raises:
        ValueError: If the platform or architecture is not supported.
    """
    system = platform.system().lower()
    arch = platform.machine().lower()
    if arch == "x86_64":
        arch = "amd64"
    if system not in ["darwin", "linux"]:
        raise ValueError(f"Unsupported platform: {system}")
    if arch not in ["amd64", "arm64"]:
        raise ValueError(f"Unsupported architecture: {arch}")

    return system, arch


class DYLDLibraryException(Exception):
    """Exception raised when the DYLD_LIBRARY_PATH environment variable is not set."""

    def __init__(
        self,
        message="\nLibrary not loaded: @rpath/libtiledb.dylib \nDYLD_LIBRARY_PATH does not include TileDB dylib file. Hint: $ export DYLD_LIBRARY_PATH=$CONDA_PREFIX/lib:$DYLD_LIBRARY_PATH",
    ):
        super().__init__(message)


class LDLibraryException(Exception):
    """Exception raised when the LD_LIBRARY_PATH environment variable is not set."""

    def __init__(
        self,
        message="\nLibrary not loaded: @rpath/libtiledb.dylib \nLD_LIBRARY_PATH does not include TileDB tile.h file. Hint: $ export LD_LIBRARY_PATH=$CONDA_PREFIX/lib:$LD_LIBRARY_PATH",
    ):
        super().__init__(message)


EXCEPTIONS_DICT_LINUX = {
    "Library not loaded: @rpath/libtiledb.dylib ": LDLibraryException
}
EXCEPTIONS_DICT_MACOS = {
    "Library not loaded: @rpath/libtiledb.dylib ": DYLDLibraryException
}


def raise_exception(string: str) -> Exception | None:
    """Return the exception class matching an error string, or ``None``.

    Looks up the stripped input against the platform-appropriate
    exceptions dictionary.  The caller is responsible for raising the
    returned class.

    Args:
        string: Error text to test, typically from subprocess stderr.

    Returns:
        The matching exception class (e.g. ``DYLDLibraryException``),
        or ``None`` if no pattern matches.
    """
    string = string.strip()
    sys, _ = get_system_architecture()
    if sys == "linux":
        exceptions_dict = EXCEPTIONS_DICT_LINUX
    else:
        exceptions_dict = EXCEPTIONS_DICT_MACOS

    for key, exception in exceptions_dict.items():
        if key.strip() in string:
            return exception
    return None
