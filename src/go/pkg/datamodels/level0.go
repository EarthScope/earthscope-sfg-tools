package datamodels

import "github.com/apache/iceberg-go"

// Level 0 tables: raw parsed output from full-rate log files, at the native
// sampling rate of the source log. Times are UTC timestamptz; other time
// scales are derived by the access API. GNSSObs is the exception: it is the
// gnsstools observations table, which also stores GPS time.

// TWTT mirrors level0.TRAVEL_TIMES.
var TWTT = TableDef{
	Name:       "travel_times",
	Level:      0,
	TimeColumn: "T_transmit",
	Doc: "Two-way travel times per transponder from SV3 range and interrogation " +
		"events (DFOP00, QCPIN). Stores the raw range and tat; the " +
		"tat/trigger-delay correction is applied when building Level 1.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "MT_ID", Type: str, Doc: "ID of mirror / seafloor transponder", Required: true},
		field{Name: "T_transmit", Type: timestamptz, Doc: "Transmission (ping) time, UTC", Required: true},
		field{Name: "T_receive", Type: timestamptz, Doc: "Reception (return) time, UTC", Required: true},
		field{Name: "TravelTime", Type: float64Type, Required: true,
			Doc: "Raw two-way range as logged [s]; authoritative, do not recompute from T_receive - T_transmit"},
		field{Name: "dbV", Type: float32Type, Doc: "Decibel-voltage signal strength", Required: true},
		field{Name: "acc", Type: int32Type, Doc: "Cross-correlation accuracy [%]", Required: true},
		field{Name: "aSNR", Type: float64Type, Doc: "Signal-to-noise ratio [dB]"},
		field{Name: "tat", Type: float64Type, Doc: "Beacon turn-around time [s]"},
	),
	Checks: map[string]Check{
		"TravelTime": {Ge: bound(0), Le: bound(600)},
		"acc":        {Ge: bound(0), Le: bound(100)},
		"aSNR":       {Ge: bound(-100), Le: bound(100)},
		"tat":        {Ge: bound(0), Le: bound(10)},
	},
}

// INSPVA mirrors level0.INSPVA.
var INSPVA = TableDef{
	Name:       "inspva",
	Level:      0,
	TimeColumn: "time",
	Doc: "NovAtel INSPVAA position, velocity and attitude (NOV000.bin), and the " +
		"NOV_INS block of SV3 events (DFOP00) at each event.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "Epoch, UTC (converted from GPS time)", Required: true},
		field{Name: "lat", Type: float64Type, Doc: "Geodetic latitude [deg]", Required: true},
		field{Name: "lon", Type: float64Type, Doc: "Geodetic longitude [deg]", Required: true},
		field{Name: "alt", Type: float64Type, Doc: "Geodetic altitude [m]", Required: true},
		field{Name: "north", Type: float64Type, Doc: "Velocity, local-tangent North [m/s]", Required: true},
		field{Name: "east", Type: float64Type, Doc: "Velocity, local-tangent East [m/s]", Required: true},
		field{Name: "up", Type: float64Type, Doc: "Velocity, local-tangent Up [m/s]", Required: true},
		field{Name: "roll", Type: float64Type, Doc: "Roll angle [deg]", Required: true},
		field{Name: "pitch", Type: float64Type, Doc: "Pitch angle [deg]", Required: true},
		field{Name: "heading", Type: float64Type, Doc: "Heading angle [deg]", Required: true},
		field{Name: "ins_status", Type: str, Doc: "INS solution status as logged, e.g. INS_SOLUTION_GOOD, INS_ALIGNING"},
		field{Name: "time_status", Type: str, Doc: "Log header GPS time status, e.g. FINESTEERING, COARSESTEERING"},
	),
	Checks: map[string]Check{
		"lat":     {Ge: bound(-90), Le: bound(90)},
		"lon":     {Ge: bound(-180), Le: bound(180)},
		"roll":    {Ge: bound(-180), Le: bound(180)},
		"pitch":   {Ge: bound(-90), Le: bound(90)},
		"heading": {Ge: bound(0), Lt: bound(360)},
	},
}

// INSSTDEV mirrors level0.INSSTDEV.
var INSSTDEV = TableDef{
	Name:       "insstdev",
	Level:      0,
	TimeColumn: "time",
	Doc: "NovAtel INSSTDEVA solution standard deviations (NOV000.bin). The " +
		"cov_* columns keep GNATSS's names but hold the logged attitude std " +
		"devs, not variances.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "Epoch, UTC (converted from GPS time)", Required: true},
		field{Name: "lat_sig", Type: float64Type, Doc: "Latitude std dev [deg]", Required: true},
		field{Name: "lon_sig", Type: float64Type, Doc: "Longitude std dev [deg]", Required: true},
		field{Name: "alt_sig", Type: float64Type, Doc: "Altitude std dev [m]", Required: true},
		field{Name: "north_sig", Type: float64Type, Doc: "North velocity std dev [m/s]", Required: true},
		field{Name: "east_sig", Type: float64Type, Doc: "East velocity std dev [m/s]", Required: true},
		field{Name: "up_sig", Type: float64Type, Doc: "Up velocity std dev [m/s]", Required: true},
		field{Name: "cov_rr", Type: float64Type, Doc: "Roll std dev [deg], as logged", Required: true},
		field{Name: "cov_pp", Type: float64Type, Doc: "Pitch std dev [deg], as logged", Required: true},
		field{Name: "cov_hh", Type: float64Type, Doc: "Heading (azimuth) std dev [deg], as logged", Required: true},
		field{Name: "ext_sol_stat", Type: str, Doc: "Extended solution status"},
		field{Name: "time_since_update", Type: str, Doc: "Time since last update"},
		field{Name: "time_status", Type: str, Doc: "Log header GPS time status, e.g. FINESTEERING, COARSESTEERING"},
	),
	Checks: map[string]Check{
		"lat_sig":   {Ge: bound(0)},
		"lon_sig":   {Ge: bound(0)},
		"alt_sig":   {Ge: bound(0)},
		"north_sig": {Ge: bound(0)},
		"east_sig":  {Ge: bound(0)},
		"up_sig":    {Ge: bound(0)},
		"cov_rr":    {Ge: bound(0)},
		"cov_pp":    {Ge: bound(0)},
		"cov_hh":    {Ge: bound(0)},
	},
}

// GNSSObs is the gnsstools observations table, defined by gnsstools
// (gitlab.com/earthscope/gnsstools/geodata gnssiceberg): schema, sort order
// and table properties. Like every table, its only identity column is
// stream_edid, network.station.campaign.filetype.logsource.
//
// Partitioning overrides gnsstools' day(time_gpst), bucket[64](stream_edid)
// with day(time_gpst), identity(stream_edid): one partition per stream per
// GPS day, which suits the few streams of a GNSS-A campaign better than
// hash buckets sized for a large network. The override applies when the
// table is created; an existing table keeps the spec it was created with.
var GNSSObs = TableDef{
	Name:       "gnss_observations",
	Level:      0,
	TimeColumn: "time_gpst",
	Doc: "GNSS observations, one row per (epoch, satellite, signal), from RANGEA " +
		"(NOVATEL, NOV000.bin, QCPIN) and RANGECMP/RANGECMP5 (NOV770), in the " +
		"gnsstools observations schema. stream_edid is " +
		"network.station.campaign.filetype.logsource.",
	External: gnsstoolsObservations,
	PartitionBy: []PartitionKey{
		{Source: "time_gpst", Transform: iceberg.DayTransform{}, Name: "time_gpst_day"},
		{Source: "stream_edid", Transform: iceberg.IdentityTransform{}},
	},
}

// AttitudeAHRS mirrors level0.ATTITUDE_AHRS.
var AttitudeAHRS = TableDef{
	Name:       "attitude_ahrs",
	Level:      0,
	TimeColumn: "time",
	Doc: "Attitude from the Sonardyne AHRS block of SV3 events (QCPIN) " +
		"and the full-rate XSENS log (AHRS00). Measurement columns are " +
		"nullable: some events report only magnetic heading. time_start_count " +
		"and time_status come from the SV3 time block and are null for AHRS00.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "Epoch, UTC", Required: true},
		field{Name: "time_instrument", Type: float64Type, Required: true,
			Doc: "Instrument-relative time [s]; a device clock reading, kept for drift/reset diagnostics"},
		field{Name: "time_start_count", Type: int32Type, Doc: "Start count for the time"},
		field{Name: "time_status", Type: str, Doc: "Status of the time data"},
		field{Name: "acx", Type: float64Type, Doc: "Acceleration, X axis [m/s^2]"},
		field{Name: "acy", Type: float64Type, Doc: "Acceleration, Y axis [m/s^2]"},
		field{Name: "acz", Type: float64Type, Doc: "Acceleration, Z axis [m/s^2]"},
		field{Name: "h", Type: float64Type, Doc: "Heading [deg]"},
		field{Name: "h_mag", Type: float64Type, Doc: "Magnetic heading [deg]"},
		field{Name: "p", Type: float64Type, Doc: "Pitch [deg]"},
		field{Name: "r", Type: float64Type, Doc: "Roll [deg]"},
	),
	Checks: map[string]Check{
		"time_instrument":  {Ge: bound(0)},
		"time_start_count": {Ge: bound(0)},
		"h":                {Ge: bound(0), Le: bound(360)},
		"h_mag":            {Ge: bound(0), Le: bound(360)},
		"p":                {Ge: bound(-90), Le: bound(90)},
		"r":                {Ge: bound(-180), Le: bound(180)},
	},
}

// SoundSpeed mirrors level0.SOUND_SPEED.
var SoundSpeed = TableDef{
	Name:       "sound_speed",
	Level:      0,
	TimeColumn: "time",
	Doc: "Sound speed profiles from CTD casts and Sea-Bird profiles. speed must " +
		"be unique per profile (per time), not table-wide.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "Cast/profile time, UTC", Required: true},
		field{Name: "depth", Type: float64Type, Doc: "Depth [m]", Required: true},
		field{Name: "speed", Type: float64Type, Doc: "Sound speed [m/s]", Required: true},
	),
	Checks: map[string]Check{
		"depth": {Ge: bound(0), Le: bound(10_000)},
		"speed": {Ge: bound(0), Le: bound(3800)},
	},
}

// AntennaPositionColumns are shared with the Level 1 PPP table.
var AntennaPositionColumns = []field{
	{Name: "time", Type: timestamptz, Doc: "Epoch, UTC", Required: true},
	{Name: "ant_x", Type: float64Type, Doc: "Antenna ECEF position X [m]", Required: true},
	{Name: "ant_y", Type: float64Type, Doc: "Antenna ECEF position Y [m]", Required: true},
	{Name: "ant_z", Type: float64Type, Doc: "Antenna ECEF position Z [m]", Required: true},
	{Name: "ant_sigx", Type: float64Type, Doc: "Antenna position std dev X, ECEF [m]"},
	{Name: "ant_sigy", Type: float64Type, Doc: "Antenna position std dev Y, ECEF [m]"},
	{Name: "ant_sigz", Type: float64Type, Doc: "Antenna position std dev Z, ECEF [m]"},
}

// AntennaPositionChecks are the checks of AntennaPositionColumns.
var AntennaPositionChecks = map[string]Check{
	"ant_sigx": {Ge: bound(0)},
	"ant_sigy": {Ge: bound(0)},
	"ant_sigz": {Ge: bound(0)},
}

// GNSSAntennaPositions mirrors level0.GNSS_ANTENNA_POSITIONS.
var GNSSAntennaPositions = TableDef{
	Name:       "gnss_antenna_positions",
	Level:      0,
	TimeColumn: "time",
	Doc: "Antenna positions from the GNSS block of SV3 events (QCPIN), " +
		"converted to ECEF at ingest. Sigmas are rotated from logged ENU; null " +
		"when Sonardyne reports the -1.0 'not available' sentinel.",
	Schema: newSchema(append(append([]field{
		{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
	}, AntennaPositionColumns...),
		field{Name: "q", Type: int32Type, Doc: "GPS fix quality (SV3GPSQuality)", Required: true},
		field{Name: "number_of_satellites", Type: int32Type, Doc: "Number of satellites used in the solution"},
		field{Name: "pdop", Type: float64Type, Doc: "Position dilution of precision"},
		field{Name: "wrms", Type: float64Type, Doc: "Weighted RMS of position residuals [m]"},
	)...),
	Checks: checks(AntennaPositionChecks, map[string]Check{
		"q":                    {Ge: bound(0), Le: bound(9)},
		"number_of_satellites": {Ge: bound(0)},
		"pdop":                 {Ge: bound(0)},
		"wrms":                 {Ge: bound(0)},
	}),
}

// GNSSSatellitesInView is Go-only: it has no Python TableDef counterpart.
var GNSSSatellitesInView = TableDef{
	Name:       "gnss_satellites_in_view",
	Level:      0,
	TimeColumn: "time",
	Doc: "Satellites in view from NMEA GSV sentences of the SV3 GNSS receiver " +
		"(GNSS00): one row per satellite per GSV group. GSV carries no time, so " +
		"time is the SV3's packet time of the group's first sentence.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the group's first GSV sentence, UTC", Required: true},
		field{Name: "talker", Type: str, Doc: "NMEA talker, e.g. GP (GPS), GL (GLONASS), GN (multi-GNSS)", Required: true},
		field{Name: "sats_in_view", Type: int32Type, Doc: "Satellites in view, as the group reports", Required: true},
		field{Name: "prn", Type: int32Type, Doc: "Satellite ID (PRN or NMEA satellite number)", Required: true},
		field{Name: "elevation", Type: float64Type, Doc: "Elevation [deg]; null when not reported"},
		field{Name: "azimuth", Type: float64Type, Doc: "Azimuth from true north [deg]; null when not reported"},
		field{Name: "snr", Type: float64Type, Doc: "Carrier-to-noise density [dB-Hz]; null when not tracked"},
	),
	Checks: map[string]Check{
		"sats_in_view": {Ge: bound(0)},
		"prn":          {Ge: bound(1)},
		"elevation":    {Ge: bound(-90), Le: bound(90)},
		"azimuth":      {Ge: bound(0), Le: bound(360)},
		"snr":          {Ge: bound(0), Le: bound(99)},
	},
}

// GNSSDOP is Go-only: it has no Python TableDef counterpart.
var GNSSDOP = TableDef{
	Name:       "gnss_dop",
	Level:      0,
	TimeColumn: "time",
	Doc: "Fix type, satellites used and dilutions of precision from NMEA GSA " +
		"sentences of the SV3 GNSS receiver (GNSS00). GSA carries no time, so " +
		"time is the SV3's packet time.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the sentence, UTC", Required: true},
		field{Name: "talker", Type: str, Doc: "NMEA talker, e.g. GP", Required: true},
		field{Name: "mode", Type: str, Doc: "Selection mode: M manual, A automatic; null when not reported"},
		field{Name: "fix_type", Type: int32Type, Doc: "Fix type: 1 none, 2 2D, 3 3D", Required: true},
		field{Name: "satellites_used", Type: str, Doc: "IDs of the satellites used, comma-separated; empty when none", Required: true},
		field{Name: "number_used", Type: int32Type, Doc: "Number of satellites used", Required: true},
		field{Name: "pdop", Type: float64Type, Doc: "Position dilution of precision"},
		field{Name: "hdop", Type: float64Type, Doc: "Horizontal dilution of precision"},
		field{Name: "vdop", Type: float64Type, Doc: "Vertical dilution of precision"},
	),
	Checks: map[string]Check{
		"fix_type":    {Ge: bound(1), Le: bound(3)},
		"number_used": {Ge: bound(0)},
		"pdop":        {Ge: bound(0)},
		"hdop":        {Ge: bound(0)},
		"vdop":        {Ge: bound(0)},
	},
}

// GNSSCourseSpeed is Go-only: it has no Python TableDef counterpart.
var GNSSCourseSpeed = TableDef{
	Name:       "gnss_course_speed",
	Level:      0,
	TimeColumn: "time",
	Doc: "Course and speed over ground from NMEA VTG sentences of the SV3 GNSS " +
		"receiver (GNSS00), in the units logged. VTG carries no time, so time " +
		"is the SV3's packet time.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the sentence, UTC", Required: true},
		field{Name: "talker", Type: str, Doc: "NMEA talker, e.g. GP", Required: true},
		field{Name: "course_true", Type: float64Type, Doc: "Course over ground, true [deg]"},
		field{Name: "course_magnetic", Type: float64Type, Doc: "Course over ground, magnetic [deg]"},
		field{Name: "speed_knots", Type: float64Type, Doc: "Speed over ground [knots]"},
		field{Name: "speed_kmh", Type: float64Type, Doc: "Speed over ground [km/h]"},
		field{Name: "mode", Type: str, Doc: "Mode indicator: A autonomous, D differential, E estimated, N not valid; null when not reported"},
	),
	Checks: map[string]Check{
		"course_true":     {Ge: bound(0), Le: bound(360)},
		"course_magnetic": {Ge: bound(0), Le: bound(360)},
		"speed_knots":     {Ge: bound(0)},
		"speed_kmh":       {Ge: bound(0)},
	},
}

// TransceiverInterrogations is Go-only: it has no Python TableDef counterpart.
var TransceiverInterrogations = TableDef{
	Name:       "transceiver_interrogations",
	Level:      0,
	TimeColumn: "time",
	Doc: "Acoustic interrogations (pings) from the SV3's dialogue with its " +
		"transceiver (TCVR00): one row per trigger, with the targets and " +
		"settings the transceiver acknowledged. time is when the SV3 sent the " +
		"trigger; the transmit time itself is in DFOP00's interrogation events.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the trigger (TRG) request, UTC", Required: true},
		field{Name: "unit", Type: str, Doc: "Transceiver unit ID from its replies, e.g. 2010"},
		field{Name: "event", Type: int32Type, Doc: "Event number (EVT)"},
		field{Name: "targets", Type: str, Doc: "Interrogated transponder addresses, comma-separated, e.g. IR5209,IR5210", Required: true},
		field{Name: "target_settings", Type: str, Doc: "Per-target settings as acknowledged, comma-separated, e.g. IR5209;RG0;W3600;LG26"},
		field{Name: "rx_window", Type: int32Type, Doc: "Receive window as logged (RXW)"},
		field{Name: "tx_channel", Type: str, Doc: "Transmit channel (CI), e.g. CI4805"},
		field{Name: "delay", Type: float64Type, Doc: "Transmit delay (D) [ms]: the SV3 trigger delay"},
		field{Name: "npl", Type: int32Type, Doc: "NPL as logged"},
	),
	Checks: map[string]Check{
		"event":     {Ge: bound(0)},
		"rx_window": {Ge: bound(0)},
		"delay":     {Ge: bound(0)},
	},
}

// TransceiverRanges is Go-only: it has no Python TableDef counterpart.
var TransceiverRanges = TableDef{
	Name:       "transceiver_ranges",
	Level:      0,
	TimeColumn: "time",
	Doc: "Per-ping range results from the SV3's transceiver (TCVR00 BR): one " +
		"row per interrogated transponder, including pings with no reply " +
		"(detected false). TravelTime is the range DFOP00 logs for the same " +
		"reply; time is the trigger time of the ping it answers.",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the trigger (TRG) the result answers, UTC", Required: true},
		field{Name: "reply_time", Type: timestamptz, Doc: "SV3 packet time of the result (BR), UTC", Required: true},
		field{Name: "unit", Type: str, Doc: "Transceiver unit ID, e.g. 2010"},
		field{Name: "event", Type: int32Type, Doc: "Event number (EVT)"},
		field{Name: "sequence", Type: int32Type, Doc: "Sequence number (SQ)"},
		field{Name: "MT_ID", Type: str, Doc: "Transponder address, e.g. IR5209", Required: true},
		field{Name: "detected", Type: boolType, Doc: "Whether a reply was detected (non-zero range)", Required: true},
		field{Name: "TravelTime", Type: float64Type, Doc: "Two-way range as logged (R, in microseconds) [s]; null when no reply was detected"},
		field{Name: "acc", Type: int32Type, Doc: "Cross-correlation (XC) [%]"},
		field{Name: "aSNR", Type: float64Type, Doc: "Signal-to-noise ratio (SNR) [dB]"},
		field{Name: "dbV", Type: float32Type, Doc: "Signal level (DBV) [dBV]"},
		field{Name: "telemetry", Type: str, Doc: "Telemetry (TEL), e.g. NONE"},
		field{Name: "doppler", Type: float64Type, Doc: "Doppler (DOP) as logged"},
	),
	Checks: map[string]Check{
		"event":      {Ge: bound(0)},
		"sequence":   {Ge: bound(0)},
		"TravelTime": {Ge: bound(0), Le: bound(600)},
		"acc":        {Ge: bound(0), Le: bound(100)},
		"aSNR":       {Ge: bound(-100), Le: bound(100)},
	},
}

// TransceiverSettings is Go-only: it has no Python TableDef counterpart.
var TransceiverSettings = TableDef{
	Name:       "transceiver_settings",
	Level:      0,
	TimeColumn: "time",
	Doc: "Transceiver configuration and identity as it reported them (TCVR00): " +
		"one row per key of each reply. Log source TCVR_CONFIG holds the CS " +
		"(settings, including TAT) and PORT replies; TCVR_INFO holds US (unit), " +
		"FS (firmware) and DIAG (diagnostics).",
	Schema: newSchema(
		field{Name: StreamEDIDName, Type: str, Required: true, Doc: "Stream EDID, network.station.campaign.file_type.log_source"},
		field{Name: "time", Type: timestamptz, Doc: "SV3 packet time of the reply, UTC", Required: true},
		field{Name: "unit", Type: str, Doc: "Transceiver unit ID, e.g. 2010"},
		field{Name: "command", Type: str, Doc: "Reply type: CS, PORT, US, FS or DIAG", Required: true},
		field{Name: "key", Type: str, Doc: "Setting name, e.g. TAT", Required: true},
		field{Name: "value", Type: str, Doc: "Setting value as logged, e.g. 640; empty for a bare flag", Required: true},
	),
}

// Level0Tables lists every Level 0 table definition.
var Level0Tables = []TableDef{
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
