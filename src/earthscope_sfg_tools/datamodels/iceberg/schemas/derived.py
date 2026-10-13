"""
Level 2 and Level 3 tables: derived by the access API from Level 1.

Derived rows can combine several source logs, so these tables carry the
scope columns but no ``Log_Source``. Level 2 GNSS-A tables use Level 1
(SV3 pseudorange) antenna positions; Level 3 uses the Level 2 PPP
positions. The GNSS-A schemas are identical between levels — the tables
share a name and differ only by level namespace.
"""

from ._base import (
    BOOLEAN,
    DOUBLE,
    FLOAT,
    INT,
    SCOPE_COLUMNS,
    STRING,
    TIMESTAMPTZ,
    Column,
    TableDef,
)
from .level1 import ANTENNA_POSITION_COLUMNS

GNSS_ANTENNA_POSITIONS_PPP = TableDef(
    name="gnss_antenna_positions_ppp",
    level=2,
    doc=(
        "Antenna positions from PPP kinematic solutions of the Level 1 GNSS "
        "observations; lat/lon/height converted to ECEF at ingest."
    ),
    columns=(
        *SCOPE_COLUMNS,
        *ANTENNA_POSITION_COLUMNS,
        Column(
            "q", INT, "GPS fix quality; null for PPP rows", required=False, ge=0, le=9
        ),
        Column(
            "number_of_satellites",
            INT,
            "Number of satellites used in the solution",
            ge=0,
        ),
        Column("pdop", DOUBLE, "Position dilution of precision", ge=0),
        Column("wrms", DOUBLE, "Weighted RMS of position residuals [m]", ge=0),
    ),
)

_GARPOS_COLUMNS: tuple[Column, ...] = (
    *SCOPE_COLUMNS,
    Column("SET", STRING, "Name of observation subset", default="S01"),
    Column("LN", STRING, "Name of survey line", default="L01"),
    Column(
        "MT", STRING, "ID of mirror transponder (must match the site-parameter file)"
    ),
    Column("TT", DOUBLE, "Corrected travel time [s] (raw range - tat - trigger delay)"),
    Column(
        "ResiTT", DOUBLE, "Travel-time residual [ms] (populated by GARPOS)", default=0.0
    ),
    Column(
        "TakeOff",
        DOUBLE,
        "Ray-path takeoff angle [deg] (populated by GARPOS)",
        default=0.0,
    ),
    Column("gamma", DOUBLE, "Sound-speed variation correction term [m/s]", default=0.0),
    Column("flag", BOOLEAN, "True = shot excluded from processing", default=False),
    Column("ST", TIMESTAMPTZ, "Transmission time of acoustic signal, UTC"),
    Column("ant_e0", DOUBLE, "Antenna position east of array center at ST [m]"),
    Column("ant_n0", DOUBLE, "Antenna position north of array center at ST [m]"),
    Column("ant_u0", DOUBLE, "Antenna position up from array center at ST [m]"),
    Column("head0", DOUBLE, "Heading at ST [deg]"),
    Column("pitch0", DOUBLE, "Pitch at ST [deg]"),
    Column("roll0", DOUBLE, "Roll at ST [deg]"),
    Column("RT", TIMESTAMPTZ, "Reception time of acoustic signal, UTC"),
    Column("ant_e1", DOUBLE, "Antenna position east of array center at RT [m]"),
    Column("ant_n1", DOUBLE, "Antenna position north of array center at RT [m]"),
    Column("ant_u1", DOUBLE, "Antenna position up from array center at RT [m]"),
    Column("head1", DOUBLE, "Heading at RT [deg]"),
    Column("pitch1", DOUBLE, "Pitch at RT [deg]"),
    Column("roll1", DOUBLE, "Roll at RT [deg]"),
)


def _community_epoch_columns(suffix: str, label: str) -> tuple[Column, ...]:
    """Optional antenna position and sigma columns at one epoch."""
    return (
        *(
            Column(
                f"ant_{a}{suffix}",
                DOUBLE,
                f"Antenna position {a}, ECEF, at {label} [m]",
                required=False,
            )
            for a in "XYZ"
        ),
        *(
            Column(
                f"ant_sig{a}{suffix}",
                DOUBLE,
                f"Antenna position std dev {a} at {label} [m]",
                required=False,
                ge=0,
            )
            for a in "XYZ"
        ),
    )


def _community_attitude_columns(suffix: str, label: str) -> tuple[Column, ...]:
    return (
        Column(
            f"heading{suffix}",
            DOUBLE,
            f"Heading at {label} [deg]",
            required=False,
            ge=0,
            le=360,
        ),
        Column(
            f"pitch{suffix}",
            DOUBLE,
            f"Pitch at {label} [deg]",
            required=False,
            ge=-90,
            le=90,
        ),
        Column(
            f"roll{suffix}",
            DOUBLE,
            f"Roll at {label} [deg]",
            required=False,
            ge=-180,
            le=180,
        ),
    )


_COMMUNITY_COLUMNS: tuple[Column, ...] = (
    *SCOPE_COLUMNS,
    Column("MT_ID", STRING, "ID of mirror transponder"),
    Column(
        "TravelTime",
        DOUBLE,
        "Corrected travel time [s] (raw range - tat - trigger delay)",
        ge=0,
    ),
    Column("T_transmit", TIMESTAMPTZ, "Transmission time, UTC"),
    Column("X_transmit", DOUBLE, "Transmit position X, ECEF [m]"),
    Column("Y_transmit", DOUBLE, "Transmit position Y, ECEF [m]"),
    Column("Z_transmit", DOUBLE, "Transmit position Z, ECEF [m]"),
    Column("T_receive", TIMESTAMPTZ, "Reception time, UTC"),
    Column("X_receive", DOUBLE, "Receive position X, ECEF [m]"),
    Column("Y_receive", DOUBLE, "Receive position Y, ECEF [m]"),
    Column("Z_receive", DOUBLE, "Receive position Z, ECEF [m]"),
    Column("TDC_ID", STRING, "Transducer ID", required=False),
    Column("aSNR", DOUBLE, "Signal-to-noise ratio [dB]", required=False),
    Column("acc", INT, "Cross-correlation accuracy [%]", required=False, ge=0, le=100),
    Column("dbV", FLOAT, "Decibel-voltage signal strength", required=False),
    Column("quality_flag", STRING, "Shot quality flag", required=False),
    *_community_epoch_columns("0", "transmit"),
    *_community_epoch_columns("1", "receive"),
    *_community_attitude_columns("0", "transmit"),
    *_community_attitude_columns("1", "receive"),
)

GARPOS_L2 = TableDef(
    name="garpos",
    level=2,
    doc="GNSS-A in GARPOS format, antenna positions from Level 1 SV3 GNSS (local ENU).",
    columns=_GARPOS_COLUMNS,
)

GARPOS_L3 = TableDef(
    name="garpos",
    level=3,
    doc="GNSS-A in GARPOS format, antenna positions from Level 2 PPP (local ENU).",
    columns=_GARPOS_COLUMNS,
)

COMMUNITY_STANDARD_L2 = TableDef(
    name="community_standard",
    level=2,
    doc="GNSS-A community standard format, antenna positions from Level 1 SV3 GNSS (ECEF).",
    columns=_COMMUNITY_COLUMNS,
)

COMMUNITY_STANDARD_L3 = TableDef(
    name="community_standard",
    level=3,
    doc="GNSS-A community standard format, antenna positions from Level 2 PPP (ECEF).",
    columns=_COMMUNITY_COLUMNS,
)

LEVEL2_TABLES: tuple[TableDef, ...] = (
    GNSS_ANTENNA_POSITIONS_PPP,
    GARPOS_L2,
    COMMUNITY_STANDARD_L2,
)

LEVEL3_TABLES: tuple[TableDef, ...] = (GARPOS_L3, COMMUNITY_STANDARD_L3)
