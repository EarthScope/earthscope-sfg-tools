"""
Time-scale conversions between the catalog (UTC) and the processing codes.

Level 1 stores every time as UTC (``timestamptz``). GNATSS works on the GPS
clock: NovAtel logs carry GPS week + time of week, and GNATSS's single
"J2000" scale is seconds of the GPS calendar since 2000-01-01T12:00:00
(``gps_ws_time_to_astrotime`` labels the GPS calendar as TT and subtracts
the J2000 epoch). Both are derived here from UTC with the leap-second table,
so epochs before 2017 get their own GPS-UTC offset rather than a fixed 18 s.
"""

from datetime import datetime, timezone

import numpy as np
import pandas as pd

GPS_EPOCH = pd.Timestamp("1980-01-06T00:00:00")  # GPS clock, naive
J2000_EPOCH = pd.Timestamp("2000-01-01T12:00:00")  # on the GPS calendar, naive
SECONDS_PER_WEEK = 7 * 86400

# UTC instants from which GPS-UTC took each value (TAI-UTC minus 19 s).
_LEAPS_FROM = pd.DatetimeIndex(
    [
        "1981-07-01", "1982-07-01", "1983-07-01", "1985-07-01", "1988-01-01",
        "1990-01-01", "1991-01-01", "1992-07-01", "1993-07-01", "1994-07-01",
        "1996-01-01", "1997-07-01", "1999-01-01", "2006-01-01", "2009-01-01",
        "2012-07-01", "2015-07-01", "2017-01-01",
    ],
    tz="UTC",
)  # fmt: skip


def _utc_index(t) -> pd.DatetimeIndex:
    """t (datetime, Timestamp, Series or array of them) as a UTC DatetimeIndex
    in nanoseconds, the unit the leap table and the arithmetic below assume."""
    idx = pd.DatetimeIndex(pd.Series(t) if np.ndim(t) else [t]).as_unit("ns")
    return idx.tz_localize("UTC") if idx.tz is None else idx.tz_convert("UTC")


def gps_minus_utc(t) -> np.ndarray:
    """GPS-UTC in whole seconds at UTC instant(s) t (0 before 1981-07-01)."""
    return np.searchsorted(_LEAPS_FROM.asi8, _utc_index(t).asi8, side="right")


def utc_to_gps_clock(t) -> pd.DatetimeIndex:
    """The GPS clock reading (naive) at UTC instant(s) t."""
    idx = _utc_index(t)
    return idx.tz_localize(None) + pd.to_timedelta(gps_minus_utc(idx), unit="s")


def gps_week_tow(t) -> tuple[np.ndarray, np.ndarray]:
    """GPS week and time of week [s] at UTC instant(s) t."""
    since = (utc_to_gps_clock(t) - GPS_EPOCH).asi8  # ns
    week, rem = np.divmod(since, SECONDS_PER_WEEK * 1_000_000_000)
    return week, rem / 1e9


def utc_to_j2000(t) -> np.ndarray:
    """GNATSS J2000 seconds (GPS calendar since 2000-01-01T12:00) at UTC t."""
    return (utc_to_gps_clock(t) - J2000_EPOCH).asi8 / 1e9


def as_utc(t: datetime | str) -> datetime:
    """t as an aware UTC datetime; naive values and ISO strings are taken as UTC."""
    if isinstance(t, str):
        t = datetime.fromisoformat(t)
    if t.tzinfo is None:
        return t.replace(tzinfo=timezone.utc)
    return t.astimezone(timezone.utc)


__all__ = [
    "GPS_EPOCH",
    "J2000_EPOCH",
    "as_utc",
    "gps_minus_utc",
    "gps_week_tow",
    "utc_to_gps_clock",
    "utc_to_j2000",
]
