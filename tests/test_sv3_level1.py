"""Tests for the Sonardyne SV3 Level 1 parsers."""

import json
import logging
import math
from pathlib import Path

import pytest

pytest.importorskip("pyiceberg")
pytest.importorskip("pyarrow")

from earthscope_sfg_tools.datamodels.iceberg.schemas import (  # noqa: E402
    ATTITUDE_AHRS,
    GNSS_ANTENNA_POSITIONS,
    TRAVEL_TIMES,
    Scope,
)
from earthscope_sfg_tools.sonardyne_tools.sv3_level1_operations import (  # noqa: E402
    _enu_sigmas_to_ecef,
    dfop00_to_attitude_ahrs,
    dfop00_to_gnss_antenna_positions,
    dfop00_to_travel_times,
    qcpin_to_travel_times,
    unix_to_utc,
)
from earthscope_sfg_tools.sonardyne_tools.sv3_operations import (  # noqa: E402
    dfop00_to_shotdata,
)

DFOP00 = Path(__file__).parent / "data" / "test_gcc1_20250822_DFOP00.raw"
SCOPE = Scope("cascadia-gorda", "GCC1", "2025_A_1126")


@pytest.mark.parametrize(
    "parse, table",
    [
        (dfop00_to_travel_times, TRAVEL_TIMES),
        (dfop00_to_gnss_antenna_positions, GNSS_ANTENNA_POSITIONS),
        (dfop00_to_attitude_ahrs, ATTITUDE_AHRS),
    ],
)
def test_dfop00_parsers_match_iceberg_schema(parse, table):
    result = parse(DFOP00, SCOPE)
    assert result.num_rows > 0
    assert result.schema.equals(table.schema.as_arrow())
    assert set(result.column("Log_Source").to_pylist()) == {"DFOP00"}
    assert set(result.column("Station").to_pylist()) == {"GCC1"}


def test_unix_to_utc_keeps_subsecond_precision():
    # The package sets the global Decimal precision to 10 digits; a UNIX time
    # must not be rounded to whole seconds.
    from decimal import Decimal

    assert unix_to_utc(Decimal("1755820812.6013587")).microsecond == 601358


def test_travel_times_keep_raw_range():
    rows = dfop00_to_travel_times(DFOP00, SCOPE).to_pylist()
    replies = [r for r in rows if r["TravelTime"] > 0]
    assert any(r["T_receive"].microsecond for r in replies)
    # Raw range minus tat and the SV3 trigger delay is the existing corrected
    # tt. Shot data is a subset: its pairing QC rejects some replies.
    shotdata = dfop00_to_shotdata(DFOP00, logging.getLogger(__name__))
    corrected = [r["TravelTime"] - r["tat"] - 0.13 for r in replies]
    for tt in shotdata.tt:
        assert any(math.isclose(tt, c, abs_tol=1e-9) for c in corrected)


def test_travel_times_keep_no_reply_events():
    # Transponders that did not reply log range 0.0, stamped at the ping or at
    # the reply timeout. Level 1 keeps them; Level 2 drops them as the
    # shot-data pipeline does today.
    rows = dfop00_to_travel_times(DFOP00, SCOPE).to_pylist()
    no_reply = [r for r in rows if r["TravelTime"] == 0]
    assert no_reply
    assert all(r["dbV"] == 0 and r["aSNR"] == 0 for r in no_reply)


def test_enu_sigma_rotation_preserves_total_variance():
    sx, sy, sz = _enu_sigmas_to_ecef(41.7, -125.5, 1.0, 2.0, 3.0)
    assert math.isclose(sx**2 + sy**2 + sz**2, 1 + 4 + 9)
    # At lat=0, lon=0: E -> Y, N -> Z, U -> X.
    assert _enu_sigmas_to_ecef(0.0, 0.0, 1.0, 2.0, 3.0) == pytest.approx((3, 1, 2))


def test_qcpin_travel_times(tmp_path):
    events = [json.loads(line) for line in DFOP00.read_text().splitlines()]
    first = next(i for i, e in enumerate(events) if e["event"] == "interrogation")
    qcpin = {"interrogation": events[first]}
    for e in events[first + 1 :]:
        if e["event"] != "range":
            break
        qcpin[e["range"]["cn"]] = e
    path = tmp_path / "test.pin"
    path.write_text(json.dumps(qcpin))

    table = qcpin_to_travel_times(path, SCOPE)
    assert table.num_rows == len(qcpin) - 1
    assert set(table.column("Log_Source").to_pylist()) == {"QCPIN"}
