"""TileDB integration module.

This module provides a deep boundary for TileDB workflows with:
- backend abstraction
- structured operation results
- service-level failure policy
- legacy-compatible helper functions
"""

from .backends import GoBinaryTileDBBackend
from .errors import TileDBBinaryExecutionError, TileDBIntegrationError
from .legacy import nov0002tile, nova2tile, tdb2rnx
from .models import TileDBOperationResult
from .service import TileDBService

__all__ = [
    "TileDBService",
    "TileDBOperationResult",
    "TileDBIntegrationError",
    "TileDBBinaryExecutionError",
    "GoBinaryTileDBBackend",
    "nova2tile",
    "nov0002tile",
    "tdb2rnx",
]
