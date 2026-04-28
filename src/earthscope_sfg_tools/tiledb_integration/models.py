"""Domain models for TileDB integration operations."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class TileDBOperationResult:
    """Result metadata for an invoked TileDB operation."""

    operation: str
    target: str
    exit_code: int
    backend: str

    @property
    def success(self) -> bool:
        """Whether the operation completed successfully."""
        return self.exit_code == 0
