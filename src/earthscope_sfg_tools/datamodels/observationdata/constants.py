"""Shared constants for seafloor geodesy parsing and transformations."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from decimal import getcontext

import julian

getcontext().prec = 10

# GPS epoch — all GPS timestamps are seconds since this date.
GNSS_START_TIME = datetime(1980, 1, 6, tzinfo=UTC)
# Modified Julian Date of the GPS epoch.
GNSS_START_TIME_JULIAN = julian.to_jd(GNSS_START_TIME.replace(tzinfo=None), "mjd")
# Upper MJD bound (~500 years) used for range validation.
GNSS_START_TIME_JULIAN_BOUNDS = julian.to_jd(
    GNSS_START_TIME.replace(tzinfo=None) + timedelta(days=365 * 500), "mjd"
)
# Current GPS–UTC leap-second offset.
LEAP_SECONDS = 18

# Hardware trigger delay (seconds) for SV2 transponders.
TRIGGER_DELAY_SV2 = 0.1
# Hardware trigger delay (seconds) for SV3 transponders.
TRIGGER_DELAY_SV3 = 0.13
# Adjusted leap-second constant used in legacy GARPOS range equations.
ADJ_LEAP = 1.0

# Mapping of transponder address suffix → Sonardyne station offset (ms).
STATION_OFFSETS = {"5209": 200, "5210": 320, "5211": 440, "5212": 560}
# Mapping of sequential index → canonical transponder address.
MASTER_STATION_ID = {"0": "5209", "1": "5210", "2": "5211", "3": "5212"}
