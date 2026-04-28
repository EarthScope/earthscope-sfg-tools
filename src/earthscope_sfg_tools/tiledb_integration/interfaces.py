"""Interfaces for TileDB integration backends."""

from __future__ import annotations

from typing import Optional, Protocol

from .models import TileDBOperationResult


class TileDBBackend(Protocol):
    """Backend contract for TileDB operations."""

    def write_ascii(
        self,
        input_files: list[str] | str,
        tdb_path: str,
        num_procs: int = 10,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        """Write NovAtel ASCII logs into a TileDB array."""

    def write_binary(
        self,
        input_files: list[str] | str,
        tdb_path: str,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        """Write NovAtel binary logs into a TileDB array."""

    def export_rinex(
        self,
        tdb_path: str,
        settings_file: str,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        """Export TileDB observations to RINEX."""
