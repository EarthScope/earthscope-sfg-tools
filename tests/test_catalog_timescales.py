from datetime import datetime, timezone

import numpy as np
import pandas as pd
import pytest

from earthscope_sfg_tools.catalog.timescales import (
    as_utc,
    gps_minus_utc,
    gps_week_tow,
    utc_to_gps_clock,
    utc_to_j2000,
)


@pytest.mark.parametrize(
    "utc, leap",
    [
        ("1981-06-30T23:59:59", 0),
        ("1981-07-01T00:00:00", 1),
        ("2000-01-01T12:00:00", 13),
        ("2016-12-31T23:59:59", 17),
        ("2017-01-01T00:00:00", 18),
        ("2025-09-15T00:00:00", 18),
    ],
)
def test_gps_minus_utc(utc, leap):
    assert gps_minus_utc(pd.Timestamp(utc, tz="UTC"))[0] == leap


def test_week_tow():
    # GPS week 2384 began Sunday 2025-09-14 00:00 GPS; a day later in UTC is
    # 86400 s + 18 leap seconds into the week.
    week, tow = gps_week_tow(datetime(2025, 9, 15, tzinfo=timezone.utc))
    assert week[0] == 2384 and tow[0] == 86418.0
    # Sub-second precision survives.
    week, tow = gps_week_tow(pd.Timestamp("2025-09-15T00:00:00.123456", tz="UTC"))
    assert tow[0] == pytest.approx(86418.123456, abs=1e-9)


def test_j2000_is_gps_calendar():
    # The J2000 epoch (12:00 on the GPS calendar) is UTC 11:59:47 in 2000.
    assert utc_to_j2000(pd.Timestamp("2000-01-01T11:59:47", tz="UTC"))[0] == 0.0
    t = pd.Series(pd.to_datetime(["2025-09-15T00:00:00Z", "2025-09-15T00:00:01.5Z"], format="ISO8601"))
    j = utc_to_j2000(t)
    assert j[1] - j[0] == pytest.approx(1.5)
    expected = (pd.Timestamp("2025-09-15T00:00:18") - pd.Timestamp("2000-01-01T12:00:00")).total_seconds()
    assert j[0] == expected


def test_vector_and_naive_inputs():
    clock = utc_to_gps_clock(np.array(["2016-12-31T23:59:59", "2017-01-01T00:00:00"], dtype="datetime64[s]"))
    assert list(clock) == [pd.Timestamp("2017-01-01T00:00:16"), pd.Timestamp("2017-01-01T00:00:18")]
    assert as_utc("2025-09-15T00:00:00") == datetime(2025, 9, 15, tzinfo=timezone.utc)
