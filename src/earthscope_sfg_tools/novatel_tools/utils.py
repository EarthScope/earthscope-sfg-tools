import datetime
import json
from pathlib import Path
import uuid

from pydantic import BaseModel, Field


class MetadataModel(BaseModel):
    marker_name: str = Field(..., description="Site name")
    rinex_version: str | None = Field(default="2.11", description="RINEX version")
    rinex_type: str | None = Field(default="O", description="RINEX type")
    rinex_system: str | None = Field(default="G", description="RINEX system")
    marker_number: str | None = Field(default="0001", description="Marker number")
    marker_type: str | None = Field(default="GEODETIC", description="Marker type")
    observer: str | None = Field(default="EarthScope", description="Observer name")
    agency: str | None = Field(default="EarthScope", description="Agency name")
    program: str | None = Field(default="gnsstools", description="Program name")
    run_by: str | None = Field(default="", description="Run by")
    date: str | None = Field(
        default_factory=lambda: datetime.datetime.now(tz=datetime.UTC).isoformat(),
        description="Date",
    )
    receiver_model: str | None = Field(default="NOV", description="Receiver model")
    receiver_serial: str | None = Field(default="XXXXXXXXXX", description="Receiver serial number")
    antenna_position: list[float] | None = Field(
        default=[0.0, 0.0, 0.0], description="Antenna position [X, Y, Z]"
    )
    antenna_offsetHEN: list[float] | None = Field(
        default=[0.0, 0.0, 0.0], description="Antenna offset [H, E, N]"
    )
    antenna_model: str | None = Field(default="NOV850 NONE", description="Antenna model")
    antenna_serial: str | None = Field(default="987654321", description="Antenna serial number")


def get_metadatav2(
    site: str,
    serialNumber: str = "XXXXXXXXXX",
    antennaPosition: list = None,
    antennaeOffsetHEN: list = None,
) -> dict:
    # TODO: these are placeholder values, need to use real metadata
    if antennaPosition is None:
        antennaPosition = [0, 0, 0]
    if antennaeOffsetHEN is None:
        antennaeOffsetHEN = [0, 0, 0]

    return {
        "rinex_version": "2.11",
        "rinex_type": "O",
        "rinex_system": "G",
        "marker_name": site,
        "marker_number": "0001",
        "marker_type": "GEODETIC",
        "observer": "EarthScope",
        "agency": "EarthScope",
        "program": "gnsstools",
        "run_by": "",
        "date": "",
        "receiver_model": "NOV",
        "receiver_serial": serialNumber,
        "receiver_firmware": "0.0.0",
        "antenna_model": "NOV850 NONE",
        "antenna_serial": "987654321",
        "antenna_position": antennaPosition,
        "antenna_offsetHEN": antennaeOffsetHEN,
    }


def check_metadata_path(metadata_path: Path | str) -> str:
    if isinstance(metadata_path, str):
        metadata_path = Path(metadata_path)
    assert metadata_path.exists(), f"Metadata file {str(metadata_path)} does not exist"
    with open(metadata_path) as f:
        metadata_dict = json.load(f)
    try:
        _ = MetadataModel(**metadata_dict).model_dump()
        return str(metadata_path)
    except Exception as e:
        raise ValueError(f"Error parsing metadata file {str(metadata_path)}: {e}") from e


def check_metadata(meta: dict | MetadataModel) -> dict:
    if isinstance(meta, dict):
        try:
            _ = MetadataModel(**meta).model_dump()
            return meta
        except Exception as e:
            raise ValueError(f"Error parsing metadata dictionary: {e}") from e
    elif isinstance(meta, MetadataModel):
        return meta.model_dump()
    else:
        raise ValueError(f"Metadata must be a dict or MetadataModel, got {type(meta)}")


def resolve_metadata(
    metadata: dict | MetadataModel | Path | str | None = None,
    site: str | None = None,
) -> dict | str:
    """Validate or generate metadata for RINEX conversion workflows.

    If ``metadata`` is provided, this validates either a metadata mapping/model or
    a path to a metadata JSON file. If ``metadata`` is omitted, site-based default
    metadata is generated.
    """
    if metadata is not None:
        if isinstance(metadata, (str, Path)):
            return check_metadata_path(metadata)
        if isinstance(metadata, (dict, MetadataModel)):
            return check_metadata(metadata)
        raise ValueError(
            f"Metadata must be a dict, MetadataModel, or path to a JSON file, got {type(metadata)}"
        )

    if site is None:
        raise ValueError("Either metadata or site must be provided")
    if not isinstance(site, str):
        raise ValueError(f"Site must be a string, got {type(site)}")
    if len(site) != 4:
        raise ValueError(f"Site must be 4 characters long, got {site}")
    return get_metadatav2(site, serialNumber=uuid.uuid4().hex[:10])


def write_metadata_json(metadata: dict, output_path: Path | str) -> Path:
    """Write validated metadata to a JSON file and return the path."""
    output_path = Path(output_path)
    with open(output_path, "w") as f:
        json.dump(metadata, f, indent=4)
    return output_path
