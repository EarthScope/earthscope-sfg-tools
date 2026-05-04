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


def check_metadata_path(metadata_path: Path | str) -> str:
    """Validate a JSON metadata file path and return it as a string."""
    RinexMetadata.load(metadata_path)  # validates; raises on bad schema or missing file
    return str(metadata_path)


def check_metadata(meta: dict | RinexMetadata) -> dict:
    """Validate a metadata dict or model and return a dict."""
    return RinexMetadata.load(meta).model_dump()


def resolve_metadata(
    metadata: "dict | RinexMetadata | Path | str | None" = None,
    site: str | None = None,
) -> RinexMetadata:
    """Resolve and validate metadata. Returns a RinexMetadata instance."""
    return RinexMetadata.load(metadata, site=site)


def write_metadata_json(metadata: dict, output_path: Path | str) -> Path:
    """Write a metadata dict to a JSON file and return the path."""
    return RinexMetadata.load(metadata).write(output_path)
