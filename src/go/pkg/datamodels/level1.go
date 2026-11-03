package datamodels

import "github.com/apache/iceberg-go"

// Level 1 tables: raw parsed output from full-rate log files, at the native
// sampling rate of the source log. Times are UTC timestamptz; other time
// scales are derived by the access API. GNSSObs is the exception: it is the
// gnsstools observations table, which also stores GPS time.

// TWTT mirrors level1.TRAVEL_TIMES.
var TWTT = TableDef{
	Name:       "travel_times",
	Level:      1,
	TimeColumn: "T_transmit",
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

// INSPVA mirrors level1.INSPVA.
var INSPVA = TableDef{
	Name:       "inspva",
	Level:      1,
	TimeColumn: "time",
	Doc:        "NovAtel INSPVAA position, velocity and attitude (NOV000.bin).",
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
		Column{Name: "ins_status", Type: str, Doc: "INS solution status as logged, e.g. INS_SOLUTION_GOOD, INS_ALIGNING"},
		Column{Name: "time_status", Type: str, Doc: "Log header GPS time status, e.g. FINESTEERING, COARSESTEERING"},
	),
}

// INSSTDEV mirrors level1.INSSTDEV.
var INSSTDEV = TableDef{
	Name:       "insstdev",
	Level:      1,
	TimeColumn: "time",
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
		Column{Name: "time_status", Type: str, Doc: "Log header GPS time status, e.g. FINESTEERING, COARSESTEERING"},
	),
}

// GNSSObs is the gnsstools observations table, defined by gnsstools
// (gitlab.com/earthscope/gnsstools/geodata gnssiceberg): schema, sort order
// and table properties. It has no scope or Log_Source columns; stream_edid
// carries both, as network:station:campaign:filetype:logsource.
//
// Partitioning overrides gnsstools' day(time_gpst), bucket[64](stream_edid)
// with day(time_gpst), identity(stream_edid): one partition per stream per
// GPS day, which suits the few streams of a GNSS-A campaign better than
// hash buckets sized for a large network. The override applies when the
// table is created; an existing table keeps the spec it was created with.
var GNSSObs = TableDef{
	Name:       "gnss_observations",
	Level:      1,
	TimeColumn: "time_gpst",
	Doc: "GNSS observations, one row per (epoch, satellite, signal), from RANGEA " +
		"(NOVATEL, NOV000.bin, QCPIN) and RANGECMP/RANGECMP5 (NOV770), in the " +
		"gnsstools observations schema. stream_edid is " +
		"network:station:campaign:filetype:logsource.",
	External: gnsstoolsObservations,
	PartitionBy: []PartitionKey{
		{Source: "time_gpst", Transform: iceberg.DayTransform{}, Name: "time_gpst_day"},
		{Source: "stream_edid", Transform: iceberg.IdentityTransform{}},
	},
}

// AttitudeAHRS mirrors level1.ATTITUDE_AHRS.
var AttitudeAHRS = TableDef{
	Name:       "attitude_ahrs",
	Level:      1,
	TimeColumn: "time",
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

// SoundSpeed mirrors level1.SOUND_SPEED.
var SoundSpeed = TableDef{
	Name:       "sound_speed",
	Level:      1,
	TimeColumn: "time",
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

// GNSSAntennaPositions mirrors level1.GNSS_ANTENNA_POSITIONS.
var GNSSAntennaPositions = TableDef{
	Name:       "gnss_antenna_positions",
	Level:      1,
	TimeColumn: "time",
	Doc: "Antenna positions from the GNSS block of SV3 events (DFOP00, QCPIN), " +
		"converted to ECEF at ingest. Sigmas are rotated from logged ENU; null " +
		"when Sonardyne reports the -1.0 'not available' sentinel.",
	Columns: level1(append(append([]Column{}, AntennaPositionColumns...),
		Column{Name: "q", Type: int32Type, Doc: "GPS fix quality (SV3GPSQuality)", Required: true, Ge: bound(0), Le: bound(9)},
		Column{Name: "number_of_satellites", Type: int32Type, Doc: "Number of satellites used in the solution", Ge: bound(0)},
		Column{Name: "pdop", Type: float64Type, Doc: "Position dilution of precision", Ge: bound(0)},
		Column{Name: "wrms", Type: float64Type, Doc: "Weighted RMS of position residuals [m]", Ge: bound(0)},
	)...),
}

// GNSSSatellitesInView is Go-only: it has no Python TableDef counterpart.
var GNSSSatellitesInView = TableDef{
	Name:       "gnss_satellites_in_view",
	Level:      1,
	TimeColumn: "time",
	Doc: "Satellites in view from NMEA GSV sentences of the SV3 GNSS receiver " +
		"(GNSS00): one row per satellite per GSV group. GSV carries no time, so " +
		"time is the SV3's packet time of the group's first sentence.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the group's first GSV sentence, UTC", Required: true},
		Column{Name: "talker", Type: str, Doc: "NMEA talker, e.g. GP (GPS), GL (GLONASS), GN (multi-GNSS)", Required: true},
		Column{Name: "sats_in_view", Type: int32Type, Doc: "Satellites in view, as the group reports", Required: true, Ge: bound(0)},
		Column{Name: "prn", Type: int32Type, Doc: "Satellite ID (PRN or NMEA satellite number)", Required: true, Ge: bound(1)},
		Column{Name: "elevation", Type: float64Type, Doc: "Elevation [deg]; null when not reported", Ge: bound(-90), Le: bound(90)},
		Column{Name: "azimuth", Type: float64Type, Doc: "Azimuth from true north [deg]; null when not reported", Ge: bound(0), Le: bound(360)},
		Column{Name: "snr", Type: float64Type, Doc: "Carrier-to-noise density [dB-Hz]; null when not tracked", Ge: bound(0), Le: bound(99)},
	),
}

// GNSSDOP is Go-only: it has no Python TableDef counterpart.
var GNSSDOP = TableDef{
	Name:       "gnss_dop",
	Level:      1,
	TimeColumn: "time",
	Doc: "Fix type, satellites used and dilutions of precision from NMEA GSA " +
		"sentences of the SV3 GNSS receiver (GNSS00). GSA carries no time, so " +
		"time is the SV3's packet time.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the sentence, UTC", Required: true},
		Column{Name: "talker", Type: str, Doc: "NMEA talker, e.g. GP", Required: true},
		Column{Name: "mode", Type: str, Doc: "Selection mode: M manual, A automatic; null when not reported"},
		Column{Name: "fix_type", Type: int32Type, Doc: "Fix type: 1 none, 2 2D, 3 3D", Required: true, Ge: bound(1), Le: bound(3)},
		Column{Name: "satellites_used", Type: str, Doc: "IDs of the satellites used, comma-separated; empty when none", Required: true},
		Column{Name: "number_used", Type: int32Type, Doc: "Number of satellites used", Required: true, Ge: bound(0)},
		Column{Name: "pdop", Type: float64Type, Doc: "Position dilution of precision", Ge: bound(0)},
		Column{Name: "hdop", Type: float64Type, Doc: "Horizontal dilution of precision", Ge: bound(0)},
		Column{Name: "vdop", Type: float64Type, Doc: "Vertical dilution of precision", Ge: bound(0)},
	),
}

// GNSSCourseSpeed is Go-only: it has no Python TableDef counterpart.
var GNSSCourseSpeed = TableDef{
	Name:       "gnss_course_speed",
	Level:      1,
	TimeColumn: "time",
	Doc: "Course and speed over ground from NMEA VTG sentences of the SV3 GNSS " +
		"receiver (GNSS00), in the units logged. VTG carries no time, so time " +
		"is the SV3's packet time.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the sentence, UTC", Required: true},
		Column{Name: "talker", Type: str, Doc: "NMEA talker, e.g. GP", Required: true},
		Column{Name: "course_true", Type: float64Type, Doc: "Course over ground, true [deg]", Ge: bound(0), Le: bound(360)},
		Column{Name: "course_magnetic", Type: float64Type, Doc: "Course over ground, magnetic [deg]", Ge: bound(0), Le: bound(360)},
		Column{Name: "speed_knots", Type: float64Type, Doc: "Speed over ground [knots]", Ge: bound(0)},
		Column{Name: "speed_kmh", Type: float64Type, Doc: "Speed over ground [km/h]", Ge: bound(0)},
		Column{Name: "mode", Type: str, Doc: "Mode indicator: A autonomous, D differential, E estimated, N not valid; null when not reported"},
	),
}

// TransceiverInterrogations is Go-only: it has no Python TableDef counterpart.
var TransceiverInterrogations = TableDef{
	Name:       "transceiver_interrogations",
	Level:      1,
	TimeColumn: "time",
	Doc: "Acoustic interrogations (pings) from the SV3's dialogue with its " +
		"transceiver (TCVR00): one row per trigger, with the targets and " +
		"settings the transceiver acknowledged. time is when the SV3 sent the " +
		"trigger; the transmit time itself is in DFOP00's interrogation events.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the trigger (TRG) request, UTC", Required: true},
		Column{Name: "unit", Type: str, Doc: "Transceiver unit ID from its replies, e.g. 2010"},
		Column{Name: "event", Type: int32Type, Doc: "Event number (EVT)", Ge: bound(0)},
		Column{Name: "targets", Type: str, Doc: "Interrogated transponder addresses, comma-separated, e.g. IR5209,IR5210", Required: true},
		Column{Name: "target_settings", Type: str, Doc: "Per-target settings as acknowledged, comma-separated, e.g. IR5209;RG0;W3600;LG26"},
		Column{Name: "rx_window", Type: int32Type, Doc: "Receive window as logged (RXW)", Ge: bound(0)},
		Column{Name: "tx_channel", Type: str, Doc: "Transmit channel (CI), e.g. CI4805"},
		Column{Name: "delay", Type: float64Type, Doc: "Transmit delay (D) [ms]: the SV3 trigger delay", Ge: bound(0)},
		Column{Name: "npl", Type: int32Type, Doc: "NPL as logged"},
	),
}

// TransceiverRanges is Go-only: it has no Python TableDef counterpart.
var TransceiverRanges = TableDef{
	Name:       "transceiver_ranges",
	Level:      1,
	TimeColumn: "time",
	Doc: "Per-ping range results from the SV3's transceiver (TCVR00 BR): one " +
		"row per interrogated transponder, including pings with no reply " +
		"(detected false). TravelTime is the range DFOP00 logs for the same " +
		"reply; time is the trigger time of the ping it answers.",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the trigger (TRG) the result answers, UTC", Required: true},
		Column{Name: "reply_time", Type: timestamptz, Doc: "SV3 packet time of the result (BR), UTC", Required: true},
		Column{Name: "unit", Type: str, Doc: "Transceiver unit ID, e.g. 2010"},
		Column{Name: "event", Type: int32Type, Doc: "Event number (EVT)", Ge: bound(0)},
		Column{Name: "sequence", Type: int32Type, Doc: "Sequence number (SQ)", Ge: bound(0)},
		Column{Name: "MT_ID", Type: str, Doc: "Transponder address, e.g. IR5209", Required: true},
		Column{Name: "detected", Type: boolType, Doc: "Whether a reply was detected (non-zero range)", Required: true},
		Column{Name: "TravelTime", Type: float64Type, Doc: "Two-way range as logged (R, in microseconds) [s]; null when no reply was detected", Ge: bound(0), Le: bound(600)},
		Column{Name: "acc", Type: int32Type, Doc: "Cross-correlation (XC) [%]", Ge: bound(0), Le: bound(100)},
		Column{Name: "aSNR", Type: float64Type, Doc: "Signal-to-noise ratio (SNR) [dB]", Ge: bound(-100), Le: bound(100)},
		Column{Name: "dbV", Type: float32Type, Doc: "Signal level (DBV) [dBV]"},
		Column{Name: "telemetry", Type: str, Doc: "Telemetry (TEL), e.g. NONE"},
		Column{Name: "doppler", Type: float64Type, Doc: "Doppler (DOP) as logged"},
	),
}

// TransceiverSettings is Go-only: it has no Python TableDef counterpart.
var TransceiverSettings = TableDef{
	Name:       "transceiver_settings",
	Level:      1,
	TimeColumn: "time",
	Doc: "Transceiver configuration and identity as it reported them (TCVR00): " +
		"one row per key of each reply. Log source TCVR_CONFIG holds the CS " +
		"(settings, including TAT) and PORT replies; TCVR_INFO holds US (unit), " +
		"FS (firmware) and DIAG (diagnostics).",
	Columns: level1(
		Column{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the reply, UTC", Required: true},
		Column{Name: "unit", Type: str, Doc: "Transceiver unit ID, e.g. 2010"},
		Column{Name: "command", Type: str, Doc: "Reply type: CS, PORT, US, FS or DIAG", Required: true},
		Column{Name: "key", Type: str, Doc: "Setting name, e.g. TAT", Required: true},
		Column{Name: "value", Type: str, Doc: "Setting value as logged, e.g. 640; empty for a bare flag", Required: true},
	),
}

// Level1Tables lists every Level 1 table definition.
var Level1Tables = []TableDef{
	TWTT,
	INSPVA,
	INSSTDEV,
	GNSSObs,
	AttitudeAHRS,
	SoundSpeed,
	GNSSAntennaPositions,
	GNSSSatellitesInView,
	GNSSDOP,
	GNSSCourseSpeed,
	TransceiverInterrogations,
	TransceiverRanges,
	TransceiverSettings,
}
