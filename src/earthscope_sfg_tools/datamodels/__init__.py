"""Pydantic and Pandera data models for SFG metadata and observation data."""

from . import metadata, observationdata
from .metadata import (
    Benchmark,
    Campaign,
    CatalogType,
    MetaDataCatalog,
    NetworkData,
    SFGDTSFSite,
    Site,
    StationData,
    Survey,
    SurveyType,
    Vessel,
    classify_survey_type,
    import_site,
    import_vessel,
)
from .observationdata import (
    AcousticDataFrame,
    GARPOSShotDataFrame,
    IMUPositionDataFrame,
    INSPVAASchema,
    INSSTDEVSchema,
    KinPositionDataFrame,
    NovatelInterrogationEvent,
    NovatelRangeEvent,
    SFGDSTFSeafloorAcousticData,
    SoundVelocityDataFrame,
    SV3InterrogationData,
    SV3ReplyData,
)

__all__ = [
    # submodules
    "metadata",
    "observationdata",
    # metadata
    "Benchmark",
    "Campaign",
    "CatalogType",
    "MetaDataCatalog",
    "NetworkData",
    "SFGDTSFSite",
    "Site",
    "StationData",
    "Survey",
    "SurveyType",
    "Vessel",
    "classify_survey_type",
    "import_site",
    "import_vessel",
    # observationdata
    "AcousticDataFrame",
    "GARPOSShotDataFrame",
    "IMUPositionDataFrame",
    "INSPVAASchema",
    "INSSTDEVSchema",
    "KinPositionDataFrame",
    "NovatelInterrogationEvent",
    "NovatelRangeEvent",
    "SFGDSTFSeafloorAcousticData",
    "SoundVelocityDataFrame",
    "SV3InterrogationData",
    "SV3ReplyData",
]
