"""Community-standard SFGDTSF site metadata model."""

from pydantic import BaseModel
import datetime


class SFGDTSFSite(BaseModel):
    """Community-standard site metadata for a GNSS-A seafloor site.

    Attributes:
        Site_name: Short human-readable site identifier.
        Campaign: Campaign name this site belongs to.
        TimeOrigin: Reference epoch for the campaign.
        RefFrame: Terrestrial reference frame. Defaults to
            ``'ITRF20'``.
        MTlist: Ordered list of mirror transponder IDs.
        MT_appPos: Approximate ECEF positions keyed by transponder ID.
        ATDoffset: ``[dEast, dNorth, dUp]`` antenna-to-transducer
            offset in metres. Defaults to ``[0, 0, 0]``.
    """

    Campaign: str
    TimeOrigin: datetime.datetime
    RefFrame: str = "ITRF20"
    MTlist: list[str] = []
    MT_appPos: dict[str, list[float]] = {}
    ATDoffset: list[float] = [0.0, 0.0, 0.0]
