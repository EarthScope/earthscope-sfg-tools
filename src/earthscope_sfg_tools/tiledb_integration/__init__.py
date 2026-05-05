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

# Re-export legacy TDB array classes and schemas so workflow code that used
# `earthscope_sfg.tiledb_schemas` continues to work via the new module path.
from .arrays import (  # noqa: E402
    TBDArray,
    TDBAcousticArray,
    TDBGNSSObsArray,
    TDBIMUPositionArray,
    TDBKinPositionArray,
    TDBShotDataArray,
)
from .schemas import (  # noqa: E402
    AcousticArraySchema,
    GNSSObsSchema,
    IMUPositionArraySchema,
    KinPositionArraySchema,
    ShotDataArraySchema,
    config,
    ctx,
    filters,
)


def _not_yet_implemented(name: str):
    """Build a placeholder callable for parser symbols not yet ported into
    earthscope-sfg-tools. These keep import-time compatibility for downstream
    consumers (e.g. earthscope-sfg-workflows) while the implementations are
    being moved over from the legacy ``earthscope_sfg`` package."""

    def _stub(*_args, **_kwargs):
        raise NotImplementedError(
            f"{name} has not yet been ported into earthscope_sfg_tools. "
            "Track progress in the migration plan."
        )

    _stub.__name__ = name
    _stub.__qualname__ = name
    return _stub


# TODO(earthscope-sfg-tools): implement these parser/converter helpers.
tile2rinex = _not_yet_implemented("tile2rinex")
rinex_qc = _not_yet_implemented("rinex_qc")
novatel_770_2tile = _not_yet_implemented("novatel_770_2tile")


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
    # Legacy-compatible TDB array classes
    "TBDArray",
    "TDBAcousticArray",
    "TDBGNSSObsArray",
    "TDBIMUPositionArray",
    "TDBKinPositionArray",
    "TDBShotDataArray",
    # Schema symbols
    "AcousticArraySchema",
    "GNSSObsSchema",
    "IMUPositionArraySchema",
    "KinPositionArraySchema",
    "ShotDataArraySchema",
    "config",
    "ctx",
    "filters",
]
