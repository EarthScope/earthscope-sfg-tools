"""Tests for SonardyneGNSSData negative-sentinel coercion.

Sonardyne firmware outputs ``-1.0`` for ``sdx``, ``sdy``, and ``sdz`` when
the GNSS position solution is unavailable.  These values must be coerced to
``None`` rather than raising a Pydantic validation error so that QC PIN files
with unavailable GNSS precision still produce valid shotdata.

Regression test for: https://github.com/EarthScope/earthscope-sfg-tools/issues
"""

from decimal import Decimal

import pytest
from pydantic import ValidationError

from earthscope_sfg_tools.datamodels.observationdata.parsing.sv3_models import (
    SV3GPSQuality,
    SonardyneGNSSData,
    TimeData,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_time() -> dict:
    """Minimal TimeData dict accepted by the model."""
    import datetime
    from earthscope_sfg_tools.datamodels.observationdata.constants import GNSS_START_TIME
    ts = GNSS_START_TIME.timestamp()
    return {
        "common": ts,
        "instrument": 0.0,
        "start_count": 0,
        "status": "OK",
    }


def _base_gnss(**overrides) -> dict:
    data = {
        "hae": 0.0,
        "latitude": 40.0,
        "longitude": -125.0,
        "q": 4,  # REAL_TIME_KINEMATIC
        "sdx": 0.05,
        "sdy": 0.05,
        "sdz": 0.10,
        "separation": None,
        "time": _make_time(),
    }
    data.update(overrides)
    return data


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


def test_valid_positive_sds():
    """Positive standard deviations should be preserved."""
    m = SonardyneGNSSData(**_base_gnss())
    assert m.sdx == Decimal("0.05")
    assert m.sdy == Decimal("0.05")
    assert m.sdz == Decimal("0.10")


def test_negative_one_sentinel_coerced_to_none():
    """The canonical Sonardyne -1.0 sentinel must become None, not a ValidationError."""
    m = SonardyneGNSSData(**_base_gnss(sdx=-1.0, sdy=-1.0, sdz=-1.0))
    assert m.sdx is None
    assert m.sdy is None
    assert m.sdz is None


def test_negative_sentinel_on_individual_fields():
    """Sentinel on only some fields, others valid."""
    m = SonardyneGNSSData(**_base_gnss(sdx=-1.0, sdy=0.03, sdz=None))
    assert m.sdx is None
    assert m.sdy == Decimal("0.03")
    assert m.sdz is None


def test_none_sds_accepted():
    """Explicit None is already valid."""
    m = SonardyneGNSSData(**_base_gnss(sdx=None, sdy=None, sdz=None))
    assert m.sdx is None
    assert m.sdy is None
    assert m.sdz is None


def test_zero_sd_accepted():
    """Zero is a valid (non-negative) standard deviation."""
    m = SonardyneGNSSData(**_base_gnss(sdx=0.0, sdy=0.0, sdz=0.0))
    assert m.sdx == Decimal("0")


def test_arbitrary_negative_coerced_to_none():
    """Any negative value (not just -1.0) is treated as a sentinel."""
    m = SonardyneGNSSData(**_base_gnss(sdx=-99.9, sdy=-0.001))
    assert m.sdx is None
    assert m.sdy is None
