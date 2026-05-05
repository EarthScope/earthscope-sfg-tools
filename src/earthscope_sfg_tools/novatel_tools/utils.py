"""Metadata utilities for RINEX conversion.

``RinexMetadata`` is the canonical class.  ``MetadataModel`` is a backward-
compatible alias.  The helper functions below are thin shims kept for import
compatibility; prefer ``RinexMetadata.load()`` and ``RinexMetadata.write()``
in new code.
"""

import uuid
from pathlib import Path

from .rinex_metadata import RinexMetadata

# Backward-compatible alias — existing ``from .utils import MetadataModel`` keeps working.
MetadataModel = RinexMetadata


# ---------------------------------------------------------------------------
# Legacy helpers (kept for import compatibility; delegate to RinexMetadata)
# ---------------------------------------------------------------------------


def get_metadatav2(
    site: str,
    serialNumber: str = "XXXXXXXXXX",
    antennaPosition: list = None,
    antennaeOffsetHEN: list = None,
) -> dict:
    """Build a RinexMetadata dict with sensible defaults.

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
    """
    if antennaPosition is None:
        antennaPosition = [0, 0, 0]
    if antennaeOffsetHEN is None:
        antennaeOffsetHEN = [0, 0, 0]
    return RinexMetadata(
        marker_name=site,
        receiver_serial=serialNumber,
        antenna_position=antennaPosition,
        antenna_offsetHEN=antennaeOffsetHEN,
    ).model_dump()


# Backward-compatible alias for legacy callers using ``get_metadata``.
get_metadata = get_metadatav2


def check_metadata_path(metadata_path: Path | str) -> str:
    """Validate a JSON metadata file path and return it as a string.

    Args:
        metadata_path: Path to an existing JSON metadata file.

    Returns:
        The path as a ``str``.

    Raises:
        FileNotFoundError: If the file does not exist.
        pydantic.ValidationError: If the file fails schema validation.
    """
    RinexMetadata.load(metadata_path)  # validates; raises on bad schema or missing file
    return str(metadata_path)


def check_metadata(meta: dict | RinexMetadata) -> dict:
    """Validate a metadata dict or model and return a plain dict.

    Args:
        meta: A raw metadata ``dict`` or an existing
            :class:`RinexMetadata` instance.

    Returns:
        Validated metadata as a plain ``dict``.

    Raises:
        pydantic.ValidationError: If the data fails schema validation.
    """
    return RinexMetadata.load(meta).model_dump()


def resolve_metadata(
    metadata: "dict | RinexMetadata | Path | str | None" = None,
    site: str | None = None,
) -> RinexMetadata:
    """Resolve and validate metadata from any supported source.

    Thin shim around :meth:`RinexMetadata.load` kept for import
    compatibility.

    Args:
        metadata: A ``dict``, JSON file path, existing
            :class:`RinexMetadata`, or ``None`` (requires ``site``).
        site: 4-character site code used when ``metadata`` is ``None``.

    Returns:
        A validated :class:`RinexMetadata` instance.
    """
    return RinexMetadata.load(metadata, site=site)


def write_metadata_json(metadata: dict, output_path: Path | str) -> Path:
    """Write a metadata dict to a JSON file and return the path.

    Args:
        metadata: Plain ``dict`` of RINEX metadata fields.
        output_path: Destination file path (parent must exist).

    Returns:
        The resolved ``Path`` of the written file.
    """
    return RinexMetadata.load(metadata).write(output_path)
