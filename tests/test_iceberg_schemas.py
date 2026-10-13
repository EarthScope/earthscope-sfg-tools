"""Tests for the GNSS-A Iceberg table schemas."""

import re
from datetime import UTC, datetime
from pathlib import Path

import pytest

pytest.importorskip("pyiceberg")

from earthscope_sfg_tools.datamodels.iceberg.schemas import (  # noqa: E402
    COMMUNITY_STANDARD_L2,
    COMMUNITY_STANDARD_L3,
    GARPOS_L2,
    GARPOS_L3,
    GNSS_OBSERVATIONS,
    LEVEL1_TABLES,
    SCOPE_NAMES,
    SOUND_SPEED,
    TABLES,
    TRAVEL_TIMES,
    Scope,
)


@pytest.mark.parametrize("table", TABLES.values(), ids=lambda t: t.qualified_name)
def test_schema_and_partition_spec_build(table):
    schema = table.schema
    assert [f.name for f in schema.fields] == table.column_names
    assert [f.field_id for f in schema.fields] == list(range(1, len(table.columns) + 1))
    # Binds cleanly against the schema (raises on a bad source_id).
    table.partition_spec.partition_type(schema)
    for f in table.sort_order.fields:
        schema.find_field(f.source_id)


@pytest.mark.parametrize(
    "table",
    [t for t in TABLES.values() if t is not GNSS_OBSERVATIONS],
    ids=lambda t: t.qualified_name,
)
def test_default_partitioning_is_scope_identity(table):
    assert [f.name for f in table.partition_spec.fields] == list(SCOPE_NAMES)


@pytest.mark.parametrize("table", TABLES.values(), ids=lambda t: t.qualified_name)
def test_log_source_only_on_level1(table):
    assert ("Log_Source" in table.column_names) == (table in LEVEL1_TABLES)


def test_constraints_carried_on_columns():
    assert TRAVEL_TIMES["TravelTime"].le == 600
    assert TRAVEL_TIMES["tat"].required is False
    assert TRAVEL_TIMES.schema.find_field("MT_ID").required is True


def test_to_arrow_stamps_scope_and_log_source():
    scope = Scope("N", "S", "C")
    rows = [{"time": datetime(2025, 1, 1, tzinfo=UTC), "depth": 1.0, "speed": 1500.0}]
    table = SOUND_SPEED.to_arrow(rows, scope, "CTD")
    assert table.schema.equals(SOUND_SPEED.schema.as_arrow())
    assert table.to_pylist()[0] == {
        **rows[0],
        "Network": "N",
        "Station": "S",
        "Campaign": "C",
        "Log_Source": "CTD",
    }
    with pytest.raises(ValueError, match="Log_Source"):
        SOUND_SPEED.to_arrow(rows, scope)


GOLDEN = Path(__file__).parent / "data" / "gnssiceberg_observations.as-created.golden"


def _golden_columns() -> list[tuple[int, str, bool, str]]:
    """(id, name, required, type) rows of the golden's schema section."""
    rows = []
    for line in GOLDEN.read_text().splitlines():
        if line.startswith("## partition spec"):
            break
        m = re.match(r"^(\d+)\s+(\S+)\s+(true|false)\s+(\S+)\s*$", line)
        if m:
            rows.append((int(m[1]), m[2], m[3] == "true", m[4]))
    return rows


def test_gnss_observations_extends_gnsstools_schema():
    golden = _golden_columns()
    assert len(golden) == 20
    ours = [
        (f.field_id, f.name, f.required, str(f.field_type))
        for f in GNSS_OBSERVATIONS.schema.fields
    ]
    assert ours[: len(golden)] == golden
    assert [name for _, name, _, _ in ours[len(golden) :]] == [
        "Network",
        "Campaign",
        "Log_Source",
    ]


def test_gnss_observations_layout():
    spec = GNSS_OBSERVATIONS.partition_spec
    assert [(f.name, str(f.transform)) for f in spec.fields] == [
        ("Network", "identity"),
        ("station_id", "identity"),
        ("Campaign", "identity"),
        ("epoch_day", "day"),
    ]
    schema = GNSS_OBSERVATIONS.schema
    assert [
        schema.find_field(f.source_id).name for f in GNSS_OBSERVATIONS.sort_order.fields
    ] == ["station_id", "session_id", "epoch", "constellation", "satellite", "signal"]


def test_gnss_observations_to_arrow_stamps_station_id():
    row = {
        "session_id": "NOV000_20250822.bin",
        "epoch": datetime(2025, 8, 22, tzinfo=UTC),
        "epoch_pico": 0,
        "constellation": "GPS",
        "satellite": 5,
        "signal": "1C",
        "loss_of_lock": False,
        "half_cycle_ambiguity": False,
        "boc_tracking": False,
    }
    out = GNSS_OBSERVATIONS.to_arrow([row], Scope("N", "GCC1", "C"), "NOV000")
    stamped = out.to_pylist()[0]
    assert (stamped["station_id"], stamped["Network"], stamped["Campaign"]) == (
        "GCC1",
        "N",
        "C",
    )
    assert "Station" not in out.column_names


@pytest.mark.parametrize("table", TABLES.values(), ids=lambda t: t.qualified_name)
def test_tables_create_in_sql_catalog(table, tmp_path):
    pytest.importorskip("sqlalchemy")
    from pyiceberg.catalog.sql import SqlCatalog

    catalog = SqlCatalog(
        "test",
        uri=f"sqlite:///{tmp_path / 'catalog.db'}",
        warehouse=f"file://{tmp_path / 'warehouse'}",
    )
    catalog.create_namespace(table.namespace)
    created = catalog.create_table(
        table.identifier,
        schema=table.schema,
        partition_spec=table.partition_spec,
        sort_order=table.sort_order,
    )
    assert [f.name for f in created.schema().fields] == table.column_names
    assert len(created.spec().fields) == len(table.partition_by)


def test_levels_map_to_namespaces():
    assert TRAVEL_TIMES.qualified_name == "gnssa_l1.travel_times"
    assert GARPOS_L2.identifier == ("gnssa_l2", "garpos")
    assert GARPOS_L3.identifier == ("gnssa_l3", "garpos")
    assert COMMUNITY_STANDARD_L2.name == COMMUNITY_STANDARD_L3.name
    assert {t.namespace for t in LEVEL1_TABLES} == {"gnssa_l1"}


def test_scope_filter_uses_table_scope_columns():
    expr = GNSS_OBSERVATIONS.scope_filter("N", "GCC1", None)
    assert {ref.name for ref in _refs(expr)} == {"Network", "station_id"}
    assert {ref.name for ref in _refs(SOUND_SPEED.scope_filter(station="S"))} == {
        "Station"
    }
    assert str(SOUND_SPEED.scope_filter()) == "AlwaysTrue()"


def _refs(expr):
    """Unbound references in an And-tree of predicates."""
    if hasattr(expr, "term"):
        return [expr.term]
    return [*_refs(expr.left), *_refs(expr.right)]
