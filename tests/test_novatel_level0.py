"""Tests for the NovAtel Level 0 parsers (``sfg convert``) and conform_arrow."""

from datetime import UTC, datetime
from pathlib import Path

import pytest

pytest.importorskip("pyiceberg")

import pyarrow as pa  # noqa: E402

from earthscope_sfg_tools.datamodels.iceberg.schemas import (  # noqa: E402
    GNSS_OBSERVATIONS,
    INSSTDEV,
    SOUND_SPEED,
    Scope,
)
from earthscope_sfg_tools.utils.go_utils import (  # noqa: E402
    BinaryNotFoundError,
    find_binary,
)

DATA = Path(__file__).parent / "data"
NOV000 = DATA / "test_gcc1_20250821_205746_00062_NOV000.bin"
NOV770 = DATA / "test_20260331_174553_00030_NOV770.raw"
SCOPE = Scope("NCB", "GCC1", "2025_A_1126")
T0 = datetime(2025, 1, 1, tzinfo=UTC)


# ---------------------------------------------------------------------------
# conform_arrow
# ---------------------------------------------------------------------------


def test_conform_arrow_stamps_scope_and_reorders():
    parsed = pa.table({"speed": [1500.0], "depth": [1.0], "time": [T0]})
    table = SOUND_SPEED.conform_arrow(parsed, SCOPE, "CTD")
    assert table.schema.equals(SOUND_SPEED.schema.as_arrow())
    assert table.to_pylist() == [
        {
            "Network": "NCB",
            "Station": "GCC1",
            "Campaign": "2025_A_1126",
            "Log_Source": "CTD",
            "time": T0,
            "depth": 1.0,
            "speed": 1500.0,
            "stream_edid": None,
        }
    ]


def test_conform_arrow_fills_missing_nullable_columns():
    parsed = pa.table(
        {
            "time": [T0],
            **{c: [0.1] for c in INSSTDEV.column_names[5:14]},
        }
    )
    table = INSSTDEV.conform_arrow(parsed, SCOPE, "NOV000")
    assert table["ext_sol_stat"].null_count == 1
    assert table["time_since_update"].null_count == 1


def test_conform_arrow_uses_table_scope_column_names():
    table = GNSS_OBSERVATIONS.conform_arrow(_empty_observations(), SCOPE, "NOV000")
    assert table.schema.equals(GNSS_OBSERVATIONS.schema.as_arrow())


@pytest.mark.parametrize(
    "parsed, match",
    [
        (pa.table({"time": [T0], "depth": [1.0], "speed": [1.0], "x": [1]}), "unknown"),
        (
            pa.table({"time": [T0], "depth": [1.0], "speed": [1.0], "Station": ["S"]}),
            "stamped",
        ),
        (pa.table({"time": [T0], "depth": [1.0]}), "missing required column 'speed'"),
        (
            pa.table(
                {"time": [T0], "depth": [1.0], "speed": pa.array([None], pa.float64())}
            ),
            "'speed' has nulls",
        ),
    ],
)
def test_conform_arrow_rejects_schema_drift(parsed, match):
    with pytest.raises(ValueError, match=match):
        SOUND_SPEED.conform_arrow(parsed, SCOPE, "CTD")


def test_conform_arrow_requires_log_source():
    parsed = pa.table({"time": [T0], "depth": [1.0], "speed": [1500.0]})
    with pytest.raises(ValueError, match="Log_Source"):
        SOUND_SPEED.conform_arrow(parsed, SCOPE)


def _empty_observations() -> pa.Table:
    stamped = {"station_id", "Network", "Campaign", "Log_Source", "publish_time"}
    fields = [f for f in GNSS_OBSERVATIONS.schema.as_arrow() if f.name not in stamped]
    return pa.schema(fields).empty_table()


# ---------------------------------------------------------------------------
# sfg convert — require the Go binary
# ---------------------------------------------------------------------------


@pytest.fixture(scope="module")
def sfg_binary():
    try:
        find_binary()
    except BinaryNotFoundError:
        pytest.skip("sfg binary not available")


@pytest.fixture(scope="module")
def nov000_tables(sfg_binary):
    from earthscope_sfg_tools.novatel_tools.novatel_level0_operations import (
        nov000_to_level0,
    )

    return nov000_to_level0(NOV000, SCOPE)


def test_nov000_cold_start_logs_are_dropped(nov000_tables):
    # The fixture is a receiver cold start: every log has time status UNKNOWN,
    # so Level 0 ingest keeps none of them and Go writes no table.
    assert nov000_tables == {}


@pytest.fixture(scope="module", params=[0, 1], ids=lambda a: f"ant{a}")
def nov770_table(sfg_binary, request):
    from earthscope_sfg_tools.novatel_tools.novatel_level0_operations import (
        nov770_to_gnss_observations,
    )

    return request.param, nov770_to_gnss_observations(
        NOV770, SCOPE, antenna=request.param
    )


def test_nov770_antennas_kept_separate(nov770_table):
    antenna, table = nov770_table
    assert table.num_rows > 0
    suffix = "_1" if antenna else ""
    streams = set(table["stream_edid"].to_pylist())
    assert streams == {f"NCB.GCC1.2025_A_1126.NOV770.RANGECMP5B{suffix}"}


def test_nov770_observations_follow_go_layout(nov770_table):
    _, obs = nov770_table
    assert {"stream_edid", "time_utc", "time_gpst", "satellite_id"} <= set(
        obs.column_names
    )
    assert set(obs["constellation"].to_pylist()) <= set("GRECJIS")
    assert obs["code"].null_count < obs.num_rows
    assert obs["time_gpst"].type.tz == "UTC"
