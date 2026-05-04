"""Shared constants for seafloor geodesy parsing and transformations."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from decimal import getcontext

import julian

getcontext().prec = 10

GNSS_START_TIME = datetime(1980, 1, 6, tzinfo=UTC)
GNSS_START_TIME_JULIAN = julian.to_jd(GNSS_START_TIME.replace(tzinfo=None), "mjd")
GNSS_START_TIME_JULIAN_BOUNDS = julian.to_jd(
    GNSS_START_TIME.replace(tzinfo=None) + timedelta(days=365 * 500), "mjd"
)
LEAP_SECONDS = 18

TRIGGER_DELAY_SV2 = 0.1
TRIGGER_DELAY_SV3 = 0.13
ADJ_LEAP = 1.0

STATION_OFFSETS = {"5209": 200, "5210": 320, "5211": 440, "5212": 560}
MASTER_STATION_ID = {"0": "5209", "1": "5210", "2": "5211", "3": "5212"}
