"""
Export a GNATSS posfilter run from the Level 1 catalog.

:func:`export_posfilter` writes, for one site window, the files GNATSS's own
loaders read (so its time joins and column handling stay GNATSS's) and a
``config.yaml`` pointing at them:

- ``INSPVAA.dat`` / ``INSSTDEVA.dat``: NovAtel ASCII logs rebuilt from
  ``inspva`` / ``insstdev``, with the logged time and INS statuses so
  GNATSS's FINESTEERING / INS_SOLUTION_GOOD filter applies as on raw logs.
- ``GPS_POS_FREED``: antenna positions (``gnss_antenna_positions``), GNATSS
  "legacy" format; sigmas as logged, or from ``insstdev`` (see
  :func:`insstdev_sigmas`).
- ``pxp_tt``: one line per ping, J2000 transmit time and the two-way travel
  time [µs] to each transponder (GNATSS "legacy" format, ``is_j2k``).
- ``sound_speed.txt`` (optional): the sound speed profile, depth negative
  down, for the GNATSS solver.

With ``solver=SolverOptions(...)`` the config also has a ``solver`` section,
so ``gnatss run config.yaml`` goes on from the posfilter's GNSS-A Level 2
frame (``output/gps_solution.csv``) to the array position.

Travel times follow GNATSS's own DFOP00 loader: the exported time is the
logged range minus the transceiver trigger delay, and each transponder's
turn-around time (TAT) is its ``internal_delay``, which the solver removes.
GNATSS places a reply at ping time + travel time, so keeping the TAT in puts
it at the time the reply arrived. The TAT comes from the site metadata (see
:func:`internal_delays`) and must be constant over the window.

``config.yaml`` is written as JSON, which YAML loaders read unchanged.
"""

import json
import logging
import zlib
from dataclasses import asdict, dataclass, field
from pathlib import Path

import numpy as np
import pandas as pd
from pymap3d import ecef2geodetic

from ..datamodels.iceberg.schemas import Scope
from .reader import Level1Reader
from .site import PosfilterSite
from .timescales import gps_week_tow, utc_to_j2000

logger = logging.getLogger(__name__)

GOOD_TIME = "FINESTEERING"
GOOD_INS = "INS_SOLUTION_GOOD"

FILES = {
    "novatel": "INSPVAA.dat",
    "novatel_std": "INSSTDEVA.dat",
    "gps_positions": "GPS_POS_FREED",
    "travel_times": "pxp_tt",
    "sound_speed": "sound_speed.txt",
}


# Reference ellipsoid of the metadata positions and the antenna ECEF.
WGS84 = {"semi_major_axis": 6378137.0, "reverse_flattening": 298.257223563}


@dataclass(frozen=True)
class SolverOptions:
    """GNATSS solver settings an export writes to ``config.yaml``.

    ``gps_sigma_limit`` drops shots whose antenna 3D sigma [m] exceeds it
    (GNATSS takes the square root of the summed squared covariance
    diagonals). GNATSS's default, 0.05, suits PPP; the posfilter's Kalman
    covariances from the SV3 GNSS stream are ~0.2 m^2 per axis, which that
    default rejects entirely.
    """

    gps_sigma_limit: float = 1.0
    distance_limit: float = 150.0  # [m] horizontal distance from the array center
    harmonic_mean_start_depth: float = 0.0  # [m] negative down


@dataclass
class ExportSummary:
    """What an export wrote, kept and dropped (also written as summary.json)."""

    out_dir: str
    site_id: str
    survey: str | None
    start: str
    end: str
    inspva_rows: int = 0
    inspva_good: int = 0  # rows GNATSS keeps (FINESTEERING, INS_SOLUTION_GOOD)
    insstdev_rows: int = 0
    insstdev_good: int = 0
    gps_rows: int = 0
    gps_dropped_no_sigma: int = 0
    gps_sigmas: str = "logged"  # where the antenna sigmas came from
    pings: int = 0
    replies: int = 0
    internal_delays: dict[str, float] = field(default_factory=dict)
    sound_speed_rows: int = 0
    warnings: list[str] = field(default_factory=list)


def novatel_crc32(text: str) -> int:
    """NovAtel's CRC-32 of a log's text between '#' and '*' (zlib's CRC-32
    without the initial and final inversion)."""
    return zlib.crc32(text.encode("ascii"), 0xFFFFFFFF) ^ 0xFFFFFFFF


def _novatel_line(log: str, time_status: str, week: int, tow: float, body: str) -> str:
    text = f"{log},COM1,0,0.0,{time_status},{week},{tow:.6f},00000000,0,0;{body}"
    return f"#{text}*{novatel_crc32(text):08x}\n"


def _status(s, default: str) -> str:
    return s if isinstance(s, str) and s else default


def write_inspvaa(df: pd.DataFrame, path: Path) -> int:
    """Write inspva rows as INSPVAA logs; returns how many GNATSS keeps.
    Rows without a logged status are written as UNKNOWN and so dropped."""
    week, tow = gps_week_tow(df["time"])
    good = 0
    with open(path, "w") as f:
        for i, r in enumerate(df.itertuples(index=False)):
            ts = _status(getattr(r, "time_status", None), "UNKNOWN")
            ins = _status(getattr(r, "ins_status", None), "INS_SOLUTION_UNKNOWN")
            good += ts == GOOD_TIME and ins == GOOD_INS
            body = (
                f"{week[i]},{tow[i]:.6f},{r.lat:.11f},{r.lon:.11f},{r.alt:.4f},"
                f"{r.north:.4f},{r.east:.4f},{r.up:.4f},"
                f"{r.roll:.9f},{r.pitch:.9f},{r.heading:.9f},{ins}"
            )
            f.write(_novatel_line("INSPVAA", ts, int(week[i]), tow[i], body))
    return good


def write_insstdeva(df: pd.DataFrame, path: Path) -> int:
    """Write insstdev rows as INSSTDEVA logs; returns how many GNATSS keeps."""
    week, tow = gps_week_tow(df["time"])
    good = 0
    with open(path, "w") as f:
        for i, r in enumerate(df.itertuples(index=False)):
            ts = _status(getattr(r, "time_status", None), "UNKNOWN")
            good += ts == GOOD_TIME
            sigs = ",".join(
                f"{v:.4f}"
                for v in (r.lat_sig, r.lon_sig, r.alt_sig, r.north_sig, r.east_sig, r.up_sig,
                          r.cov_rr, r.cov_pp, r.cov_hh)
            )  # fmt: skip
            body = f"{sigs},{_status(r.ext_sol_stat, '0')},{_status(r.time_since_update, '0')},0,0,0"
            f.write(_novatel_line("INSSTDEVA", ts, int(week[i]), tow[i], body))
    return good


def write_gps_pos_freed(df: pd.DataFrame, path: Path) -> tuple[int, int]:
    """Write antenna positions as GPS_POS_FREED; rows without sigmas are
    dropped (GNATSS needs them). Returns (written, dropped)."""
    sig = ["ant_sigx", "ant_sigy", "ant_sigz"]
    keep = df[sig].notna().all(axis=1)
    rows = df[keep]
    j2000 = utc_to_j2000(rows["time"])
    with open(path, "w") as f:
        for j, r in zip(j2000, rows.itertuples(index=False)):
            f.write(
                f"{j:.6f} GPSPOS {r.ant_x:.4f} {r.ant_y:.4f} {r.ant_z:.4f} "
                f"{r.ant_sigx:.4f} {r.ant_sigy:.4f} {r.ant_sigz:.4f}\n"
            )
    return int(keep.sum()), int((~keep).sum())


def insstdev_sigmas(gps: pd.DataFrame, insstdev: pd.DataFrame, *, tolerance: float = 1.0) -> pd.DataFrame:
    """``gps`` with its ECEF sigmas replaced by the INS position sigmas.

    For the DFOP00 stream: the SV3 GNSS block logs the INS position but the
    receiver's single-point sigmas (13-17 m at NBR1, against 1-2 m from
    INSSTDEV), and with those GNATSS's Kalman filter follows the integrated
    INS velocity instead, drifting metres from the track. Each row takes the
    nearest ``insstdev`` row within ``tolerance`` seconds (lat/lon/alt sigmas as
    north/east/up), rotated to ECEF at the row's position (the diagonal of
    R diag(sig^2) R^T). Rows without one get null sigmas, which
    :func:`write_gps_pos_freed` drops.
    """
    std = insstdev[["time", "lat_sig", "lon_sig", "alt_sig"]].dropna().sort_values("time")
    gps = gps.sort_values("time")
    m = _nearest(gps["time"], std, tolerance)
    lat, lon, _ = ecef2geodetic(gps["ant_x"].to_numpy(), gps["ant_y"].to_numpy(), gps["ant_z"].to_numpy())
    lat, lon = np.radians(lat), np.radians(lon)
    # ECEF components of the local east, north and up unit vectors.
    east = np.stack([-np.sin(lon), np.cos(lon), np.zeros_like(lon)])
    north = np.stack([-np.sin(lat) * np.cos(lon), -np.sin(lat) * np.sin(lon), np.cos(lat)])
    up = np.stack([np.cos(lat) * np.cos(lon), np.cos(lat) * np.sin(lon), np.sin(lat)])
    var = (
        east**2 * m["lon_sig"].to_numpy() ** 2
        + north**2 * m["lat_sig"].to_numpy() ** 2
        + up**2 * m["alt_sig"].to_numpy() ** 2
    )
    return gps.assign(ant_sigx=np.sqrt(var[0]), ant_sigy=np.sqrt(var[1]), ant_sigz=np.sqrt(var[2]))


def _nearest(times: pd.Series, df: pd.DataFrame, tolerance: float) -> pd.DataFrame:
    """The row of ``df`` (sorted by ``time``) nearest each of ``times``,
    all null where none is within ``tolerance`` seconds."""
    t = pd.DatetimeIndex(df["time"]).as_unit("ns").asi8
    q = pd.DatetimeIndex(times).as_unit("ns").asi8
    if len(t) == 0:
        return df.reindex(range(len(q)))
    i = np.clip(np.searchsorted(t, q), 0, len(t) - 1)
    before = np.maximum(i - 1, 0)
    i = np.where(np.abs(t[before] - q) < np.abs(t[i] - q), before, i)
    out = df.iloc[i].reset_index(drop=True)
    out.loc[np.abs(t[i] - q) > tolerance * 1e9] = np.nan
    return out


def internal_delays(df: pd.DataFrame, site: PosfilterSite) -> tuple[dict[str, float], list[str]]:
    """Each transponder's turn-around time [s] for GNATSS ``internal_delay``,
    and warnings.

    The site metadata's TAT wins: the TAT the SV3 logs per reply is its own
    setting, which is not updated when a transponder is reprogrammed (NBR1
    2025: logged 0.20/0.32/0.44 s after the change to 0.08/0.36/0.64 s; only
    the metadata values fit the acoustic geometry). The logged TAT is used
    only when the metadata has none, and must then be constant over the
    window; a logged TAT disagreeing with the metadata is a warning.
    """
    delays: dict[str, float] = {}
    warnings: list[str] = []
    for mt, tp in site.by_mt_id().items():
        logged = sorted(float(v) for v in df.loc[df["MT_ID"] == mt, "tat"].dropna().round(6).unique())
        if tp.tat is not None:
            delays[tp.pxp_id] = tp.tat
            if any(abs(v - tp.tat) > 1e-6 for v in logged):
                warnings.append(f"{tp.pxp_id}: logged TAT {logged} s differs from metadata {tp.tat} s; using metadata")
        elif len(logged) == 1:
            delays[tp.pxp_id] = float(logged[0])
            warnings.append(f"{tp.pxp_id}: no metadata TAT for the window; using logged {logged[0]} s")
        elif logged:
            raise ValueError(f"{tp.pxp_id} ({mt}): no metadata TAT and logged TAT changes within the window: {logged} s")
        else:
            raise ValueError(f"{tp.pxp_id} ({mt}): no metadata TAT and no replies in the window")
    return delays, warnings


def write_pxp_tt(df: pd.DataFrame, site: PosfilterSite, path: Path) -> tuple[int, int]:
    """Write travel times as pxp_tt (J2000 transmit time, µs per transponder
    in ``site.transponders`` order, 0 = no reply): the logged range minus the
    trigger delay, TAT left in for GNATSS's ``internal_delay``.

    Returns (pings, replies). Raises if a reply comes from a transponder the
    site does not list.
    """
    by_mt = site.by_mt_id()
    unknown = sorted(set(df["MT_ID"]) - set(by_mt))
    if unknown:
        raise ValueError(f"replies from transponders not in the site metadata window: {unknown}")

    cols = [tp.pxp_id for tp in site.transponders]
    tt_us = np.rint((df["TravelTime"] - site.trigger_delay) * 1e6).astype("int64")
    wide = (
        pd.DataFrame({"T_transmit": df["T_transmit"], "pxp": df["MT_ID"].map(lambda m: by_mt[m].pxp_id), "tt": tt_us})
        .pivot_table(index="T_transmit", columns="pxp", values="tt", aggfunc="first")
        .reindex(columns=cols)
        .fillna(0)
        .astype("int64")
        .sort_index()
    )
    j2000 = utc_to_j2000(wide.index.to_series())
    with open(path, "w") as f:
        for j, row in zip(j2000, wide.itertuples(index=False)):
            f.write(f"{j:.6f} " + " ".join(str(v) for v in row) + "\n")
    return len(wide), int((wide > 0).to_numpy().sum())


def write_sound_speed(df: pd.DataFrame, path: Path) -> int:
    """Write one profile as GNATSS reads it: ``depth speed``, depth negative down."""
    rows = df.sort_values("depth")
    with open(path, "w") as f:
        for r in rows.itertuples(index=False):
            f.write(f"{-r.depth:.4f} {r.speed:.6f}\n")
    return len(rows)


def gnatss_config(
    site: PosfilterSite,
    scope: Scope,
    out_dir: Path,
    *,
    internal_delays: dict[str, float],
    solver: SolverOptions | None = None,
) -> dict:
    """The GNATSS configuration for an export in ``out_dir``.

    Always a ``posfilter`` section; a ``solver`` section too when ``solver``
    is given, reading the exported ``sound_speed.txt``. The metadata heights
    are ellipsoidal, so the geoid undulation (which GNATSS adds to them) is 0.
    """
    out_dir = out_dir.resolve()
    lat, lon, alt = site.array_center
    forward, rightward, downward = site.atd_offset
    cfg = {
        "site_id": site.site_id,
        "campaign": scope.campaign,
        "time_origin": site.start.strftime("%Y-%m-%d %H:%M:%S"),
        "array_center": {"lat": lat, "lon": lon, "alt": alt},
        "transponders": [
            {"pxp_id": tp.pxp_id, "lat": tp.lat, "lon": tp.lon, "height": tp.height,
             "internal_delay": internal_delays[tp.pxp_id]}
            for tp in site.transponders
        ],  # fmt: skip
        "travel_times_correction": 0.0,
        "transducer_delay_time": 0.0,
        "posfilter": {
            "export": {"full": False},
            "atd_offsets": {"forward": forward, "rightward": rightward, "downward": downward},
            "input_files": {
                "novatel": {"path": str(out_dir / FILES["novatel"])},
                "novatel_std": {"path": str(out_dir / FILES["novatel_std"])},
                "gps_positions": {"path": str(out_dir / FILES["gps_positions"]), "format": "legacy"},
                "travel_times": {
                    "path": str(out_dir / FILES["travel_times"]),
                    "format": "legacy",
                    "loader_kwargs": {"is_j2k": True},
                },
            },
        },
        "output": {"path": str(out_dir / "output") + "/"},
    }
    if solver is not None:
        cfg["solver"] = {
            "reference_ellipsoid": WGS84,
            "geoid_undulation": 0.0,
            "gps_sigma_limit": solver.gps_sigma_limit,
            "distance_limit": solver.distance_limit,
            "harmonic_mean_start_depth": solver.harmonic_mean_start_depth,
            "input_files": {"sound_speed": {"path": str(out_dir / FILES["sound_speed"])}},
        }
    return cfg


def export_posfilter(
    reader: Level1Reader,
    site: PosfilterSite,
    scope: Scope,
    out_dir: str | Path,
    *,
    positions: str = "DFOP00",
    sound_speed: bool = True,
    solver: SolverOptions | None = None,
    position_sigmas: str = "logged",
) -> ExportSummary:
    """Export a GNATSS posfilter run for ``site``'s window into ``out_dir``.

    Parameters
    ----------
    reader : Level1Reader
        Over the Level 1 catalog.
    site : PosfilterSite
        Site parameters and window (:meth:`PosfilterSite.from_metadata`).
    scope : Scope
        Network, station, campaign of the Level 1 rows.
    out_dir : str or Path
        Directory to write; created if needed.
    positions : str
        File type of the antenna position stream: ``"DFOP00"`` (SV3 GNSS
        block) or ``"QCPIN"``; a PPP table replaces this at Level 3.
    sound_speed : bool
        Also export the latest sound speed profile at or before the window end.
    solver : SolverOptions, optional
        Add a GNATSS ``solver`` section; needs the sound speed profile.
    position_sigmas : str
        Antenna sigmas: ``"logged"`` (the stream's own) or ``"insstdev"``
        (:func:`insstdev_sigmas`, for streams that log the INS position).

    Returns
    -------
    ExportSummary
    """
    if position_sigmas not in ("logged", "insstdev"):
        raise ValueError(f"position_sigmas must be 'logged' or 'insstdev', not {position_sigmas!r}")
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    t0, t1 = site.start, site.end
    s = ExportSummary(str(out.resolve()), site.site_id, site.survey, t0.isoformat(), t1.isoformat())

    def read(table, **kw):
        df = reader.read(table, scope, t0, t1, **kw)
        if df.empty:
            raise ValueError(f"no {table} rows for {scope} in {t0} - {t1}")
        return df

    inspva = read("inspva")
    s.inspva_rows, s.inspva_good = len(inspva), write_inspvaa(inspva, out / FILES["novatel"])
    insstdev = read("insstdev")
    s.insstdev_rows, s.insstdev_good = len(insstdev), write_insstdeva(insstdev, out / FILES["novatel_std"])
    if s.inspva_good == 0:
        s.warnings.append("no INSPVA row has FINESTEERING and INS_SOLUTION_GOOD (statuses missing?)")
    elif s.inspva_good < s.inspva_rows:
        s.warnings.append(f"{s.inspva_rows - s.inspva_good} INSPVA rows lack good time/INS status; GNATSS drops them")

    gps = read("gnss_antenna_positions", file_type=positions)
    if position_sigmas == "insstdev":
        gps = insstdev_sigmas(gps, insstdev)
    s.gps_sigmas = position_sigmas
    s.gps_rows, s.gps_dropped_no_sigma = write_gps_pos_freed(gps, out / FILES["gps_positions"])

    tt = read("travel_times")
    s.internal_delays, tat_warnings = internal_delays(tt, site)
    s.warnings += tat_warnings
    s.pings, s.replies = write_pxp_tt(tt, site, out / FILES["travel_times"])

    if solver is not None and not sound_speed:
        raise ValueError("the solver needs the sound speed profile")
    if sound_speed:
        sv = reader.read("sound_speed", scope, end=t1)
        if sv.empty:
            s.warnings.append("no sound speed profile at or before the window end")
        else:
            latest = sv[sv["time"] == sv["time"].max()]
            if latest["stream_edid"].nunique() > 1:
                latest = latest[latest["stream_edid"] == latest["stream_edid"].iloc[0]]
                s.warnings.append("several sound speed profiles at the same time; exported the first stream")
            s.sound_speed_rows = write_sound_speed(latest, out / FILES["sound_speed"])
    if solver is not None and s.sound_speed_rows == 0:
        raise ValueError(f"no sound speed profile for {scope} at or before {t1}; the solver needs one")

    cfg = gnatss_config(site, scope, out, internal_delays=s.internal_delays, solver=solver)
    (out / "config.yaml").write_text(json.dumps(cfg, indent=2) + "\n")
    (out / "summary.json").write_text(json.dumps(asdict(s), indent=2) + "\n")
    for w in s.warnings:
        logger.warning("%s: %s", out, w)
    return s
