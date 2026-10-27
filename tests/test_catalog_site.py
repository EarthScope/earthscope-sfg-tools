import json
from datetime import datetime, timezone
from pathlib import Path

import pytest

from earthscope_sfg_tools.catalog.site import PosfilterSite
from earthscope_sfg_tools.datamodels.metadata.earthscope.site import Site


def _site(**overrides) -> Site:
    """A trimmed copy of NBR1's site_metadata.json."""
    tp = lambda addr, start: {  # noqa: E731
        "address": addr,
        "start": start,
        "tat": [{"value": 200.0, "start": start, "end": "2025-09-15T02:26:00"}, {"value": 80.0, "start": "2025-09-15T02:26:00"}],
    }
    data = {
        "names": ["NBR1"],
        "networks": ["cascadia-gorda"],
        "timeOrigin": "2021-09-08T00:00:00",
        "arrayCenter": {"latitude": 42.13061187, "longitude": -125.11096152, "elevation": -29.94},
        "benchmarks": [
            {"name": "NBR1-1", "benchmarkID": "USGS001", "start": "2023-09-08T22:23:47",
             "aPrioriLocation": {"latitude": 42.14325481, "longitude": -125.1113693, "elevation": -1831.69},
             "transponders": [tp("5209", "2023-09-08T22:23:47")]},
            {"name": "NBR1-2", "benchmarkID": "USGS002", "start": "2021-09-08T20:19:26",
             "aPrioriLocation": {"latitude": 42.12436027, "longitude": -125.12612096, "elevation": -1867.926},
             "transponders": [tp("5210", "2021-09-08T20:19:26")]},
            # Retired before the campaign: never a transponder of the run.
            {"name": "NBR1-OLD", "benchmarkID": "USGS000", "start": "2019-01-01T00:00:00", "end": "2021-01-01T00:00:00",
             "aPrioriLocation": {"latitude": 42.1, "longitude": -125.1, "elevation": -1800.0},
             "transponders": [tp("5100", "2019-01-01T00:00:00")]},
        ],
        "campaigns": [
            {"name": "2025_A_1126", "type": "SV3", "vesselCode": "1126",
             "start": "2025-09-14T07:32:00", "end": "2025-09-21T13:08:00",
             "vessel": {"name": "1126", "type": "waveglider", "model": "SV3",
                        "atdOffsets": [{"serialNumber": "641-0654", "x": 0.0, "y": 0.00053, "z": 0.92813}],
                        "acousticTransceivers": [{"type": "x", "serialNumber": "1", "frequency": "MF",
                                                  "start": "2023-02-28T00:00:00", "triggerDelay": 0.13,
                                                  "delayIncludedInTWTT": True}]},
             "surveys": [
                 {"id": "S1", "type": "circle drive", "benchmarkIDs": ["NBR1-1", "NBR1-2"],
                  "start": "2025-09-14T07:32:00", "end": "2025-09-14T16:27:00"},
                 {"id": "S2", "type": "center", "benchmarkIDs": ["NBR1-1", "NBR1-9"],
                  "start": "2025-09-15T02:45:00", "end": "2025-09-21T13:08:00"},
             ]},
        ],
    }  # fmt: skip
    data.update(overrides)
    return Site(**data)


def test_from_survey():
    s = PosfilterSite.from_metadata(_site(), "2025_A_1126", survey="S1")
    assert s.site_id == "NBR1" and s.survey == "S1"
    assert s.start == datetime(2025, 9, 14, 7, 32, tzinfo=timezone.utc)
    assert s.array_center == (42.13061187, -125.11096152, -29.94)
    assert [t.pxp_id for t in s.transponders] == ["NBR1-1", "NBR1-2"]
    assert s.by_mt_id()["IR5209"].height == -1831.69
    assert s.atd_offset == (0.0, 0.00053, 0.92813)
    assert s.trigger_delay == 0.13


def test_window_excludes_retired_benchmarks():
    s = PosfilterSite.from_metadata(_site(), "2025_A_1126", start="2025-09-16", end="2025-09-17")
    assert [t.address for t in s.transponders] == ["5209", "5210"]


def test_errors():
    with pytest.raises(ValueError, match="NBR1-9"):
        PosfilterSite.from_metadata(_site(), "2025_A_1126", survey="S2")  # unknown benchmark
    with pytest.raises(ValueError, match="campaign"):
        PosfilterSite.from_metadata(_site(), "2099_A_0000")
    with pytest.raises(ValueError, match="arrayCenter"):
        PosfilterSite.from_metadata(_site(arrayCenter=None), "2025_A_1126", survey="S1")
    with pytest.raises(ValueError, match="not both"):
        PosfilterSite.from_metadata(_site(), "2025_A_1126", survey="S1", start="2025-09-14")


def test_tat_change_within_window_is_an_error():
    # The fixture's TATs change at 2025-09-15T02:26.
    with pytest.raises(ValueError, match="TAT changes"):
        PosfilterSite.from_metadata(_site(), "2025_A_1126", start="2025-09-15T02:00", end="2025-09-15T03:00")
    s = PosfilterSite.from_metadata(_site(), "2025_A_1126", start="2025-09-15T03:00", end="2025-09-15T04:00")
    assert [t.tat for t in s.transponders] == [0.08, 0.08]


def test_trigger_delay_not_in_twtt():
    site = _site()
    site.campaigns[0].vessel.acousticTransceivers[0].delayIncludedInTWTT = False
    assert PosfilterSite.from_metadata(site, "2025_A_1126", survey="S1").trigger_delay == 0.0


NBR1 = Path("/Volumes/DunbarSSD/Project/SeafloorGeodesy/SFGMain/cascadia-gorda/NBR1/metadata/site_metadata.json")


@pytest.mark.skipif(not NBR1.exists(), reason="NBR1 site metadata not mounted")
def test_real_nbr1():
    s = PosfilterSite.from_metadata(Site.from_json(str(NBR1)), "2025_A_1126", survey="2025_A_1126_NBR1_3")
    assert [t.pxp_id for t in s.transponders] == ["NBR1-1", "NBR1-2", "NBR1-3"]
    assert set(s.by_mt_id()) == {"IR5209", "IR5210", "IR5211"}
    assert json.dumps(s.atd_offset) == "[0.0, 0.00053, 0.92813]"
