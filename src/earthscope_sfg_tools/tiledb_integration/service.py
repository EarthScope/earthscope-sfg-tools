"""High-level service for TileDB operations."""

from __future__ import annotations

from typing import Optional

from .backends import GoBinaryTileDBBackend
from .errors import TileDBBinaryExecutionError
from .interfaces import TileDBBackend
from .models import TileDBOperationResult


class TileDBService:
    """Facade for TileDB ingest/export workflows.

    This deepens the previous shallow wrapper setup by centralizing:
    - operation naming and result shape
    - backend selection
    - failure policy
    """

    def __init__(
        self,
        backend: TileDBBackend | None = None,
        *,
        raise_on_failure: bool = True,
    ) -> None:
        self._backend = backend or GoBinaryTileDBBackend()
        self._raise_on_failure = raise_on_failure

    def ingest_ascii(
        self,
        input_files: list[str] | str,
        tdb_path: str,
        num_procs: int = 10,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        result = self._backend.write_ascii(
            input_files=input_files,
            tdb_path=tdb_path,
            num_procs=num_procs,
            search_paths=search_paths,
        )
        return self._enforce_policy(result)

    def ingest_binary(
        self,
        input_files: list[str] | str,
        tdb_path: str,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        result = self._backend.write_binary(
            input_files=input_files,
            tdb_path=tdb_path,
            search_paths=search_paths,
        )
        return self._enforce_policy(result)

    def export_to_rinex(
        self,
        tdb_path: str,
        settings_file: str,
        search_paths: Optional[list[str]] = None,
    ) -> TileDBOperationResult:
        result = self._backend.export_rinex(
            tdb_path=tdb_path,
            settings_file=settings_file,
            search_paths=search_paths,
        )
        return self._enforce_policy(result)

    def _enforce_policy(self, result: TileDBOperationResult) -> TileDBOperationResult:
        if self._raise_on_failure and not result.success:
            raise TileDBBinaryExecutionError(result)
        return result
