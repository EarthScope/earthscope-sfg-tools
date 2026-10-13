"""
Iceberg table schemas for the GNSS-A data lake.

Each table is a :class:`TableDef`; use ``.schema``, ``.partition_spec`` and
``.sort_order`` to create it in a catalog, ``.to_arrow()`` to build rows for
it, and the per-column constraints to validate rows at ingest. Level 1
parsers live in the ``*_tools`` packages and return Arrow tables built with
``.to_arrow()``. See plans/iceberg-gnatss-proposal.md for the design.
"""

from ._base import (
    NAMESPACE_PREFIX,
    SCOPE_NAMES,
    Column,
    PartitionKey,
    Scope,
    TableDef,
    level_namespace,
)
from .derived import (
    COMMUNITY_STANDARD_L2,
    COMMUNITY_STANDARD_L3,
    GARPOS_L2,
    GARPOS_L3,
    GNSS_ANTENNA_POSITIONS_PPP,
    LEVEL2_TABLES,
    LEVEL3_TABLES,
)
from .level1 import (
    ATTITUDE_AHRS,
    GNSS_ANTENNA_POSITIONS,
    GNSS_OBSERVATIONS,
    INSPVA,
    INSSTDEV,
    LEVEL1_TABLES,
    SOUND_SPEED,
    TRAVEL_TIMES,
)

# Keyed by qualified name: L2 and L3 tables share bare names.
TABLES: dict[str, TableDef] = {
    t.qualified_name: t for t in (*LEVEL1_TABLES, *LEVEL2_TABLES, *LEVEL3_TABLES)
}

__all__ = [
    "NAMESPACE_PREFIX",
    "SCOPE_NAMES",
    "level_namespace",
    "PartitionKey",
    "Column",
    "Scope",
    "TableDef",
    "TABLES",
    "LEVEL1_TABLES",
    "LEVEL2_TABLES",
    "LEVEL3_TABLES",
    "TRAVEL_TIMES",
    "INSPVA",
    "INSSTDEV",
    "GNSS_OBSERVATIONS",
    "ATTITUDE_AHRS",
    "SOUND_SPEED",
    "GNSS_ANTENNA_POSITIONS",
    "GNSS_ANTENNA_POSITIONS_PPP",
    "GARPOS_L2",
    "GARPOS_L3",
    "COMMUNITY_STANDARD_L2",
    "COMMUNITY_STANDARD_L3",
]
