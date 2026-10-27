from datetime import datetime, timedelta, timezone

import pytest
from pyiceberg.catalog.sql import SqlCatalog

from earthscope_sfg_tools.catalog import Level1Reader, open_catalog, stream_edid
from earthscope_sfg_tools.datamodels.iceberg.schemas import INSPVA, TRAVEL_TIMES, Scope

SCOPE = Scope("cascadia-gorda", "NBR1", "2025_A_1126")
OTHER = Scope("cascadia-gorda", "NCC1", "2025_A_1126")
T0 = datetime(2025, 9, 15, tzinfo=timezone.utc)


def _inspva(scope, n, log_source="INSPVAA", file_type="NOV000"):
    return INSPVA.to_arrow(
        [
            {
                "time": T0 + timedelta(seconds=i),
                "lat": 42.0, "lon": -125.0, "alt": -30.0,
                "north": 0.0, "east": 0.0, "up": 0.0,
                "roll": 0.0, "pitch": 0.0, "heading": float(i),
                "ins_status": "INS_SOLUTION_GOOD", "time_status": "FINESTEERING",
                "stream_edid": stream_edid(scope, file_type, log_source),
            }
            for i in reversed(range(n))  # written out of order; read sorts
        ],
        scope,
        log_source,
    )  # fmt: skip


@pytest.fixture
def warehouse(tmp_path):
    cat = SqlCatalog(
        "default", uri=f"sqlite:///{tmp_path}/catalog.db", warehouse=f"file://{tmp_path}"
    )
    for tdef in (INSPVA, TRAVEL_TIMES):
        cat.create_namespace_if_not_exists(tdef.namespace)
        cat.create_table(
            f"{tdef.namespace}.{tdef.name}", tdef.schema, partition_spec=tdef.partition_spec
        )
    tbl = cat.load_table(f"{INSPVA.namespace}.{INSPVA.name}")
    tbl.append(_inspva(SCOPE, 10))
    tbl.append(_inspva(OTHER, 4))
    tbl.append(_inspva(SCOPE, 3, file_type="OTHER"))
    return tmp_path


def test_read_scope_window_and_order(warehouse):
    r = Level1Reader(open_catalog(warehouse))
    df = r.read("inspva", SCOPE, T0 + timedelta(seconds=2), T0 + timedelta(seconds=5), file_type="NOV000")
    assert list(df["heading"]) == [2.0, 3.0, 4.0]  # end exclusive, sorted by time
    assert (df["Station"] == "NBR1").all()
    assert str(df["time"].dt.tz) == "UTC"


def test_read_streams_and_columns(warehouse):
    r = Level1Reader(open_catalog(warehouse))
    assert len(r.read("inspva", SCOPE)) == 13  # both file types
    assert len(r.read("inspva", SCOPE, file_type="OTHER")) == 3
    df = r.read("inspva", SCOPE, file_type="NOV000", log_source="INSPVAA", columns=["time", "ins_status"])
    assert list(df.columns) == ["time", "ins_status"] and len(df) == 10
    assert r.read("travel_times", SCOPE).empty


def test_errors(tmp_path, warehouse):
    with pytest.raises(FileNotFoundError):
        open_catalog(tmp_path / "missing")
    r = Level1Reader(open_catalog(warehouse))
    with pytest.raises(KeyError):
        r.read("garpos", SCOPE)
    with pytest.raises(ValueError):
        r.read("inspva", SCOPE, log_source="INSPVAA")
