"""Pydantic models representing Sonardyne/SV3 event payloads."""

from __future__ import annotations

from decimal import Decimal
from enum import Enum

from pydantic import BaseModel, Field, field_validator

from ..constants import GNSS_START_TIME


class SV3GPSQuality(Enum):
    """GPS fix quality codes reported in Sonardyne GNSS data."""

    FIX_NOT_AVAILABLE = 0
    SINGLE_POINT = 1
    PSEUDO_RANGE_DIFFERENTIAL = 2
    REAL_TIME_KINEMATIC = 4
    FLOAT_RTK = 5
    DEAD_RECKONING = 6
    MANUAL_INPUT_MODE = 7
    SIMULATION_MODE = 8
    WAAS_SBAS = 9


class SonardyneSolutionStatus(Enum):
    """NovAtel/Sonardyne position solution status codes."""

    SOL_COMPUTED = 0
    INSUFFICIENT_OBS = 1
    NO_CONVERGENCE = 2
    SINGULARITY = 3
    COV_TRACE = 4
    TEST_DIST = 5
    COLD_START = 6
    V_H_LIMIT = 7
    VARIANCE = 8
    RESIDUALS = 9
    INTEGRITY_WARNING = 13
    PENDING = 18
    INVALID_FIX = 19
    UNAUTHORIZED = 20
    INVALID_RATE = 22


class SonardynePositionType(Enum):
    """NovAtel/Sonardyne position type identifiers."""

    NONE = 0
    FIXEDPOS = 1
    FIXEDHEIGHT = 2
    DOPPLER_VELOCITY = 8
    SINGLE = 16
    PSRDIFF = 17
    WAAS = 18
    PROPAGATED = 19
    FLOAT_L1 = 32
    NARROW_FLOAT = 34
    L1_INT = 48
    WIDE_INT = 49
    NARROW_INT = 50
    RTK_DIRECT_INS = 51
    INS_SBAS = 52
    INS_PSRSP = 53
    INS_PSRDIFF = 54
    INS_RTKFLOAT = 55
    INS_RTKFIXED = 56
    PPP_CONVERGING = 68
    PPP = 69
    OPERATIONAL = 70
    WARNING = 71
    OUT_OF_BOUNDS = 72
    INS_PPP_CONVERGING = 73
    INS_PPP = 74
    PPP_BASIC_CONVERGING = 77
    PPP_BASIC = 78
    INS_PPP_BASIC_CONVERGING = 79
    INS_PPP_BASIC = 80


class TimeData(BaseModel):
    """Timestamp block from a Sonardyne SV3 event JSON."""

    common: Decimal = Field(
        description="TZ unaware UNIX time", ge=GNSS_START_TIME.timestamp()
    )
    instrument: Decimal = Field(description="Instrument time in seconds", ge=0)
    start_count: int = Field(description="Start count for the time", ge=0)
    status: str = Field(description="Status of the time data")


class SonardyneHeadingData(BaseModel):
    """GNSS dual-antenna heading solution from a Sonardyne event log."""

    gpst: Decimal = Field(description="GPS time in seconds since GNSS start time", ge=0)
    h: Decimal = Field(description="GNSS Computed Heading in degrees", ge=0, le=360)
    p: Decimal = Field(description="GNSS Computed Pitch in degrees", ge=-90, le=90)
    position_type: SonardynePositionType = Field(description="Type of position data")
    receiver_status: str = Field(description="Status of the receiver")
    sdh: Decimal | None = Field(
        description="Standard deviation of heading in degrees", ge=0
    )
    sdp: Decimal | None = Field(
        description="Standard deviation of pitch in degrees", ge=0
    )
    solution_type: SonardyneSolutionStatus = Field(
        description="Solution type of the heading data"
    )
    sv_used: int = Field(description="Number of satellites used in the solution", ge=0)
    sv_visible: int = Field(
        description="Number of satellites visible", ge=0, alias="sv_visable"
    )
    time: TimeData = Field(description="Time data associated with the log")


class SonardyneINSData(BaseModel):
    """NovAtel SPAN INS attitude and velocity solution from a Sonardyne event log."""

    gpst: Decimal = Field(description="GPS time in seconds since GNSS start time", ge=0)
    h: Decimal = Field(description="SPAN INS Computed Heading in degrees", ge=0, le=360)
    p: Decimal = Field(description="SPAN INS Computed Pitch in degrees", ge=-90, le=90)
    r: Decimal = Field(description="SPAN INS Computed Roll in degrees", ge=-180, le=180)
    receiver_status: str = Field(description="Status of the receiver")
    solution_type: SonardyneSolutionStatus = Field(
        description="Solution type of the INS data"
    )
    time: TimeData = Field(description="Time data associated with the log")
    velx: Decimal = Field(
        description="SPAN INS measured acceleration X axis in m/s^2", alias="vx"
    )
    vely: Decimal = Field(
        description="SPAN INS measured acceleration Y axis in m/s^2", alias="vy"
    )
    velz: Decimal = Field(
        description="SPAN INS measured acceleration Z axis in m/s^2", alias="vz"
    )


class SonardyneRangeData(BaseModel):
    """Raw range string and timestamp from a Sonardyne acoustic reply."""

    raw: str = Field(description="Raw range data as a string")
    time: TimeData = Field(description="Time data associated with the range data")


class SonardyneGNSSData(BaseModel):
    """GNSS position fix and standard deviations from a Sonardyne event log."""

    hae: Decimal = Field(
        description="Height above ellipsoid in meters", ge=-1000, le=1000
    )
    latitude: Decimal = Field(description="Latitude in degrees", ge=-90, le=90)
    longitude: Decimal = Field(description="Longitude in degrees", ge=-180, le=180)
    q: SV3GPSQuality = Field(description="Quality indicator")
    sdx: Decimal | None = Field(description="Standard deviation east [m]", ge=0)
    sdy: Decimal | None = Field(description="Standard deviation north [m]", ge=0)
    sdz: Decimal | None = Field(description="Standard deviation up [m]", ge=0)
    separation: Decimal | None = Field(description="Separation")
    time: TimeData = Field(description="Time data associated with the log")


class SonardyneAHRSData(BaseModel):
    """AHRS attitude (heading, pitch, roll) and acceleration from a Sonardyne event log."""

    acx: Decimal = Field(description="Acceleration X axis in m/s^2")
    acy: Decimal = Field(description="Acceleration Y axis in m/s^2")
    acz: Decimal = Field(description="Acceleration Z axis in m/s^2")
    h: Decimal = Field(description="Heading in degrees", ge=Decimal(0), le=Decimal(360))
    h_mag: Decimal | None = Field(
        description="Magnetic heading in degrees", ge=Decimal(0), le=Decimal(360)
    )
    p: Decimal = Field(description="Pitch in degrees", ge=Decimal(-90), le=Decimal(90))
    r: Decimal = Field(description="Roll in degrees", ge=Decimal(-180), le=Decimal(180))
    time: TimeData = Field(description="Time data associated with the log")


class SonardyneRangeDiagnosticData(BaseModel):
    """Acoustic signal quality diagnostics for one transponder reply."""

    dbv: Decimal = Field(description="Decibel voltage in volts")
    snr: Decimal = Field(description="Signal-to-noise ratio in dB")
    xc: Decimal = Field(description="Cross-correlation % - signal quality")

    @field_validator("dbv", "snr", "xc", mode="before")
    @classmethod
    def convert_from_list(cls, value: list[int | float] | int | float):
        if isinstance(value, list) and len(value) == 1:
            return Decimal(value[0])
        return Decimal(value)


class SonardyneRangeReplyData(BaseModel):
    """Acoustic range reply from one transponder, including TAT and diagnostics."""

    cn: str = Field(description="transponder ID", max_length=20)
    diag: SonardyneRangeDiagnosticData = Field(description="Range diagnostic data")
    range: Decimal = Field(description="Two-way travel time in seconds")
    tat: Decimal = Field(description="Beacon turn around time in seconds", ge=0)

    @field_validator("tat", mode="after")
    @classmethod
    def convert_tat(cls, value: Decimal):
        return float(value) / 1000.0


class SonardyneObservations(BaseModel):
    """Bundle of all sensor observations attached to one Sonardyne event."""

    AHRS: SonardyneAHRSData | None = Field(description="AHRS data")
    GNSS: SonardyneGNSSData | None = Field(description="GNSS data")
    NOV_HEADING: SonardyneHeadingData | None = Field(
        description="Sonardyne heading data"
    )
    NOV_INS: SonardyneINSData | None = Field(description="Sonardyne INS data")
    NOV_RANGE: SonardyneRangeData | None = Field(description="Sonardyne range data")


class SonardyneRangeEvent(BaseModel):
    """Full Sonardyne range (reply) event parsed from a DFOP00 JSONL log."""

    event: str = "range"
    event_id: int = Field(description="Tracking-cycle ID", ge=0)
    observations: SonardyneObservations = Field(description="Event observations")
    range: SonardyneRangeReplyData = Field(description="Range reply data")
    sequence: int = Field(description="Sequence ID", ge=0)
    time: TimeData = Field(description="Event time")
    uid: str | None = Field(description="Unique identifier", max_length=50)


class SonardyneInterrogationEvent(BaseModel):
    """Full Sonardyne interrogation event parsed from a DFOP00 JSONL log."""

    event: str = "interrogation"
    event_id: int = Field(description="Tracking-cycle ID", ge=0)
    observations: SonardyneObservations = Field(description="Observations")
    sequence: int = Field(description="Sequence ID", ge=0)
    time: TimeData = Field(description="Event time")
    type: str = Field(description="Interrogation type")


NovatelInterrogationEvent = SonardyneInterrogationEvent
NovatelRangeEvent = SonardyneRangeEvent
