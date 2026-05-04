from pydantic import BaseModel
import datetime 

class SFGDTSFSite(BaseModel):
    Site_name: str
    Campaign: str
    TimeOrigin: datetime.datetime
    RefFrame: str = "ITRF20"
    MTlist: list[str] = []
    MT_appPos: dict[str, list[float]] = {}
    ATDoffset: list[float] = [0.0, 0.0, 0.0]


