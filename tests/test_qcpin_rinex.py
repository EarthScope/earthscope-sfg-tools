"""Tests for QCPIN RANGEA -> NovAtel ASCII -> RINEX conversion."""

import json
from pathlib import Path

import pytest

from earthscope_sfg_tools.novatel_tools import qcpin_to_rinex
from earthscope_sfg_tools.novatel_tools.novatel_ascii_operations import (
    qcpin_to_novatelpin,
)
from earthscope_sfg_tools.utils.go_utils import BinaryNotFoundError, find_binary

DFOP00 = Path(__file__).parent / "data" / "test_gcc1_20250822_DFOP00.raw"


@pytest.fixture
def qcpin(tmp_path):
    """A QCPIN-shaped JSON built from the DFOP00 fixture's SV3 events."""
    events = [json.loads(line) for line in DFOP00.read_text().splitlines()]
    path = tmp_path / "test.pin"
    path.write_text(json.dumps({f"event{i}": e for i, e in enumerate(events)}))
    return path


def test_qcpin_to_novatelpin_writes_sorted_rangea(qcpin, tmp_path):
    out = qcpin_to_novatelpin(qcpin, tmp_path)
    lines = out.read_text().splitlines()
    assert out.name == "test_novpin.txt"
    assert lines and all(line.startswith("#RANGEA") for line in lines)
    gps_times = [
        tuple(map(float, line.split(";")[0].split(",")[5:7])) for line in lines
    ]
    assert gps_times == sorted(gps_times)


def test_qcpin_to_rinex(qcpin, tmp_path):
    try:
        find_binary()
    except BinaryNotFoundError:
        pytest.skip("sfg binary not available")
    outdir = tmp_path / "out"
    outdir.mkdir()
    rinex = qcpin_to_rinex(qcpin, outdir, site="GCC1")
    assert [p.suffix for p in rinex] == [".rnx"]
    assert rinex[0].stat().st_size > 0
