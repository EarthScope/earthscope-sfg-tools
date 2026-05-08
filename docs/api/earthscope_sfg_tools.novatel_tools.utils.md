# utils

`earthscope_sfg_tools.novatel_tools.utils`

Metadata utilities for RINEX conversion.

``RinexMetadata`` is the canonical class.  ``MetadataModel`` is a backward-
compatible alias.  The helper functions below are thin shims kept for import
compatibility; prefer ``RinexMetadata.load()`` and ``RinexMetadata.write()``
in new code.

## `check_metadata(meta: dict | earthscope_sfg_tools.novatel_tools.rinex_metadata.RinexMetadata) -> dict`

Validate a metadata dict or model and return a plain dict.

Args:
    meta: A raw metadata ``dict`` or an existing
        :class:`RinexMetadata` instance.

Returns:
    Validated metadata as a plain ``dict``.

Raises:
    pydantic.ValidationError: If the data fails schema validation.

## `check_metadata_path(metadata_path: pathlib.Path | str) -> str`

Validate a JSON metadata file path and return it as a string.

Args:
    metadata_path: Path to an existing JSON metadata file.

Returns:
    The path as a ``str``.

Raises:
    FileNotFoundError: If the file does not exist.
    pydantic.ValidationError: If the file fails schema validation.

## `get_metadata(site: str, serialNumber: str = 'XXXXXXXXXX', antennaPosition: list = None, antennaeOffsetHEN: list = None) -> dict`

Build a RinexMetadata dict with sensible defaults.

.. deprecated::
    Prefer ``RinexMetadata.load()`` in new code.

Args:
    site: 4-character marker name.
    serialNumber: Receiver serial number. Defaults to
        ``'XXXXXXXXXX'``.
    antennaPosition: ``[X, Y, Z]`` reference position in metres.
        Defaults to ``[0, 0, 0]``.
    antennaeOffsetHEN: ``[H, E, N]`` antenna offset in metres.
        Defaults to ``[0, 0, 0]``.

Returns:
    Validated metadata as a plain ``dict``.

## `get_metadatav2(site: str, serialNumber: str = 'XXXXXXXXXX', antennaPosition: list = None, antennaeOffsetHEN: list = None) -> dict`

Build a RinexMetadata dict with sensible defaults.

.. deprecated::
    Prefer ``RinexMetadata.load()`` in new code.

Args:
    site: 4-character marker name.
    serialNumber: Receiver serial number. Defaults to
        ``'XXXXXXXXXX'``.
    antennaPosition: ``[X, Y, Z]`` reference position in metres.
        Defaults to ``[0, 0, 0]``.
    antennaeOffsetHEN: ``[H, E, N]`` antenna offset in metres.
        Defaults to ``[0, 0, 0]``.

Returns:
    Validated metadata as a plain ``dict``.

## `resolve_metadata(metadata: 'dict | RinexMetadata | Path | str | None' = None, site: str | None = None) -> earthscope_sfg_tools.novatel_tools.rinex_metadata.RinexMetadata`

Resolve and validate metadata from any supported source.

Thin shim around :meth:`RinexMetadata.load` kept for import
compatibility.

Args:
    metadata: A ``dict``, JSON file path, existing
        :class:`RinexMetadata`, or ``None`` (requires ``site``).
    site: 4-character site code used when ``metadata`` is ``None``.

Returns:
    A validated :class:`RinexMetadata` instance.

## `write_metadata_json(metadata: dict, output_path: pathlib.Path | str) -> pathlib.Path`

Write a metadata dict to a JSON file and return the path.

Args:
    metadata: Plain ``dict`` of RINEX metadata fields.
    output_path: Destination file path (parent must exist).

Returns:
    The resolved ``Path`` of the written file.
