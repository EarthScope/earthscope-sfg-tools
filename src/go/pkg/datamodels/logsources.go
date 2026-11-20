package datamodels

import "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"

// Log sources: the (file type, log source) pairs the Level 0 parsers stamp
// into stream_edid, declared once here and listed on the table each feeds
// (TableDef.Sources). A var is named <FileType><Name>, dropping "-" and
// keeping "_". datamodels sits below fileparse, so file types and names are
// literals; fileparse's parsers stamp the same strings (pkg/fileparse
// has the full matrix, log_sources.md). A log source whose name carries
// NovAtel's _<antenna> suffix (edid.AntennaLogName) is declared for the
// primary and the second antenna.
var (
	// NOV000: NovAtel ASCII logs in GPSA packets.
	NOV000INSPVAA   = edid.LogSource{FileType: "NOV000", Name: edid.LogINSPVAA}
	NOV000INSSTDEVA = edid.LogSource{FileType: "NOV000", Name: "INSSTDEVA"}
	NOV000RANGEA    = edid.LogSource{FileType: "NOV000", Name: "RANGEA"}

	// NVA: bare NovAtel ASCII logs, the same logs as NOV000.
	NVAINSPVAA   = edid.LogSource{FileType: "NVA", Name: edid.LogINSPVAA}
	NVAINSSTDEVA = edid.LogSource{FileType: "NVA", Name: "INSSTDEVA"}
	NVARANGEA    = edid.LogSource{FileType: "NVA", Name: "RANGEA"}

	// NOV770: NovAtel binary logs; _1 is the second antenna.
	NOV770RANGECMPB    = edid.LogSource{FileType: "NOV770", Name: "RANGECMPB"}
	NOV770RANGECMPB_1  = edid.LogSource{FileType: "NOV770", Name: edid.AntennaLogName("RANGECMPB", 1)}
	NOV770RANGECMP5B   = edid.LogSource{FileType: "NOV770", Name: "RANGECMP5B"}
	NOV770RANGECMP5B_1 = edid.LogSource{FileType: "NOV770", Name: edid.AntennaLogName("RANGECMP5B", 1)}

	// DFOP00: Sonardyne SV3 events.
	DFOP00INTGRPLY = edid.LogSource{FileType: "DFOP00", Name: edid.LogINTGRPLY}
	DFOP00INSPVAA  = edid.LogSource{FileType: "DFOP00", Name: edid.LogINSPVAA}

	// QCPIN: SV3 QC file.
	QCPININTGRPLY = edid.LogSource{FileType: "QCPIN", Name: edid.LogINTGRPLY}
	QCPINAHRS     = edid.LogSource{FileType: "QCPIN", Name: edid.LogAHRS}
	QCPINGNSSINS  = edid.LogSource{FileType: "QCPIN", Name: edid.LogGNSSINS}
	QCPINRANGEA   = edid.LogSource{FileType: "QCPIN", Name: "RANGEA"}

	// AHRS00: Xsens attitude in GPSA packets.
	AHRS00AHRS = edid.LogSource{FileType: "AHRS00", Name: edid.LogAHRS}

	// GNSS00: SV3 GNSS receiver NMEA in GPSA packets.
	GNSS00GNSSPSEUDO = edid.LogSource{FileType: "GNSS00", Name: edid.LogGNSSPSEUDO}
	GNSS00GSA        = edid.LogSource{FileType: "GNSS00", Name: "GSA"}
	GNSS00GSV        = edid.LogSource{FileType: "GNSS00", Name: "GSV"}
	GNSS00VTG        = edid.LogSource{FileType: "GNSS00", Name: "VTG"}

	// CTD and SEABIRD: sound speed casts.
	CTDCTD         = edid.LogSource{FileType: "CTD", Name: "CTD"}
	SEABIRDSEABIRD = edid.LogSource{FileType: "SEABIRD", Name: "SEABIRD"}

	// TCVR00: the vars use readable names (TCVR00RangeResult, ...), an
	// exception to the <FileType><Name> rule.
	// TCVR00: acoustic transceiver dialogue in GPSA packets.
	TCVR00Interrogate = edid.LogSource{FileType: "TCVR00", Name: "INTERROGATE"}
	TCVR00RangeResult = edid.LogSource{FileType: "TCVR00", Name: "RANGE_RESULT"}
	TCVR00Config      = edid.LogSource{FileType: "TCVR00", Name: "TCVR_CONFIG"}
	TCVR00Info        = edid.LogSource{FileType: "TCVR00", Name: "TCVR_INFO"}
)
