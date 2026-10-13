"""
Column and table definitions shared by the GNSS-A Iceberg schemas.

Iceberg only stores types and nullability, so each :class:`Column` also
carries the value constraints from the proposal (``ge``/``le``/``lt``/
``isin``) for the ingest layer to enforce, and a ``default`` that the
access API applies when building derived rows.

Field IDs are assigned in declaration order starting at 1. Iceberg tracks
columns by ID, so new columns must only ever be appended to a table
definition — never inserted or reordered.

By default a table is partitioned by identity on Network, Station and
Campaign. A table can override its partitioning, its sort order, and which
columns hold the scope values (see ``GNSS_OBSERVATIONS``, which follows the
gnsstools ``observations`` layout).

Each level is its own catalog namespace (``gnssa_l1``, ``gnssa_l2``,
``gnssa_l3``), so a table's name only needs to be unique within its level.
"""

from dataclasses import dataclass, field
from functools import reduce
from typing import Any, Iterable

import pyarrow as pa
from pyiceberg.expressions import AlwaysTrue, And, BooleanExpression, EqualTo
from pyiceberg.partitioning import PartitionField, PartitionSpec
from pyiceberg.schema import Schema
from pyiceberg.table.sorting import NullOrder, SortDirection, SortField, SortOrder
from pyiceberg.transforms import IdentityTransform, Transform
from pyiceberg.types import (
    BooleanType,
    DoubleType,
    FloatType,
    IcebergType,
    IntegerType,
    LongType,
    NestedField,
    StringType,
    TimestamptzType,
)

STRING = StringType()
INT = IntegerType()
LONG = LongType()
FLOAT = FloatType()
DOUBLE = DoubleType()
BOOLEAN = BooleanType()
TIMESTAMPTZ = TimestamptzType()

# Default names of the columns holding the Network, Station, Campaign scope.
SCOPE_NAMES: tuple[str, str, str] = ("Network", "Station", "Campaign")

# Iceberg reserves IDs >= 1000 for partition fields by convention.
_PARTITION_FIELD_ID_START = 1000

NAMESPACE_PREFIX = "gnssa"


def level_namespace(level: int) -> str:
    """Catalog namespace holding the tables of ``level``, e.g. ``gnssa_l1``."""
    return f"{NAMESPACE_PREFIX}_l{level}"


@dataclass(frozen=True)
class Column:
    """A single table column plus the constraints enforced at ingest.

    Parameters
    ----------
    name : str
        Column name.
    type : IcebergType
        Iceberg primitive type.
    doc : str
        Column description (stored as the Iceberg field doc).
    required : bool
        Whether the column is non-nullable. Required columns can later be
        relaxed to optional via schema evolution; the reverse is not allowed.
    ge, le, gt, lt : float or None
        Inclusive / exclusive bounds on the value.
    isin : tuple or None
        Allowed values.
    default : Any
        Value the access API fills in when building derived rows.
    """

    name: str
    type: IcebergType
    doc: str
    required: bool = True
    ge: float | None = None
    le: float | None = None
    gt: float | None = None
    lt: float | None = None
    isin: tuple[Any, ...] | None = None
    default: Any = None


@dataclass(frozen=True)
class PartitionKey:
    """One partition field: a transform applied to a source column."""

    source: str
    transform: Transform = IdentityTransform()
    name: str | None = None  # defaults to the source column name


def _identity_partitions(names: Iterable[str]) -> tuple[PartitionKey, ...]:
    return tuple(PartitionKey(n) for n in names)


@dataclass(frozen=True)
class Scope:
    """Partition scope stamped onto every row: Network, Station, Campaign."""

    network: str
    station: str
    campaign: str

    def as_columns(self, names: tuple[str, str, str] = SCOPE_NAMES) -> dict[str, str]:
        """Scope values keyed by the (network, station, campaign) column names."""
        return dict(zip(names, (self.network, self.station, self.campaign)))


@dataclass(frozen=True)
class TableDef:
    """An Iceberg table definition: name, level, ordered columns and layout.

    Parameters
    ----------
    scope_names : tuple of str
        Names of the columns holding the (network, station, campaign) scope.
    partition_by : tuple of PartitionKey, optional
        Partition fields; defaults to identity on ``scope_names``.
    sort_by : tuple of str
        Columns of the table's sort order (ascending, nulls first).
    """

    name: str
    level: int
    doc: str
    columns: tuple[Column, ...]
    scope_names: tuple[str, str, str] = SCOPE_NAMES
    partition_by: tuple[PartitionKey, ...] | None = None
    sort_by: tuple[str, ...] = ()
    _by_name: dict[str, Column] = field(init=False, repr=False, compare=False)

    def __post_init__(self) -> None:
        by_name = {c.name: c for c in self.columns}
        if len(by_name) != len(self.columns):
            raise ValueError(f"Duplicate column names in table {self.name!r}")
        if self.partition_by is None:
            object.__setattr__(
                self, "partition_by", _identity_partitions(self.scope_names)
            )
        referenced = {
            *self.scope_names,
            *(p.source for p in self.partition_by),
            *self.sort_by,
        }
        missing = sorted(referenced - by_name.keys())
        if missing:
            raise ValueError(
                f"Table {self.name!r} references unknown columns {missing}"
            )
        object.__setattr__(self, "_by_name", by_name)

    def __getitem__(self, name: str) -> Column:
        return self._by_name[name]

    @property
    def column_names(self) -> list[str]:
        return [c.name for c in self.columns]

    @property
    def namespace(self) -> str:
        """Catalog namespace of this table's level."""
        return level_namespace(self.level)

    @property
    def identifier(self) -> tuple[str, str]:
        """Catalog identifier ``(namespace, name)``."""
        return (self.namespace, self.name)

    @property
    def qualified_name(self) -> str:
        """Dotted catalog name, e.g. ``gnssa_l2.garpos``."""
        return f"{self.namespace}.{self.name}"

    def scope_filter(
        self,
        network: str | None = None,
        station: str | None = None,
        campaign: str | None = None,
    ) -> BooleanExpression:
        """Row filter on this table's scope columns; ``None`` fields match all."""
        values = (network, station, campaign)
        return reduce(
            And,
            (
                EqualTo(name, value)
                for name, value in zip(self.scope_names, values)
                if value is not None
            ),
            AlwaysTrue(),
        )

    @property
    def schema(self) -> Schema:
        """The Iceberg schema, with field IDs assigned in declaration order."""
        fields = [
            NestedField(
                field_id=i,
                name=c.name,
                field_type=c.type,
                required=c.required,
                doc=c.doc,
            )
            for i, c in enumerate(self.columns, start=1)
        ]
        return Schema(*fields)

    def to_arrow(
        self,
        rows: Iterable[dict[str, Any]],
        scope: Scope,
        log_source: str | None = None,
    ) -> pa.Table:
        """Build an Arrow table with this table's schema from parsed rows.

        Scope columns (and ``Log_Source`` for Level 1 tables) are stamped onto
        every row; the rows supply everything else.
        """
        stamp: dict[str, Any] = scope.as_columns(self.scope_names)
        if "Log_Source" in self._by_name:
            if log_source is None:
                raise ValueError(f"Table {self.name!r} requires a Log_Source")
            stamp["Log_Source"] = log_source
        return pa.Table.from_pylist(
            [{**row, **stamp} for row in rows], schema=self.schema.as_arrow()
        )

    @property
    def partition_spec(self) -> PartitionSpec:
        """The partition spec built from ``partition_by``."""
        schema = self.schema
        return PartitionSpec(
            *(
                PartitionField(
                    source_id=schema.find_field(key.source).field_id,
                    field_id=_PARTITION_FIELD_ID_START + i,
                    transform=key.transform,
                    name=key.name or key.source,
                )
                for i, key in enumerate(self.partition_by)
            )
        )

    @property
    def sort_order(self) -> SortOrder:
        """Ascending, nulls-first sort on ``sort_by``; unsorted when empty."""
        schema = self.schema
        return SortOrder(
            *(
                SortField(
                    source_id=schema.find_field(name).field_id,
                    transform=IdentityTransform(),
                    direction=SortDirection.ASC,
                    null_order=NullOrder.NULLS_FIRST,
                )
                for name in self.sort_by
            )
        )


SCOPE_COLUMNS: tuple[Column, ...] = (
    Column("Network", STRING, "Network identifier"),
    Column("Station", STRING, "Station identifier"),
    Column("Campaign", STRING, "Campaign identifier"),
)

LOG_SOURCE = Column("Log_Source", STRING, "Source log file or log type")

LEVEL1_SCOPE: tuple[Column, ...] = (*SCOPE_COLUMNS, LOG_SOURCE)
