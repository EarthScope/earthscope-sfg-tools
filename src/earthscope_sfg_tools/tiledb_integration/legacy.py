"""Legacy-compatible TileDB helper functions.

These helpers keep the return-code-oriented workflow that existing code expects,
while routing through the new TileDBService architecture.
"""

from __future__ import annotations

from typing import Optional

from .service import TileDBService


def nova2tile(
    input_files: list[str] | str,
    tdb_path: str,
    num_procs: int = 10,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Legacy-compatible wrapper for ASCII -> TileDB ingest."""
    service = TileDBService(raise_on_failure=False)
    result = service.ingest_ascii(
        input_files=input_files,
        tdb_path=tdb_path,
        num_procs=num_procs,
        search_paths=search_paths,
    )
    return result.exit_code


def nov0002tile(
    input_files: list[str] | str,
    tdb_path: str,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Legacy-compatible wrapper for binary -> TileDB ingest."""
    service = TileDBService(raise_on_failure=False)
    result = service.ingest_binary(
        input_files=input_files,
        tdb_path=tdb_path,
        search_paths=search_paths,
    )
    return result.exit_code


def tdb2rnx(
    tdb_path: str,
    settings_file: str,
    search_paths: Optional[list[str]] = None,
) -> int:
    """Legacy-compatible wrapper for TileDB -> RINEX export."""
    service = TileDBService(raise_on_failure=False)
    result = service.export_to_rinex(
        tdb_path=tdb_path,
        settings_file=settings_file,
        search_paths=search_paths,
    )
    return result.exit_code
