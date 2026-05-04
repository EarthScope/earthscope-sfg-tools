"""Metadata helpers for RINEX workflows.

Core metadata model + validation live in ``earthscope_sfg_tools.novatel_tools.utils``
and are re-exported here to preserve historical import paths.
"""

from ..novatel_tools.utils import (
    MetadataModel,
    check_metadata,
    check_metadata_path,
    get_metadatav2,
)


def get_metadata(site: str, serialNumber: str = "XXXXXXXXXX") -> dict:
    # TODO: these are placeholder values, need to use real metadata
    return {
        "markerName": site,
        "markerType": "WATER_CRAFT",
        "observer": "PGF",
        "agency": "Pacific GPS Facility",
        "receiver": {
            "serialNumber": "XXXXXXXXXX",
            "model": "NOV OEMV1",
            "firmware": "4.80",
        },
        "antenna": {
            "serialNumber": "ACC_G5ANT_52AT1",
            "model": "NONE",
            "position": [
                0.000,
                0.000,
                0.000,
            ],  # reference position for site what ref frame?
            "offsetHEN": [0.0, 0.0, 0.0],  # read from lever arms file?
        },
    }


__all__ = [
    "MetadataModel",
    "check_metadata",
    "check_metadata_path",
    "get_metadata",
    "get_metadatav2",
]
