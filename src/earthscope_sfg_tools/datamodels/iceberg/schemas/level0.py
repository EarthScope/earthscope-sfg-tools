"""
Level 1 tables: raw parsed output from full-rate log files.

Every Level 1 table carries the scope columns plus ``Log_Source`` and is
stored at the native sampling rate of its source log. Times are UTC
``timestamptz``; other time scales are derived by the access API. The one
exception is ``gnss_observations.epoch``, which holds GPS time to stay
compatible with gnsstools readers.
"""

from pyiceberg.transforms import DayTransform

from ._base import (
    BOOLEAN,
    DOUBLE,
    FLOAT,
    INT,
    LEVEL1_SCOPE,
    LOG_SOURCE,
    STREAM_EDID,
    STRING,
    TIMESTAMPTZ,
    Column,
    PartitionKey,
    TableDef,
)

TRAVEL_TIMES = TableDef(
    name="travel_times",
    level=1,
    doc=(
        "Two-way travel times per transponder from SV3 range and interrogation "
        "events (DFOP00, QCPIN). Stores the raw range and tat; the "
        "tat/trigger-delay correction is applied when building Level 2."
    ),
    columns=(
        *LEVEL1_SCOPE,
        Column("MT_ID", STRING, "ID of mirror / seafloor transponder"),
        Column("T_transmit", TIMESTAMPTZ, "Transmission (ping) time, UTC"),
        Column("T_receive", TIMESTAMPTZ, "Reception (return) time, UTC"),
        Column(
            "TravelTime",
            DOUBLE,
            "Raw two-way range as logged [s]; authoritative, do not recompute "
            "from T_receive - T_transmit",
            ge=0,
            le=600,
        ),
        Column("dbV", FLOAT, "Decibel-voltage signal strength"),
        Column("acc", INT, "Cross-correlation accuracy [%]", ge=0, le=100),
        Column(
            "aSNR",
            DOUBLE,
            "Signal-to-noise ratio [dB]",
            required=False,
            ge=-100,
            le=100,
        ),
        Column(
            "tat", DOUBLE, "Beacon turn-around time [s]", required=False, ge=0, le=10
        ),
        STREAM_EDID,
    ),
)

INSPVA = TableDef(
    name="inspva",
    level=1,
    doc="NovAtel INSPVAA position, velocity and attitude (NOV000.bin).",
    columns=(
        *LEVEL1_SCOPE,
        Column("time", TIMESTAMPTZ, "Epoch, UTC (converted from GPS time)"),
        Column("lat", DOUBLE, "Geodetic latitude [deg]", ge=-90, le=90),
        Column("lon", DOUBLE, "Geodetic longitude [deg]", ge=-180, le=180),
        Column("alt", DOUBLE, "Geodetic altitude [m]"),
        Column("north", DOUBLE, "Velocity, local-tangent North [m/s]"),
        Column("east", DOUBLE, "Velocity, local-tangent East [m/s]"),
        Column("up", DOUBLE, "Velocity, local-tangent Up [m/s]"),
        Column("roll", DOUBLE, "Roll angle [deg]", ge=-180, le=180),
        Column("pitch", DOUBLE, "Pitch angle [deg]", ge=-90, le=90),
        Column("heading", DOUBLE, "Heading angle [deg]", ge=0, lt=360),
        Column(
            "ins_status",
            STRING,
            "INS solution status as logged, e.g. INS_SOLUTION_GOOD, INS_ALIGNING",
            required=False,
        ),
        Column(
            "time_status",
            STRING,
            "Log header GPS time status, e.g. FINESTEERING, COARSESTEERING",
            required=False,
        ),
        STREAM_EDID,
    ),
)

INSSTDEV = TableDef(
    name="insstdev",
    level=1,
    doc=(
        "NovAtel INSSTDEVA solution standard deviations (NOV000.bin). The "
        "cov_* columns keep GNATSS's names but hold the logged attitude std "
        "devs, not variances."
    ),
    columns=(
        *LEVEL1_SCOPE,
        Column("time", TIMESTAMPTZ, "Epoch, UTC (converted from GPS time)"),
        Column("lat_sig", DOUBLE, "Latitude std dev [deg]", ge=0),
        Column("lon_sig", DOUBLE, "Longitude std dev [deg]", ge=0),
        Column("alt_sig", DOUBLE, "Altitude std dev [m]", ge=0),
        Column("north_sig", DOUBLE, "North velocity std dev [m/s]", ge=0),
        Column("east_sig", DOUBLE, "East velocity std dev [m/s]", ge=0),
        Column("up_sig", DOUBLE, "Up velocity std dev [m/s]", ge=0),
        Column("cov_rr", DOUBLE, "Roll std dev [deg], as logged", ge=0),
        Column("cov_pp", DOUBLE, "Pitch std dev [deg], as logged", ge=0),
        Column("cov_hh", DOUBLE, "Heading (azimuth) std dev [deg], as logged", ge=0),
        Column("ext_sol_stat", STRING, "Extended solution status", required=False),
        Column("time_since_update", STRING, "Time since last update", required=False),
        Column(
            "time_status",
            STRING,
            "Log header GPS time status, e.g. FINESTEERING, COARSESTEERING",
            required=False,
        ),
        STREAM_EDID,
    ),
)

# gnsstools ``observations`` table (geodata/gnssiceberg, v0.105.0): the first
# 20 columns must keep these names, types, requiredness and order so that
# gnsstools' Iceberg readers (``ReadObservationRows``) work on this table.
# Our scope columns are appended after them.
GNSSICEBERG_CONSTELLATIONS: tuple[str, ...] = (
    "GPS",
    "GLO",
    "GAL",
    "BDS",
    "QZSS",
    "IRNSS",
    "SBAS",
)

GNSS_OBSERVATIONS = TableDef(
    name="gnss_observations",
    level=1,
    doc=(
        "GNSS observations, one row per (epoch, satellite, signal), from RANGEA "
        "(NOVATEL, NOV000.bin, QCPIN) and RANGECMP/RANGECMP5 (NOV770). Extends "
        "the gnsstools observations schema with scope columns; station_id holds "
        "the Station. Log_Source keeps multiple antennas separate."
    ),
    columns=(
        Column("station_id", STRING, "Station identifier (the Station scope)"),
        Column(
            "session_id",
            STRING,
            "Ingest session; the source file name, for provenance and reloads",
        ),
        Column(
            "epoch",
            TIMESTAMPTZ,
            "Observation epoch in GPS time (no leap-second shift, as gnsstools "
            "stores it), truncated to the microsecond",
        ),
        Column(
            "epoch_pico",
            INT,
            "Sub-nanosecond remainder of the epoch [ps] (gnsstools EpochPico); "
            "nanoseconds below the microsecond are not stored",
            ge=0,
            le=999,
        ),
        Column("clock_offset", DOUBLE, "Receiver clock offset [s]", required=False),
        Column(
            "constellation",
            STRING,
            "gnsstools system name, e.g. GPS, GLO, GAL, BDS",
            isin=GNSSICEBERG_CONSTELLATIONS,
        ),
        Column("satellite", INT, "Satellite number: PRN, or slot for GLONASS"),
        Column("signal", STRING, "RINEX 3 observation code, e.g. 1C"),
        Column(
            "code", DOUBLE, "Pseudorange [m]; null when not tracked", required=False
        ),
        Column(
            "phase",
            DOUBLE,
            "Carrier phase [cycles]; null when not tracked",
            required=False,
        ),
        Column(
            "doppler", DOUBLE, "Doppler [Hz]; null when not tracked", required=False
        ),
        Column(
            "snr",
            DOUBLE,
            "Carrier-to-noise density [dB-Hz]; null when not tracked",
            required=False,
        ),
        Column("loss_of_lock", BOOLEAN, "RINEX LLI bit 0"),
        Column("half_cycle_ambiguity", BOOLEAN, "RINEX LLI bit 1"),
        Column("boc_tracking", BOOLEAN, "RINEX LLI bit 2"),
        Column("lock_time", DOUBLE, "Continuous phase lock time [s]", required=False),
        Column("epoch_flag", INT, "0 = OK, >1 = event; null when 0", required=False),
        Column(
            "fcn",
            INT,
            "GLONASS frequency channel number; null for other constellations "
            "(0 is a valid channel)",
            required=False,
            ge=-7,
            le=6,
        ),
        Column(
            "write_time",
            TIMESTAMPTZ,
            "When this row was written to Iceberg",
            required=False,
        ),
        Column(
            "publish_time",
            TIMESTAMPTZ,
            "When the epoch arrived at the ingest pipeline",
            required=False,
        ),
        Column("Network", STRING, "Network identifier"),
        Column("Campaign", STRING, "Campaign identifier"),
        LOG_SOURCE,
    ),
    scope_names=("Network", "station_id", "Campaign"),
    partition_by=(
        PartitionKey("Network"),
        PartitionKey("station_id"),
        PartitionKey("Campaign"),
        PartitionKey("epoch", DayTransform(), "epoch_day"),
    ),
    sort_by=(
        "station_id",
        "session_id",
        "epoch",
        "constellation",
        "satellite",
        "signal",
    ),
)

ATTITUDE_AHRS = TableDef(
    name="attitude_ahrs",
    level=1,
    doc=(
        "Attitude from the Sonardyne AHRS block of SV3 events (DFOP00, QCPIN) "
        "and the full-rate XSENS log (AHRS00). Measurement columns are "
        "nullable: some events report only magnetic heading. time_start_count "
        "and time_status come from the SV3 time block and are null for AHRS00."
    ),
    columns=(
        *LEVEL1_SCOPE,
        Column("time", TIMESTAMPTZ, "Epoch, UTC"),
        Column(
            "time_instrument",
            DOUBLE,
            "Instrument-relative time [s]; a device clock reading, kept for "
            "drift/reset diagnostics",
            ge=0,
        ),
        Column(
            "time_start_count",
            INT,
            "Start count for the time",
            required=False,
            ge=0,
        ),
        Column("time_status", STRING, "Status of the time data", required=False),
        Column("acx", DOUBLE, "Acceleration, X axis [m/s^2]", required=False),
        Column("acy", DOUBLE, "Acceleration, Y axis [m/s^2]", required=False),
        Column("acz", DOUBLE, "Acceleration, Z axis [m/s^2]", required=False),
        Column("h", DOUBLE, "Heading [deg]", required=False, ge=0, le=360),
        Column("h_mag", DOUBLE, "Magnetic heading [deg]", required=False, ge=0, le=360),
        Column("p", DOUBLE, "Pitch [deg]", required=False, ge=-90, le=90),
        Column("r", DOUBLE, "Roll [deg]", required=False, ge=-180, le=180),
        STREAM_EDID,
    ),
)

SOUND_SPEED = TableDef(
    name="sound_speed",
    level=1,
    doc=(
        "Sound speed profiles from CTD casts and Sea-Bird profiles. speed must "
        "be unique per profile (per time), not table-wide."
    ),
    columns=(
        *LEVEL1_SCOPE,
        Column("time", TIMESTAMPTZ, "Cast/profile time, UTC"),
        Column("depth", DOUBLE, "Depth [m]", ge=0, le=10_000),
        Column("speed", DOUBLE, "Sound speed [m/s]", ge=0, le=3800),
        STREAM_EDID,
    ),
)

# Shared with the Level 2 PPP table; only nullability of the solution-quality
# columns differs between the two.
ANTENNA_POSITION_COLUMNS: tuple[Column, ...] = (
    Column("time", TIMESTAMPTZ, "Epoch, UTC"),
    Column("ant_x", DOUBLE, "Antenna ECEF position X [m]"),
    Column("ant_y", DOUBLE, "Antenna ECEF position Y [m]"),
    Column("ant_z", DOUBLE, "Antenna ECEF position Z [m]"),
    Column(
        "ant_sigx", DOUBLE, "Antenna position std dev X, ECEF [m]", required=False, ge=0
    ),
    Column(
        "ant_sigy", DOUBLE, "Antenna position std dev Y, ECEF [m]", required=False, ge=0
    ),
    Column(
        "ant_sigz", DOUBLE, "Antenna position std dev Z, ECEF [m]", required=False, ge=0
    ),
)

GNSS_ANTENNA_POSITIONS = TableDef(
    name="gnss_antenna_positions",
    level=1,
    doc=(
        "Antenna positions from the GNSS block of SV3 events (DFOP00, QCPIN), "
        "converted to ECEF at ingest. Sigmas are rotated from logged ENU; null "
        "when Sonardyne reports the -1.0 'not available' sentinel."
    ),
    columns=(
        *LEVEL1_SCOPE,
        *ANTENNA_POSITION_COLUMNS,
        Column("q", INT, "GPS fix quality (SV3GPSQuality)", ge=0, le=9),
        Column(
            "number_of_satellites",
            INT,
            "Number of satellites used in the solution",
            required=False,
            ge=0,
        ),
        Column("pdop", DOUBLE, "Position dilution of precision", required=False, ge=0),
        Column(
            "wrms",
            DOUBLE,
            "Weighted RMS of position residuals [m]",
            required=False,
            ge=0,
        ),
    ),
)

LEVEL1_TABLES: tuple[TableDef, ...] = (
    TRAVEL_TIMES,
    INSPVA,
    INSSTDEV,
    GNSS_OBSERVATIONS,
    ATTITUDE_AHRS,
    SOUND_SPEED,
    GNSS_ANTENNA_POSITIONS,
)
