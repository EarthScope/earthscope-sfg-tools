"""Backend implementations for TileDB integration."""

from __future__ import annotations

from typing import Optional

from ..novatel_tools import go_binaries
from .models import TileDBOperationResult


class GoBinaryTileDBBackend:
    """TileDB backend that delegates to compiled Go binaries."""

    backend_name = "go-binary"

    def write_ascii(
        self,
        input_files: list[str] | str,
        tdb_path: str,
        num_procs: int = 10,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        exit_code = go_binaries.nova2tile(
            input_files=input_files,
            tdb_path=tdb_path,
            num_procs=num_procs,
            search_paths=search_paths,
        )
        return TileDBOperationResult(
            operation="nova2tile",
            target=tdb_path,
            exit_code=exit_code,
            backend=self.backend_name,
        )

    def write_binary(
        self,
        input_files: list[str] | str,
        tdb_path: str,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        exit_code = go_binaries.nov0002tile(
            input_files=input_files,
            tdb_path=tdb_path,
            search_paths=search_paths,
        )
        return TileDBOperationResult(
            operation="nov0002tile",
            target=tdb_path,
            exit_code=exit_code,
            backend=self.backend_name,
        )

    def export_rinex(
        self,
        tdb_path: str,
        settings_file: str,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        exit_code = go_binaries.tdb2rnx(
            tdb_path=tdb_path,
            settings_file=settings_file,
            search_paths=search_paths,
        )
        return TileDBOperationResult(
            operation="tdb2rnx",
            target=tdb_path,
            exit_code=exit_code,
            backend=self.backend_name,
        )
