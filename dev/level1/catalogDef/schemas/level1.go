package schemas

import "github.com/apache/iceberg-go"

// Level 1 tables: raw parsed output from full-rate log files, at the native
// sampling rate of the source log. Times are UTC timestamptz; other time
// scales are derived by the access API. GNSSObs is the exception: it
// follows the gnsstools observations layout, which also stores GPS time.

// TWTT mirrors schemas.level1.TRAVEL_TIMES.
var TWTT = TableDef{
	Name:  "travel_times",
	Level: 1,
	Doc: "Two-way travel times per transponder from SV3 range and interrogation " +
		"events (DFOP00, QCPIN). Stores the raw range and tat; the " +
		"tat/trigger-delay correction is applied when building Level 2.",
	Columns: level1(
		Column{Name: "MT_ID", Type: str, Doc: "ID of mirror / seafloor transponder", Required: true},
		Column{Name: "T_transmit", Type: timestamptz, Doc: "Transmission (ping) time, UTC", Required: true},
		Column{Name: "T_receive", Type: timestamptz, Doc: "Reception (return) time, UTC", Required: true},
		Column{Name: "TravelTime", Type: float64Type, Required: true, Ge: bound(0), Le: bound(600),
			Doc: "Raw two-way range as logged [s]; authoritative, do not recompute from T_receive - T_transmit"},
		Column{Name: "dbV", Type: float32Type, Doc: "Decibel-voltage signal strength", Required: true},
		Column{Name: "acc", Type: int32Type, Doc: "Cross-correlation accuracy [%]", Required: true, Ge: bound(0), Le: bound(100)},
		Column{Name: "aSNR", Type: float64Type, Doc: "Signal-to-noise ratio [dB]", Ge: bound(-100), Le: bound(100)},
		Column{Name: "tat", Type: float64Type, Doc: "Beacon turn-around time [s]", Ge: bound(0), Le: bound(10)},
	),
}

// INSPVA mirrors schemas.level1.INSPVA.
var INSPVA = TableDef{
	Name:  "inspva",
	Level: 1,
	Doc:   "NovAtel INSPVAA position, velocity and attitude (NOV000.bin).",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "Epoch, UTC (converted from GPS time)", Required: true},
		Column{Name: "lat", Type: float64Type, Doc: "Geodetic latitude [deg]", Required: true, Ge: bound(-90), Le: bound(90)},
		Column{Name: "lon", Type: float64Type, Doc: "Geodetic longitude [deg]", Required: true, Ge: bound(-180), Le: bound(180)},
		Column{Name: "alt", Type: float64Type, Doc: "Geodetic altitude [m]", Required: true},
		Column{Name: "north", Type: float64Type, Doc: "Velocity, local-tangent North [m/s]", Required: true},
		Column{Name: "east", Type: float64Type, Doc: "Velocity, local-tangent East [m/s]", Required: true},
		Column{Name: "up", Type: float64Type, Doc: "Velocity, local-tangent Up [m/s]", Required: true},
		Column{Name: "roll", Type: float64Type, Doc: "Roll angle [deg]", Required: true, Ge: bound(-180), Le: bound(180)},
		Column{Name: "pitch", Type: float64Type, Doc: "Pitch angle [deg]", Required: true, Ge: bound(-90), Le: bound(90)},
		Column{Name: "heading", Type: float64Type, Doc: "Heading angle [deg]", Required: true, Ge: bound(0), Lt: bound(360)},
	),
}

// INSSTDEV mirrors schemas.level1.INSSTDEV.
var INSSTDEV = TableDef{
	Name:  "insstdev",
	Level: 1,
	Doc: "NovAtel INSSTDEVA solution standard deviations (NOV000.bin). The " +
		"cov_* columns keep GNATSS's names but hold the logged attitude std " +
		"devs, not variances.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "Epoch, UTC (converted from GPS time)", Required: true},
		Column{Name: "lat_sig", Type: float64Type, Doc: "Latitude std dev [deg]", Required: true, Ge: bound(0)},
		Column{Name: "lon_sig", Type: float64Type, Doc: "Longitude std dev [deg]", Required: true, Ge: bound(0)},
		Column{Name: "alt_sig", Type: float64Type, Doc: "Altitude std dev [m]", Required: true, Ge: bound(0)},
		Column{Name: "north_sig", Type: float64Type, Doc: "North velocity std dev [m/s]", Required: true, Ge: bound(0)},
		Column{Name: "east_sig", Type: float64Type, Doc: "East velocity std dev [m/s]", Required: true, Ge: bound(0)},
		Column{Name: "up_sig", Type: float64Type, Doc: "Up velocity std dev [m/s]", Required: true, Ge: bound(0)},
		Column{Name: "cov_rr", Type: float64Type, Doc: "Roll std dev [deg], as logged", Required: true, Ge: bound(0)},
		Column{Name: "cov_pp", Type: float64Type, Doc: "Pitch std dev [deg], as logged", Required: true, Ge: bound(0)},
		Column{Name: "cov_hh", Type: float64Type, Doc: "Heading (azimuth) std dev [deg], as logged", Required: true, Ge: bound(0)},
		Column{Name: "ext_sol_stat", Type: str, Doc: "Extended solution status"},
		Column{Name: "time_since_update", Type: str, Doc: "Time since last update"},
	),
}

// GNSSConstellations are the one-character RINEX constellation codes.
var GNSSConstellations = []string{"G", "R", "E", "C", "J", "I", "S"}

// GNSSObs is the gnsstools observations table
// (gitlab.com/earthscope/gnsstools geodata/gnssiceberg, getObservationsTableSchema).
// The first 20 columns keep its names, types, requiredness and order so that
// gnsstools readers work on this table; the Level 1 scope columns are appended
// after them. Sort order matches gnsstools; partitioning is our scope plus
// gnsstools' day(time_gpst), in place of its bucket(stream_edid).
var GNSSObs = TableDef{
	Name:  "gnss_observations",
	Level: 1,
	Doc: "GNSS observations, one row per (epoch, satellite, signal), from RANGEA " +
		"(NOVATEL, NOV000.bin, QCPIN) and RANGECMP/RANGECMP5 (NOV770). Extends " +
		"the gnsstools observations schema with the Level 1 scope columns. " +
		"Log_Source keeps multiple antennas separate.",
	Columns: []Column{
		{Name: "stream_edid", Type: str, Required: true,
			Doc: "Stream EDID identifying the receiver data stream"},
		{Name: "time_utc", Type: timestamptz, Required: true,
			Doc: "Observation epoch on the UTC scale, for differencing against arrival_time and write_time. " +
				"NOT the partition source; filter time_gpst instead. Microsecond precision; " +
				"sub-microsecond remainder in time_sub_us_ps."},
		{Name: "time_gpst", Type: timestamptz, Required: true,
			Doc: "Observation epoch on the GPS scale (GPST). PARTITION SOURCE day(time_gpst); filter windows here. " +
				"A GPST wall clock in a tz-aware column: UTC readers are off by the leap offset, days are GPS days. " +
				"Exact epoch: time_sub_us_ps."},
		{Name: "time_sub_us_ps", Type: int32Type, Required: true, Ge: bound(0), Lt: bound(1e6),
			Doc: "ps - sub-microsecond remainder of the epoch (0-999999). time_utc and time_gpst are " +
				"truncated to the microsecond, so this keeps the epoch exact."},
		{Name: "clock_offset", Type: float64Type, Doc: "s - receiver clock offset"},
		{Name: "constellation", Type: str, Required: true, IsIn: GNSSConstellations,
			Doc: "One-character RINEX constellation code (G, R, E, C, J, I, S)"},
		{Name: "satellite_id", Type: int32Type, Required: true,
			Doc: "RINEX satellite number: PRN, or slot number for GLONASS"},
		{Name: "signal", Type: str, Required: true, Doc: "RINEX 3 observation code, e.g. 1C"},
		{Name: "code", Type: float64Type, Doc: "m - pseudorange"},
		{Name: "phase", Type: float64Type, Doc: "cycles - carrier phase"},
		{Name: "doppler", Type: float64Type, Doc: "Hz"},
		{Name: "snr", Type: float64Type, Doc: "dB-Hz - carrier-to-noise density"},
		{Name: "loss_of_lock", Type: boolType, Doc: "RINEX LLI bit 0. Null when there is no phase observation"},
		{Name: "half_cycle_ambiguity", Type: boolType, Doc: "RINEX LLI bit 1. Null when there is no phase observation"},
		{Name: "boc_tracking", Type: boolType, Doc: "RINEX LLI bit 2. Null when there is no phase observation"},
		{Name: "lock_time", Type: float64Type, Doc: "s - continuous phase lock time"},
		{Name: "epoch_flag", Type: int32Type, Doc: "0 = OK, >1 = event"},
		{Name: "fcn", Type: int32Type, Ge: bound(-7), Le: bound(6),
			Doc: "GLONASS frequency channel number (-7 to +6). Null for other constellations and when " +
				"the channel is not known; 0 is a valid channel"},
		{Name: "write_time", Type: timestamptz, Doc: "When this row was written to Iceberg"},
		{Name: "arrival_time", Type: timestamptz,
			Doc: "When the epoch arrived at the ingest pipeline. arrival_time - time_utc is the ingest latency"},
		Level1Scope[0], Level1Scope[1], Level1Scope[2], Level1Scope[3],
	},
	PartitionBy: append(append([]PartitionKey{}, scopePartitions...),
		PartitionKey{Source: "time_gpst", Transform: iceberg.DayTransform{}, Name: "time_gpst_day"},
	),
	SortBy: []string{"stream_edid", "time_gpst", "constellation", "satellite_id", "signal"},
}

// AttitudeAHRS mirrors schemas.level1.ATTITUDE_AHRS.
var AttitudeAHRS = TableDef{
	Name:  "attitude_ahrs",
	Level: 1,
	Doc: "Attitude from the Sonardyne AHRS block of SV3 events (DFOP00, QCPIN) " +
		"and the full-rate XSENS log (AHRS00). Measurement columns are " +
		"nullable: some events report only magnetic heading. time_start_count " +
		"and time_status come from the SV3 time block and are null for AHRS00.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "Epoch, UTC", Required: true},
		Column{Name: "time_instrument", Type: float64Type, Required: true, Ge: bound(0),
			Doc: "Instrument-relative time [s]; a device clock reading, kept for drift/reset diagnostics"},
		Column{Name: "time_start_count", Type: int32Type, Doc: "Start count for the time", Ge: bound(0)},
		Column{Name: "time_status", Type: str, Doc: "Status of the time data"},
		Column{Name: "acx", Type: float64Type, Doc: "Acceleration, X axis [m/s^2]"},
		Column{Name: "acy", Type: float64Type, Doc: "Acceleration, Y axis [m/s^2]"},
		Column{Name: "acz", Type: float64Type, Doc: "Acceleration, Z axis [m/s^2]"},
		Column{Name: "h", Type: float64Type, Doc: "Heading [deg]", Ge: bound(0), Le: bound(360)},
		Column{Name: "h_mag", Type: float64Type, Doc: "Magnetic heading [deg]", Ge: bound(0), Le: bound(360)},
		Column{Name: "p", Type: float64Type, Doc: "Pitch [deg]", Ge: bound(-90), Le: bound(90)},
		Column{Name: "r", Type: float64Type, Doc: "Roll [deg]", Ge: bound(-180), Le: bound(180)},
	),
}

// SoundSpeed mirrors schemas.level1.SOUND_SPEED.
var SoundSpeed = TableDef{
	Name:  "sound_speed",
	Level: 1,
	Doc: "Sound speed profiles from CTD casts and Sea-Bird profiles. speed must " +
		"be unique per profile (per time), not table-wide.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "Cast/profile time, UTC", Required: true},
		Column{Name: "depth", Type: float64Type, Doc: "Depth [m]", Required: true, Ge: bound(0), Le: bound(10_000)},
		Column{Name: "speed", Type: float64Type, Doc: "Sound speed [m/s]", Required: true, Ge: bound(0), Le: bound(3800)},
	),
}

// AntennaPositionColumns are shared with the Level 2 PPP table.
var AntennaPositionColumns = []Column{
	{Name: "time", Type: timestamptz, Doc: "Epoch, UTC", Required: true},
	{Name: "ant_x", Type: float64Type, Doc: "Antenna ECEF position X [m]", Required: true},
	{Name: "ant_y", Type: float64Type, Doc: "Antenna ECEF position Y [m]", Required: true},
	{Name: "ant_z", Type: float64Type, Doc: "Antenna ECEF position Z [m]", Required: true},
	{Name: "ant_sigx", Type: float64Type, Doc: "Antenna position std dev X, ECEF [m]", Ge: bound(0)},
	{Name: "ant_sigy", Type: float64Type, Doc: "Antenna position std dev Y, ECEF [m]", Ge: bound(0)},
	{Name: "ant_sigz", Type: float64Type, Doc: "Antenna position std dev Z, ECEF [m]", Ge: bound(0)},
}

// Level1Tables lists every Level 1 table definition.
var Level1Tables = []TableDef{
	TWTT,
	INSPVA,
	INSSTDEV,
	GNSSObs,
	AttitudeAHRS,
	SoundSpeed,
}
