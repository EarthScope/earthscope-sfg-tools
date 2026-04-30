from pydantic import BaseModel, Field
import datetime

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
    receiver_serial: str | None = Field(
        default="XXXXXXXXXX", description="Receiver serial number"
    )
    antenna_position: list[float] | None = Field(
        default=[0.0, 0.0, 0.0], description="Antenna position [X, Y, Z]"
    )
    antenna_offsetHEN: list[float] | None = Field(
        default=[0.0, 0.0, 0.0], description="Antenna offset [H, E, N]"
    )
    antenna_model: str | None = Field(
        default="NOV850 NONE", description="Antenna model"
    )
    antenna_serial: str | None = Field(
        default="987654321", description="Antenna serial number"
    )


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


def check_metadata_path(metadata_path: Path | str) -> str:
    """Validate and normalize a metadata file path.

    Parameters
    ----------
    metadata_path : Path | str
        Path to the metadata JSON file.

    Returns
    -------
    str
        The validated metadata file path as a string.

    Raises
    ------
    AssertionError
        If the metadata file does not exist.
    ValueError
        If the metadata file cannot be parsed into ``MetadataModel``.
    """
    if isinstance(metadata_path, str):
        metadata_path = Path(metadata_path)
    assert metadata_path.exists(), f"Metadata file {str(metadata_path)} does not exist"
    with open(metadata_path) as f:
        metadata_dict = json.load(f)
    try:
        _ = MetadataModel(**metadata_dict).model_dump()
        return str(metadata_path)
    except Exception as e:
        raise ValueError(
            f"Error parsing metadata file {str(metadata_path)}: {e}"
        ) from e


def check_metadata(meta: dict | MetadataModel) -> dict:
    """Validate and normalize metadata input into a dictionary.

    Parameters
    ----------
    meta : dict or MetadataModel
        Metadata to validate and normalize.

    Returns
    -------
    dict
        A dictionary representation of the validated metadata.

    Raises
    ------
    ValueError
        If validation fails or ``meta`` is an unsupported type.
    """
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
