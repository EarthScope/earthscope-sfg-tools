"""Community-standard data models for GNSS-A acoustic exchange."""

from __future__ import annotations

import datetime
from typing import Any

import pandera.pandas as pa
import pymap3d as pm
from pandera.typing import Series
from pydantic import BaseModel


class SFGDSTFSeafloorAcousticData(pa.DataFrameModel):
    MT_ID: Series[str] = pa.Field(description="ID of mirror transponder")
    TravelTime: Series[float] = pa.Field(description="Observed travel time [s]", ge=0)
    T_transmit: Series[float] = pa.Field(description="Transmission time [s]", ge=0)
    X_transmit: Series[float] = pa.Field(description="Transmit position X ECEF [m]")
    Y_transmit: Series[float] = pa.Field(description="Transmit position Y ECEF [m]")
    Z_transmit: Series[float] = pa.Field(description="Transmit position Z ECEF [m]")
    T_receive: Series[float] = pa.Field(description="Reception time [s]", ge=0)
    X_receive: Series[float] = pa.Field(description="Receive position X ECEF [m]")
    Y_receive: Series[float] = pa.Field(description="Receive position Y ECEF [m]")
    Z_receive: Series[float] = pa.Field(description="Receive position Z ECEF [m]")

    TDC_ID: Series[str] | None = pa.Field(default=None)
    aSNR: Series[float] | None = pa.Field(default=None)
    acc: Series[int] | None = pa.Field(default=None)
    dbV: Series[float] | None = pa.Field(default=None)
    quality_flag: Series[str] | None = pa.Field(default=None)
    ant_X0: Series[float] | None = pa.Field(default=None)
    ant_Y0: Series[float] | None = pa.Field(default=None)
    ant_Z0: Series[float] | None = pa.Field(default=None)
    ant_sigX0: Series[float] | None = pa.Field(default=None)
    ant_sigY0: Series[float] | None = pa.Field(default=None)
    ant_sigZ0: Series[float] | None = pa.Field(default=None)
    ant_X1: Series[float] | None = pa.Field(default=None)
    ant_Y1: Series[float] | None = pa.Field(default=None)
    ant_Z1: Series[float] | None = pa.Field(default=None)
    ant_sigX1: Series[float] | None = pa.Field(default=None)
    ant_sigY1: Series[float] | None = pa.Field(default=None)
    ant_sigZ1: Series[float] | None = pa.Field(default=None)
    heading0: Series[float] | None = pa.Field(default=None, ge=0, le=360)
    pitch0: Series[float] | None = pa.Field(default=None, ge=-90, le=90)
    roll0: Series[float] | None = pa.Field(default=None, ge=-180, le=180)
    roll1: Series[float] | None = pa.Field(default=None, ge=-180, le=180)


class SFGDTSFSite(BaseModel):
    Site_name: str
    Campaign: str
    TimeOrigin: datetime.datetime
    RefFrame: str = "ITRF"
    MTlist: list[str] = []
    MT_appPos: dict[str, list[float]] = {}
    ATDoffset: list[float] = [0.0, 0.0, 0.0]

    @classmethod
    def from_site_vessel(cls, site: Any, vessel: Any, campaign_id: str) -> "SFGDTSFSite":
        mt_app_pos = {}
        for benchmark in site.benchmarks:
            east, north, up = pm.geodetic2ecef(
                lat=benchmark.aPrioriLocation.latitude,
                lon=benchmark.aPrioriLocation.longitude,
                alt=benchmark.aPrioriLocation.elevation,
            )
            mt_app_pos[benchmark.benchmarkID] = [east, north, up]

        found_campaign = None
        for camp in site.campaigns:
            if camp.name == campaign_id:
                found_campaign = camp
                break

        if found_campaign is None:
            raise ValueError(f"Campaign ID {campaign_id} not found in site campaigns.")

        return cls(
            Site_name=site.names[0],
            Campaign=found_campaign.name,
            TimeOrigin=site.timeOrigin,
            RefFrame=(site.referenceFrames[0].name if site.referenceFrames else "ITRF"),
            MTlist=[b.benchmarkID for b in site.benchmarks],
            MT_appPos=mt_app_pos,
            ATDoffset=[vessel.atdOffsets[0].x, vessel.atdOffsets[0].y, vessel.atdOffsets[0].z],
        )
