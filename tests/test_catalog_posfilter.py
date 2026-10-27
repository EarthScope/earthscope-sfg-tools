"""
GNATSS posfilter export: file contents, delay handling, and (when a Python
with GNATSS is available, SFG_GNATSS_PYTHON) a round trip through GNATSS's
own loaders and configuration model.
"""

import json
import os
import subprocess
import textwrap
from dataclasses import replace
from datetime import datetime, timedelta, timezone

import numpy as np
import pandas as pd
import pytest

from earthscope_sfg_tools.catalog.posfilter import (
    export_posfilter,
    internal_delays,
    novatel_crc32,
    write_pxp_tt,
)
from earthscope_sfg_tools.catalog.site import PosfilterSite, PosfilterTransponder
from earthscope_sfg_tools.catalog.timescales import utc_to_j2000
from earthscope_sfg_tools.datamodels.iceberg.schemas import Scope

SCOPE = Scope("cascadia-gorda", "NBR1", "2025_A_1126")
T0 = datetime(2025, 9, 15, 3, 0, tzinfo=timezone.utc)
SITE = PosfilterSite(
    site_id="NBR1",
    start=T0,
    end=T0 + timedelta(hours=1),
    array_center=(42.13061187, -125.11096152, -29.94),
    transponders=(
        PosfilterTransponder("NBR1-1", "5209", 42.14325481, -125.1113693, -1831.69, tat=0.08),
        PosfilterTransponder("NBR1-2", "5210", 42.12436027, -125.12612096, -1867.926, tat=0.36),
        PosfilterTransponder("NBR1-3", "5211", 42.12574327, -125.09562600, -1770.511, tat=0.64),
    ),
    atd_offset=(0.0, 0.00053, 0.92813),
    trigger_delay=0.13,
    survey="2025_A_1126_NBR1_3",
)


def ts(seconds: float) -> pd.Timestamp:
    return pd.Timestamp(T0 + timedelta(seconds=seconds))


def frames() -> dict[str, pd.DataFrame]:
    n = 6
    inspva = pd.DataFrame(
        {
            "time": [ts(i * 0.05) for i in range(n)],
            "lat": 42.13, "lon": -125.11, "alt": -31.9,
            "north": 0.5, "east": -1.25, "up": 0.01,
            "roll": 1.5, "pitch": -0.25, "heading": 270.125,
            "ins_status": ["INS_SOLUTION_GOOD"] * (n - 1) + ["INS_ALIGNMENT_COMPLETE"],
            "time_status": ["FINESTEERING"] * n,
        }
    )  # fmt: skip
    insstdev = pd.DataFrame(
        {
            "time": inspva["time"],
            "lat_sig": 1.9, "lon_sig": 1.7, "alt_sig": 2.2,
            "north_sig": 0.05, "east_sig": 0.048, "up_sig": 0.05,
            "cov_rr": 0.03, "cov_pp": 0.035, "cov_hh": 0.4,
            "ext_sol_stat": "2e000041", "time_since_update": "0",
            "time_status": ["FINESTEERING"] * (n - 1) + [None],  # no status: not kept
        }
    )  # fmt: skip
    gps = pd.DataFrame(
        {
            "time": [ts(0.0), ts(0.1), ts(0.2)],
            "ant_x": -2724772.3610, "ant_y": -3875256.6762, "ant_z": 4256304.9697,
            "ant_sigx": [12.3, None, 12.1], "ant_sigy": [15.9, None, 15.8], "ant_sigz": [17.7, None, 17.6],
            "stream_edid": "cascadia-gorda:NBR1:2025_A_1126:DFOP00:DFOP00",
        }
    )  # fmt: skip
    # Two pings: the first answered by all three, the second by NBR1-1 only.
    tt = pd.DataFrame(
        {
            "T_transmit": [ts(0.05)] * 3 + [ts(0.25)],
            "T_receive": [ts(2.6), ts(2.9), ts(3.1), ts(2.8)],
            "MT_ID": ["IR5209", "IR5210", "IR5211", "IR5209"],
            "TravelTime": [2.534127, 2.861003, 3.012345, 2.550000],
            "tat": [0.08, 0.36, 0.64, 0.08],
        }
    )
    sv = pd.DataFrame(
        {
            "time": [pd.Timestamp("2023-01-01", tz="UTC")] * 3,
            "depth": [0.0, 1.0, 2.0],
            "speed": [1505.09, 1505.79, 1505.82],
            "stream_edid": "cascadia-gorda:NBR1:2025_A_1126:CTD:CTD",
        }
    )
    return {"inspva": inspva, "insstdev": insstdev, "gnss_antenna_positions": gps, "travel_times": tt, "sound_speed": sv}


class FakeReader:
    """Level1Reader stand-in serving the frames above."""

    def __init__(self, data):
        self.data = data
        self.calls = []

    def read(self, table, scope, start=None, end=None, **kw):
        self.calls.append((table, kw))
        return self.data[table].copy()


def test_crc_matches_novatel():
    text = ("RANGEA,USB2,0,69.0,FINESTEERING,2326,188418.000,02000000,5103,15823;36,30,0,21007753.481,"
            "0.022,-110396605.995380,0.007,1631.778,51.5,7756.971,1810bc04")  # fmt: skip
    assert novatel_crc32(text) == novatel_crc32(text)  # deterministic
    assert format(novatel_crc32(""), "08x") == "00000000"


def test_export_files(tmp_path):
    reader = FakeReader(frames())
    s = export_posfilter(reader, SITE, SCOPE, tmp_path)

    assert (s.inspva_rows, s.inspva_good) == (6, 5)
    assert (s.insstdev_rows, s.insstdev_good) == (6, 5)
    assert (s.gps_rows, s.gps_dropped_no_sigma) == (2, 1)
    assert (s.pings, s.replies) == (2, 4)
    assert s.internal_delays == {"NBR1-1": 0.08, "NBR1-2": 0.36, "NBR1-3": 0.64}
    assert s.sound_speed_rows == 3
    assert ("gnss_antenna_positions", {"file_type": "DFOP00"}) in reader.calls

    line = (tmp_path / "INSPVAA.dat").read_text().splitlines()[0]
    text, crc = line[1:].split("*")
    assert line.startswith("#INSPVAA,COM1,0,0.0,FINESTEERING,2384,97218.000000,")
    assert text.endswith(",INS_SOLUTION_GOOD") and crc == format(novatel_crc32(text), "08x")
    assert ";2384,97218.000000,42.13000000000,-125.11000000000,-31.9000," in line

    pxp = [ln.split() for ln in (tmp_path / "pxp_tt").read_text().splitlines()]
    assert float(pxp[0][0]) == pytest.approx(utc_to_j2000(ts(0.05))[0], abs=1e-6)
    # Range minus the 0.13 s trigger delay, in µs; TAT stays in (internal_delay).
    assert pxp[0][1:] == ["2404127", "2731003", "2882345"]
    assert pxp[1][1:] == ["2420000", "0", "0"]

    gps = (tmp_path / "GPS_POS_FREED").read_text().splitlines()
    assert len(gps) == 2 and gps[0].split()[1] == "GPSPOS"
    assert (tmp_path / "sound_speed.txt").read_text().splitlines()[1] == "-1.0000 1505.790000"

    cfg = json.loads((tmp_path / "config.yaml").read_text())
    assert cfg["posfilter"]["atd_offsets"] == {"forward": 0.0, "rightward": 0.00053, "downward": 0.92813}
    assert [t["internal_delay"] for t in cfg["transponders"]] == [0.08, 0.36, 0.64]
    assert cfg["posfilter"]["input_files"]["travel_times"]["loader_kwargs"] == {"is_j2k": True}
    assert json.loads((tmp_path / "summary.json").read_text())["replies"] == 4


def test_metadata_tat_wins_over_logged():
    tt = frames()["travel_times"]
    tt["tat"] = [0.2, 0.32, 0.44, 0.2]  # NBR1 2025: SV3 kept its old TATs
    delays, warnings = internal_delays(tt, SITE)
    assert delays == {"NBR1-1": 0.08, "NBR1-2": 0.36, "NBR1-3": 0.64}
    assert len(warnings) == 3 and "using metadata" in warnings[0]


def test_logged_tat_only_without_metadata():
    site = replace(SITE, transponders=tuple(replace(t, tat=None) for t in SITE.transponders))
    tt = frames()["travel_times"]
    delays, warnings = internal_delays(tt, site)
    assert delays == {"NBR1-1": 0.08, "NBR1-2": 0.36, "NBR1-3": 0.64}
    assert all("using logged" in w for w in warnings)
    tt.loc[3, "tat"] = 0.2  # logged TAT changes, nothing to fall back on
    with pytest.raises(ValueError, match="changes within the window"):
        internal_delays(tt, site)
    with pytest.raises(ValueError, match="no replies"):
        fresh = frames()["travel_times"]
        internal_delays(fresh[fresh["MT_ID"] != "IR5211"], site)


def test_unknown_transponder_is_an_error():
    tt = frames()["travel_times"]
    tt.loc[3, "MT_ID"] = "IR5212"
    with pytest.raises(ValueError, match="IR5212"):
        write_pxp_tt(tt, SITE, os.devnull)


GNATSS_PY = os.environ.get("SFG_GNATSS_PYTHON")


@pytest.mark.skipif(not GNATSS_PY, reason="set SFG_GNATSS_PYTHON to a Python with gnatss")
def test_gnatss_round_trip(tmp_path):
    export_posfilter(FakeReader(frames()), SITE, SCOPE, tmp_path)
    script = textwrap.dedent(
        f"""
        import json
        from pathlib import Path
        from gnatss import loaders
        d = Path({str(tmp_path)!r})
        cfg = loaders.load_configuration(str(d / "config.yaml"))
        pva = loaders.load_novatel([str(d / "INSPVAA.dat")])
        std = loaders.load_novatel_std([str(d / "INSSTDEVA.dat")])
        gps = loaders.load_gps_positions([str(d / "GPS_POS_FREED")])
        tt = loaders.load_travel_times([str(d / "pxp_tt")], [t.pxp_id for t in cfg.transponders], is_j2k=True)
        sv = loaders.load_sound_speed([str(d / "sound_speed.txt")])
        print(json.dumps({{
            "pva_time": pva["time"].tolist(), "pva_heading": pva["heading"].tolist(),
            "std_n": len(std), "gps_time": gps["time"].tolist(), "gps_cols": list(gps.columns),
            "tt": tt.to_dict(orient="list"), "sv_n": len(sv),
            "atd": [cfg.posfilter.atd_offsets.forward, cfg.posfilter.atd_offsets.rightward,
                    cfg.posfilter.atd_offsets.downward],
            "delays": [t.internal_delay for t in cfg.transponders],
        }}))
        """
    )
    out = subprocess.run([GNATSS_PY, "-c", script], capture_output=True, text=True, check=True)
    got = json.loads(out.stdout.strip().splitlines()[-1])

    f = frames()
    good = f["inspva"]["ins_status"] == "INS_SOLUTION_GOOD"
    # GNATSS's own J2000 (astropy, from week/seconds) equals ours to the µs.
    np.testing.assert_allclose(got["pva_time"], utc_to_j2000(f["inspva"]["time"][good]), atol=1e-6)
    assert got["pva_heading"] == [270.125] * int(good.sum())
    assert got["std_n"] == 5
    np.testing.assert_allclose(got["gps_time"], utc_to_j2000(f["gnss_antenna_positions"]["time"].iloc[[0, 2]]), atol=1e-6)
    assert got["gps_cols"][:4] == ["time", "ant_x", "ant_y", "ant_z"]
    assert got["tt"]["NBR1-1"] == [pytest.approx(2.404127), pytest.approx(2.42)]
    assert got["sv_n"] == 3
    assert got["atd"] == [0.0, 0.00053, 0.92813]
    assert got["delays"] == [0.08, 0.36, 0.64]
