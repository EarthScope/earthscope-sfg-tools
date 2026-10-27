"""
Site parameters for a GNATSS posfilter run, from the EarthScope site and
campaign metadata (``datamodels.metadata.earthscope``).

A run covers one window: a campaign survey's, or an explicit one. The
transponders are the benchmarks the survey names (or all), each active over
the window. Missing metadata is an error rather than a default: a wrong
array center or lever arm silently moves every transducer position.
"""

from dataclasses import dataclass
from datetime import datetime

from ..datamodels.metadata.earthscope.site import Site
from ..datamodels.observationdata.constants import TRIGGER_DELAY_SV3
from .timescales import as_utc


@dataclass(frozen=True)
class PosfilterTransponder:
    """A seafloor transponder as GNATSS names and places it."""

    pxp_id: str  # GNATSS transponder id: the benchmark name, e.g. NBR1-1
    address: str  # acoustic address, e.g. 5209 (travel_times.MT_ID IR5209)
    lat: float  # [deg]
    lon: float  # [deg]
    height: float  # ellipsoidal height [m], negative below the ellipsoid
    tat: float | None = None  # turn-around time [s] over the window; None if the metadata has none


@dataclass(frozen=True)
class PosfilterSite:
    """Everything a posfilter run needs from the site metadata.

    ``atd_offset`` is (forward, rightward, downward) in metres, the GNATSS
    body frame, from the metadata's ``AtdOffset`` (x, y, z) taken in that
    order. ``trigger_delay`` is the transceiver delay [s] included in the
    logged two-way range (0 when the metadata says it is not included).
    """

    site_id: str
    start: datetime
    end: datetime
    array_center: tuple[float, float, float]  # lat [deg], lon [deg], alt [m]
    transponders: tuple[PosfilterTransponder, ...]
    atd_offset: tuple[float, float, float]
    trigger_delay: float
    survey: str | None = None

    def by_mt_id(self) -> dict[str, PosfilterTransponder]:
        """Transponders keyed by travel_times.MT_ID, e.g. ``IR5209``."""
        return {"IR" + t.address: t for t in self.transponders}

    @classmethod
    def from_metadata(
        cls,
        site: Site,
        campaign: str,
        *,
        survey: str | None = None,
        start: datetime | str | None = None,
        end: datetime | str | None = None,
        site_id: str | None = None,
    ) -> "PosfilterSite":
        """Build the site parameters for a survey of ``campaign``, or a window.

        Parameters
        ----------
        site : Site
            EarthScope site metadata (``Site.from_json``).
        campaign : str
            Campaign name, e.g. ``2025_A_1126``.
        survey : str, optional
            Survey id; its window and benchmarks define the run.
        start, end : datetime or ISO str, optional
            Explicit UTC window, when no survey is given (default: the
            campaign's).
        site_id : str, optional
            GNATSS site id; the site's first name by default.
        """
        camp = _one([c for c in site.campaigns if c.name == campaign], f"campaign {campaign!r}")
        benchmark_ids = None
        if survey is not None:
            if start is not None or end is not None:
                raise ValueError("give a survey or a start/end window, not both")
            sv = _one([s for s in camp.surveys if s.id == survey], f"survey {survey!r} in {campaign}")
            t0, t1, benchmark_ids = as_utc(sv.start), as_utc(sv.end), set(sv.benchmarkIDs)
        else:
            t0 = as_utc(start) if start is not None else as_utc(camp.start)
            t1 = as_utc(end) if end is not None else as_utc(camp.end)
        if t1 <= t0:
            raise ValueError(f"empty window {t0} - {t1}")

        if site.arrayCenter is None or site.arrayCenter.latitude is None or site.arrayCenter.longitude is None:
            raise ValueError("site metadata has no arrayCenter")
        center = (
            site.arrayCenter.latitude,
            site.arrayCenter.longitude,
            site.arrayCenter.elevation or 0.0,
        )

        transponders = []
        for bm in site.benchmarks:
            if benchmark_ids is not None and bm.name not in benchmark_ids:
                continue
            if not _covers(bm.start, bm.end, t0, t1):
                continue
            active = [t for t in bm.transponders or [] if _covers(t.start, t.end, t0, t1)]
            if not active:
                continue
            tp = _one(active, f"transponder of benchmark {bm.name} over the window")
            loc = bm.aPrioriLocation
            if loc is None or None in (loc.latitude, loc.longitude, loc.elevation):
                raise ValueError(f"benchmark {bm.name} has no complete aPrioriLocation")
            tats = [x.value / 1000 for x in tp.tat or [] if _covers(x.start, x.end, t0, t1)]
            if len(set(tats)) > 1:
                raise ValueError(
                    f"{bm.name} ({tp.address}): TAT changes within the window per the metadata "
                    f"({sorted(set(tats))} s); split the window at the change"
                )
            transponders.append(
                PosfilterTransponder(
                    bm.name, tp.address, loc.latitude, loc.longitude, loc.elevation,
                    tat=tats[0] if len(tats) == 1 else None,
                )  # fmt: skip
            )
        if benchmark_ids is not None and len(transponders) != len(benchmark_ids):
            found = {t.pxp_id for t in transponders}
            raise ValueError(f"survey {survey!r}: no active transponder for {sorted(benchmark_ids - found)}")
        if not transponders:
            raise ValueError("no transponders active over the window")

        vessel = camp.vessel
        if vessel is None or not vessel.atdOffsets:
            raise ValueError(f"campaign {campaign} has no vessel atdOffsets")
        offsets = vessel.atdOffsets
        if len(offsets) > 1:
            serials = {t.serialNumber for t in vessel.acousticTransducers if _covers(t.start, t.end, t0, t1)}
            offsets = [o for o in offsets if o.serialNumber in serials]
        atd = _one(offsets, f"ATD offset for campaign {campaign}'s transducer")

        trigger_delay = TRIGGER_DELAY_SV3
        txs = [t for t in vessel.acousticTransceivers if _covers(t.start, t.end, t0, t1)]
        if txs:
            tx = _one(txs, "acoustic transceiver over the window")
            if tx.delayIncludedInTWTT is False:
                trigger_delay = 0.0
            elif tx.triggerDelay is not None:
                trigger_delay = tx.triggerDelay

        return cls(
            site_id=site_id or site.names[0],
            start=t0,
            end=t1,
            array_center=center,
            transponders=tuple(sorted(transponders, key=lambda t: t.pxp_id)),
            atd_offset=(atd.x, atd.y, atd.z),
            trigger_delay=trigger_delay,
            survey=survey,
        )


def _covers(start: datetime | None, end: datetime | None, t0: datetime, t1: datetime) -> bool:
    """Whether [start, end) (open where None) overlaps the window [t0, t1)."""
    return (start is None or as_utc(start) < t1) and (end is None or as_utc(end) > t0)


def _one(items: list, what: str):
    if len(items) != 1:
        raise ValueError(f"{'no' if not items else len(items)} {what}" + ("" if not items else ", want one"))
    return items[0]
