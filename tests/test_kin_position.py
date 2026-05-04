"""Tests for the PPP kinematic (.kin) position file fixture.

The .kin format is the WaSPP/GIPSY-style output:
  - Fixed-width header terminated by "END OF HEADER"
  - Column header line starting with "* Mjd"
  - Data lines: MJD  Sod  [* flag]  X  Y  Z  Lat  Lon  Height  Nsat/...  PDOP

There is no Python parser yet; these tests document the file structure and
expected values so a future parser can be validated against them.
"""

from pathlib import Path

import pytest

FIXTURE = Path(__file__).parent / "data" / "test_kin_2025233_gcc1.kin"

# ---------------------------------------------------------------------------
# Helpers (intentionally minimal — not a full parser)
# ---------------------------------------------------------------------------

END_OF_HEADER = "END OF HEADER"


def _read_kin(path: Path) -> tuple[list[str], list[str]]:
    """Return (header_lines, data_lines) split at END OF HEADER."""
    lines = path.read_text().splitlines()
    split = next(i for i, l in enumerate(lines) if END_OF_HEADER in l)
    # line after END OF HEADER is the column label row; data starts one further
    return lines[: split + 1], lines[split + 2 :]


def _parse_data_line(line: str) -> dict:
    """Parse one data line into a dict of floats/ints."""
    tokens = line.split()
    flagged = tokens[2] == "*"
    offset = 1 if flagged else 0
    return {
        "mjd": int(tokens[0]),
        "sod": float(tokens[1]),
        "flagged": flagged,
        "x": float(tokens[2 + offset]),
        "y": float(tokens[3 + offset]),
        "z": float(tokens[4 + offset]),
        "latitude": float(tokens[5 + offset]),
        "longitude": float(tokens[6 + offset]),
        "height": float(tokens[7 + offset]),
        "nsat": int(tokens[8 + offset]),
        "pdop": float(tokens[-1]),
    }


# ---------------------------------------------------------------------------
# File-structure tests
# ---------------------------------------------------------------------------


class TestKinFileStructure:
    def test_fixture_exists(self):
        assert FIXTURE.exists()

    def test_has_end_of_header(self):
        text = FIXTURE.read_text()
        assert END_OF_HEADER in text

    def test_header_contains_station(self):
        header, _ = _read_kin(FIXTURE)
        station_lines = [l for l in header if "STATION" in l]
        assert len(station_lines) == 1
        assert "gcc1" in station_lines[0].lower()

    def test_header_contains_obs_epochs(self):
        header, _ = _read_kin(FIXTURE)
        assert any("OBS FIRST EPOCH" in l for l in header)
        assert any("OBS LAST EPOCH" in l for l in header)

    def test_header_lists_fields(self):
        """Header must describe Mjd, Sod, Latitude, Longitude, Height."""
        header, _ = _read_kin(FIXTURE)
        header_text = "\n".join(header)
        for field in ("Mjd", "Sod", "Latitude", "Longitude", "Height"):
            assert field in header_text


# ---------------------------------------------------------------------------
# Data-content tests
# ---------------------------------------------------------------------------


class TestKinDataContent:
    @pytest.fixture(scope="class")
    def records(self):
        _, data_lines = _read_kin(FIXTURE)
        return [_parse_data_line(l) for l in data_lines if l.strip()]

    def test_epoch_count(self, records):
        """Fixture should contain 451 epochs."""
        assert len(records) == 451

    def test_flagged_epoch_count(self, records):
        """Exactly 2 epochs are flagged in this file."""
        flagged = [r for r in records if r["flagged"]]
        assert len(flagged) == 2

    def test_all_same_mjd(self, records):
        """All epochs share Modified Julian Day 60908."""
        mjds = {r["mjd"] for r in records}
        assert mjds == {60908}

    def test_sod_range(self, records):
        """Second-of-day spans 75 522.6 s → 75 972.6 s (~7.5 min of data)."""
        sods = [r["sod"] for r in records]
        assert min(sods) == pytest.approx(75522.6, abs=0.1)
        assert max(sods) == pytest.approx(75972.6, abs=0.1)

    def test_latitude_range(self, records):
        """Site is at ~41.6° N latitude."""
        lats = [r["latitude"] for r in records]
        assert all(41.0 < lat < 42.5 for lat in lats)

    def test_longitude_range(self, records):
        """Site longitude is ~234.5° (east of Greenwich, Pacific coast)."""
        lons = [r["longitude"] for r in records]
        assert all(233.0 < lon < 236.0 for lon in lons)

    def test_height_range(self, records):
        """Ellipsoidal height for this nearshore site is roughly -40 to 0 m."""
        heights = [r["height"] for r in records]
        assert all(-50.0 < h < 10.0 for h in heights)

    def test_nsat_positive(self, records):
        """Every epoch should report at least one tracked satellite."""
        assert all(r["nsat"] > 0 for r in records)

    def test_pdop_nonnegative(self, records):
        """PDOP is either 0 (unfixed) or a positive value."""
        assert all(r["pdop"] >= 0.0 for r in records)

    def test_nonzero_pdop_range(self, records):
        """Fixed epochs should have PDOP between 1.5 and 5."""
        fixed = [r["pdop"] for r in records if r["pdop"] > 0]
        assert len(fixed) > 0
        assert all(1.0 <= p <= 6.0 for p in fixed)

    def test_interval_is_one_second(self, records):
        """Epochs are spaced 1 second apart."""
        sods = sorted(r["sod"] for r in records)
        gaps = [round(sods[i + 1] - sods[i], 2) for i in range(len(sods) - 1)]
        assert all(g == 1.0 for g in gaps)
