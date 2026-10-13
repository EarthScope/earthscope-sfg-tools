"""Tests for the sound speed Level 1 parsers."""

from datetime import UTC, datetime

import pytest

pytest.importorskip("pyiceberg")
pytest.importorskip("pyarrow")

from earthscope_sfg_tools.datamodels.iceberg.schemas import Scope  # noqa: E402
from earthscope_sfg_tools.seafloor_site_tools.soundspeed_level1_operations import (  # noqa: E402
    ctd_to_sound_speed,
)

SCOPE = Scope("cascadia-gorda", "GCC1", "2025_A_1126")


def test_ctd_to_sound_speed(tmp_path):
    path = tmp_path / "ctd.txt"
    path.write_text("-0.5,1500.1\n-10.0 1499.8\n")
    cast_time = datetime(2025, 8, 22, tzinfo=UTC)
    rows = ctd_to_sound_speed(path, SCOPE, cast_time).to_pylist()
    assert [(r["depth"], r["speed"]) for r in rows] == [(0.5, 1500.1), (10.0, 1499.8)]
    assert all(r["time"] == cast_time for r in rows)
    assert all(r["Log_Source"] == "CTD" for r in rows)


def test_sound_speed_rejects_naive_cast_time(tmp_path):
    path = tmp_path / "ctd.txt"
    path.write_text("-0.5,1500.1\n")
    with pytest.raises(ValueError, match="timezone-aware"):
        ctd_to_sound_speed(path, SCOPE, datetime(2025, 8, 22))
