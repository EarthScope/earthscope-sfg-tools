"""
Scope identifies where a dataset belongs in the seafloor geodesy data lake.

Every Iceberg table carries network, station, and campaign columns so data
from all stations can live in a single table and be queried by scope.
"""

from dataclasses import dataclass


@dataclass(frozen=True)
class Scope:
    """
    A (network, station, campaign) triple that labels every row written to an
    Iceberg table.

    Attributes:
        network:  The network identifier (e.g. "SEAFLOOR").
        station:  The station identifier (e.g. "NCL1").
        campaign: The campaign or session identifier (e.g. "2024-CRUISE-01").
    """

    network: str
    station: str
    campaign: str
