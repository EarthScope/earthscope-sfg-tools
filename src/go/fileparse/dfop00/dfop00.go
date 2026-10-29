// Package dfop00 parses Sonardyne SV3 DFOP00 logs: one JSON event per line,
// "interrogation" (a ping) or "range" (a transponder reply). Event decoding
// and the rows built from events are shared with QCPIN (see package sv3).
package dfop00

import (
	"log/slog"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/sv3"
)

const (
	// FileType names the file type in stream EDIDs.
	FileType = "DFOP00"
	// LogType is the one log type DFOP00 files carry (SV3 JSON events), and
	// the Log_Source value of their rows.
	LogType = "DFOP00"
)

// TWTT parses the logType (LogType) events of the DFOP00 file at path into
// travel_times rows (see datamodels.ParsedSchema):
// each range event pairs with the most recent preceding interrogation, and a
// zero range (no reply detected) yields no row. The caller releases the
// table.
func TWTT(path, logType string) (arrow.Table, error) {
	events, err := parseEvents(path, logType)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildTWTT(sv3.TravelTimes(events), LogType)
}

// AttitudeAHRS parses the logType (LogType) events of the DFOP00 file at path
// into attitude_ahrs rows (see datamodels.ParsedSchema): one row per distinct
// AHRS block (consecutive events usually repeat the same block). The caller
// releases the table.
func AttitudeAHRS(path, logType string) (arrow.Table, error) {
	events, err := parseEvents(path, logType)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildAttitudeAHRS(sv3.Attitudes(events), LogType)
}

// AntennaPositions parses the logType (LogType) events of the DFOP00 file at
// path into gnss_antenna_positions rows (see datamodels.ParsedSchema): one row
// per distinct GNSS block (keyed by its fix time), converted to ECEF. The
// caller releases the table.
func AntennaPositions(path, logType string) (arrow.Table, error) {
	events, err := parseEvents(path, logType)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildGNSSAntennaPositions(sv3.AntennaPositions(events), LogType)
}

// parseEvents reads the events of the DFOP00 file at path, logging how many
// lines were not valid JSON. logType must be LogType.
func parseEvents(path, logType string) ([]sv3.Event, error) {
	if err := logtype.Check(FileType, logType, LogType); err != nil {
		return nil, err
	}
	events, bad, err := sv3.ReadJSONL(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed DFOP00", "file", filepath.Base(path), "events", len(events), "bad_lines", bad)
	return events, nil
}
