"""
Apache Iceberg integration for seafloor geodesy data.

Drop-in replacement for tiledb_integration. All array classes expose the same
write_df / read_df / get_unique_dates interface so pipeline callers need no changes.
"""

from .arrays import (
    IcebergAcousticArray,
    IcebergGNSSObsArray,
    IcebergIMUPositionArray,
    IcebergKinPositionArray,
    IcebergShotDataArray,
)
from .catalog import get_catalog
from .scope import Scope

__all__ = [
    "get_catalog",
    "Scope",
    "IcebergKinPositionArray",
    "IcebergAcousticArray",
    "IcebergIMUPositionArray",
    "IcebergShotDataArray",
    "IcebergGNSSObsArray",
]
