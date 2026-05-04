"""TileDB integration module.

This module provides a deep boundary for TileDB workflows with:
- backend abstraction
- structured operation results
- service-level failure policy
- legacy-compatible helper functions
"""

try:
    import tiledb  # noqa: F401
except ImportError as e:
    raise ImportError(
        "tiledb is required for earthscope_sfg_tools.tiledb_integration. "
        "Install it with: pip install earthscope-sfg-tools[tiledb]"
    ) from e



__all__ = [
    "TileDBService",
    "TileDBOperationResult",
    "TileDBIntegrationError",
    "TileDBBinaryExecutionError",
    "GoBinaryTileDBBackend",
    "nova2tile",
    "nov0002tile",
    "novatel_770_2tile",
    "tdb2rnx",
    "tile2rinex",
]
