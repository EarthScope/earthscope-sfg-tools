"""Error types for TileDB integration."""

from __future__ import annotations

from .models import TileDBOperationResult


class TileDBIntegrationError(RuntimeError):
    """Base error for TileDB integration workflows."""


class TileDBBinaryExecutionError(TileDBIntegrationError):
    """Raised when a TileDB command exits with a non-zero status."""

    def __init__(self, result: TileDBOperationResult):
        self.result = result
        message = (
            f"TileDB operation '{result.operation}' failed via {result.backend} "
            f"(exit_code={result.exit_code}, target='{result.target}')"
        )
        super().__init__(message)
