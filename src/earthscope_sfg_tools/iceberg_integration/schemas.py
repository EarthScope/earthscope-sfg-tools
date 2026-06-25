"""
PyIceberg schema definitions for seafloor geodesy data types.

Each schema maps to its TileDB counterpart but is enriched with three scope
columns — network, station, campaign — so all stations share a single table
and data can be queried by scope + time range.

The GNSS obs schema aligns with the gnsstools gnssiceberg.ObservationsIcebergWriter
schema (19 fields, schema version 2) so the two systems can write to the same
table.
"""

from pyiceberg.partitioning import PartitionField, PartitionSpec
from pyiceberg.schema import Schema
from pyiceberg.transforms import BucketTransform, DayTransform, IdentityTransform
from pyiceberg.types import (
    BooleanType,
    DoubleType,
    FloatType,
    IntegerType,
    LongType,
    NestedField,
    StringType,
    TimestamptzType,
)

# ---------------------------------------------------------------------------
# Shared scope fields — prepended to every schema.
# Field IDs 1–3 are reserved for scope across all tables.
# ---------------------------------------------------------------------------
_SCOPE_FIELDS = [
    NestedField(1, "network", StringType(), required=True),
    NestedField(2, "station", StringType(), required=True),
    NestedField(3, "campaign", StringType(), required=True),
]

_SCOPE_PARTITION_FIELDS = [
    PartitionField(source_id=2, field_id=1001, transform=IdentityTransform(), name="station"),
    PartitionField(source_id=3, field_id=1002, transform=IdentityTransform(), name="campaign"),
]

# ---------------------------------------------------------------------------
# Kinematic position  (field IDs 10–19)
# ---------------------------------------------------------------------------

KinPositionSchema = Schema(
    *_SCOPE_FIELDS,
    NestedField(10, "time", TimestamptzType(), required=True),
    NestedField(11, "latitude", DoubleType(), required=True),
    NestedField(12, "longitude", DoubleType(), required=True),
    NestedField(13, "height", DoubleType(), required=True),
    NestedField(14, "east", DoubleType(), required=True),
    NestedField(15, "north", DoubleType(), required=True),
    NestedField(16, "up", DoubleType(), required=True),
    NestedField(17, "number_of_satellites", IntegerType(), required=True),
    NestedField(18, "pdop", DoubleType(), required=True),
    NestedField(19, "wrms", DoubleType(), required=True),
)

KinPositionPartitionSpec = PartitionSpec(
    PartitionField(source_id=10, field_id=1000, transform=DayTransform(), name="date"),
    *_SCOPE_PARTITION_FIELDS,
)

# ---------------------------------------------------------------------------
# IMU position  (field IDs 20–39)
# ---------------------------------------------------------------------------

IMUPositionSchema = Schema(
    *_SCOPE_FIELDS,
    NestedField(20, "time", TimestamptzType(), required=True),
    NestedField(21, "azimuth", DoubleType(), required=True),
    NestedField(22, "pitch", DoubleType(), required=True),
    NestedField(23, "roll", DoubleType(), required=True),
    NestedField(24, "latitude", DoubleType(), required=True),
    NestedField(25, "longitude", DoubleType(), required=True),
    NestedField(26, "height", DoubleType(), required=True),
    NestedField(27, "latitude_std", DoubleType(), required=False),
    NestedField(28, "longitude_std", DoubleType(), required=False),
    NestedField(29, "height_std", DoubleType(), required=False),
    NestedField(30, "northVelocity", DoubleType(), required=True),
    NestedField(31, "eastVelocity", DoubleType(), required=True),
    NestedField(32, "upVelocity", DoubleType(), required=True),
    NestedField(33, "northVelocity_std", DoubleType(), required=False),
    NestedField(34, "eastVelocity_std", DoubleType(), required=False),
    NestedField(35, "upVelocity_std", DoubleType(), required=False),
    NestedField(36, "roll_std", DoubleType(), required=False),
    NestedField(37, "pitch_std", DoubleType(), required=False),
    NestedField(38, "azimuth_std", DoubleType(), required=False),
)

IMUPositionPartitionSpec = PartitionSpec(
    PartitionField(source_id=20, field_id=1000, transform=DayTransform(), name="date"),
    *_SCOPE_PARTITION_FIELDS,
)

# ---------------------------------------------------------------------------
# Acoustic ranging  (field IDs 40–49)
# ---------------------------------------------------------------------------

AcousticSchema = Schema(
    *_SCOPE_FIELDS,
    # pingTime matches the AcousticDataFrame pandera field name (GPS float seconds
    # are converted to TimestampTz on write and back to float on read).
    NestedField(40, "pingTime", TimestamptzType(), required=True),
    NestedField(41, "transponderID", StringType(), required=True),
    NestedField(42, "returnTime", TimestamptzType(), required=True),
    NestedField(43, "tt", DoubleType(), required=True),
    NestedField(44, "dbv", FloatType(), required=True),
    NestedField(45, "xc", IntegerType(), required=True),
    NestedField(46, "snr", DoubleType(), required=True),
    NestedField(47, "tat", DoubleType(), required=True),
)

AcousticPartitionSpec = PartitionSpec(
    PartitionField(source_id=40, field_id=1000, transform=DayTransform(), name="date"),
    *_SCOPE_PARTITION_FIELDS,
)

# ---------------------------------------------------------------------------
# Shot data  (field IDs 50–79)
# ---------------------------------------------------------------------------

ShotDataSchema = Schema(
    *_SCOPE_FIELDS,
    NestedField(50, "pingTime", TimestamptzType(), required=True),
    NestedField(51, "transponderID", StringType(), required=True),
    NestedField(52, "returnTime", TimestamptzType(), required=True),
    NestedField(53, "head0", DoubleType(), required=True),
    NestedField(54, "pitch0", DoubleType(), required=True),
    NestedField(55, "roll0", DoubleType(), required=True),
    NestedField(56, "head1", DoubleType(), required=True),
    NestedField(57, "pitch1", DoubleType(), required=True),
    NestedField(58, "roll1", DoubleType(), required=True),
    NestedField(59, "east0", DoubleType(), required=True),
    NestedField(60, "north0", DoubleType(), required=True),
    NestedField(61, "up0", DoubleType(), required=True),
    NestedField(62, "east1", DoubleType(), required=True),
    NestedField(63, "north1", DoubleType(), required=True),
    NestedField(64, "up1", DoubleType(), required=True),
    NestedField(65, "east_std0", DoubleType(), required=False),
    NestedField(66, "north_std0", DoubleType(), required=False),
    NestedField(67, "up_std0", DoubleType(), required=False),
    NestedField(68, "east_std1", DoubleType(), required=False),
    NestedField(69, "north_std1", DoubleType(), required=False),
    NestedField(70, "up_std1", DoubleType(), required=False),
    NestedField(71, "tt", DoubleType(), required=True),
    NestedField(72, "dbv", FloatType(), required=True),
    NestedField(73, "xc", IntegerType(), required=True),
    NestedField(74, "snr", DoubleType(), required=True),
    NestedField(75, "tat", DoubleType(), required=True),
    NestedField(76, "isUpdated", BooleanType(), required=True),
)

ShotDataPartitionSpec = PartitionSpec(
    PartitionField(source_id=50, field_id=1000, transform=DayTransform(), name="date"),
    *_SCOPE_PARTITION_FIELDS,
)

# ---------------------------------------------------------------------------
# GNSS observations
#
# Aligned with gnsstools/geodata/gnssiceberg/observations.go (schema version 2,
# 19 observation fields + 3 scope fields).
#
# Key differences from the TileDB schema:
#   - constellation is a string (e.g. "GPS") not a uint8 sys code
#   - satellite is int32 PRN, signal is a string code (e.g. "C1C")
#   - epoch is TimestampTz at microsecond precision + epoch_pico for sub-us
#   - Richer flag fields: loss_of_lock, half_cycle_ambiguity, boc_tracking
#   - network/station/campaign scope columns added
# ---------------------------------------------------------------------------

GNSSObsSchema = Schema(
    *_SCOPE_FIELDS,
    # Scope within gnsstools: station maps to station_id, campaign to session_id
    NestedField(100, "epoch", TimestamptzType(), required=True),
    NestedField(101, "epoch_pico", IntegerType(), required=True),
    NestedField(102, "clock_offset", DoubleType(), required=False),
    NestedField(103, "constellation", StringType(), required=True),
    NestedField(104, "satellite", IntegerType(), required=True),
    NestedField(105, "signal", StringType(), required=True),
    NestedField(106, "code", DoubleType(), required=False),      # pseudorange (m)
    NestedField(107, "phase", DoubleType(), required=False),     # carrier phase (cycles)
    NestedField(108, "doppler", DoubleType(), required=False),   # Doppler (Hz)
    NestedField(109, "snr", DoubleType(), required=False),       # signal-to-noise (dB-Hz)
    NestedField(110, "loss_of_lock", BooleanType(), required=True),
    NestedField(111, "half_cycle_ambiguity", BooleanType(), required=True),
    NestedField(112, "boc_tracking", BooleanType(), required=True),
    NestedField(113, "lock_time", DoubleType(), required=False),
    NestedField(114, "epoch_flag", IntegerType(), required=False),
    NestedField(115, "fcn", IntegerType(), required=False),      # GLONASS only
    NestedField(116, "write_time", TimestamptzType(), required=False),
)

# Partition: day(epoch) + bucket(32, station) mirrors gnsstools' partition spec.
GNSSObsPartitionSpec = PartitionSpec(
    PartitionField(source_id=100, field_id=1000, transform=DayTransform(), name="epoch_day"),
    PartitionField(source_id=2, field_id=1001, transform=BucketTransform(32), name="station_bucket"),
)
