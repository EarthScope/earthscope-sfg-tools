"""
Iceberg table schemas for the GNSS-A data lake.

Each table is a :class:`TableDef`; use ``.schema``, ``.partition_spec`` and
``.sort_order`` to create it in a catalog, ``.to_arrow()`` to build rows for
it, and the per-column constraints to validate rows at ingest. Level 0
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
    COMMUNITY_STANDARD_L1,
    COMMUNITY_STANDARD_L2,
    GARPOS_L1,
    GARPOS_L2,
    GNSS_ANTENNA_POSITIONS_PPP,
    LEVEL1_TABLES,
    LEVEL2_TABLES,
)
from .level0 import (
    ATTITUDE_AHRS,
    GNSS_ANTENNA_POSITIONS,
    GNSS_OBSERVATIONS,
    INSPVA,
    INSSTDEV,
    LEVEL0_TABLES,
    SOUND_SPEED,
    TRAVEL_TIMES,
)

# Keyed by qualified name: L2 and L3 tables share bare names.
TABLES: dict[str, TableDef] = {
    t.qualified_name: t for t in (*LEVEL0_TABLES, *LEVEL1_TABLES, *LEVEL2_TABLES)
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
    "LEVEL0_TABLES",
    "LEVEL1_TABLES",
    "LEVEL2_TABLES",
    "TRAVEL_TIMES",
    "INSPVA",
    "INSSTDEV",
    "GNSS_OBSERVATIONS",
    "ATTITUDE_AHRS",
    "SOUND_SPEED",
    "GNSS_ANTENNA_POSITIONS",
    "GNSS_ANTENNA_POSITIONS_PPP",
    "GARPOS_L1",
    "GARPOS_L2",
    "COMMUNITY_STANDARD_L1",
    "COMMUNITY_STANDARD_L2",
]
