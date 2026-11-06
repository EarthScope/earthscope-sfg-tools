// Package dfop00 parses Sonardyne SV3 DFOP00 logs: one JSON event per line,
// "interrogation" (a ping) or "range" (a transponder reply). Event decoding
// and the rows built from events are shared with QCPIN (see package sv3).
package dfop00

import (
	"log/slog"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/sv3"
)

// FileType names the file type in stream EDIDs.
const FileType = "DFOP00"

// TWTT parses the events of the DFOP00 file at path into
// travel_times rows (see datamodels.ParsedSchema):
// each range event pairs with the most recent preceding interrogation, and a
// zero range (no reply detected) yields no row. The caller releases the
// table.
func TWTT(path string) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildTWTT(sv3.TravelTimes(events), datamodels.LogINTGRPLY)
}

// INSPVA parses the NOV_INS blocks of the DFOP00 file at path into inspva
// rows (see datamodels.ParsedSchema): one row per distinct block, the NovAtel
// INS solution at the event. The caller releases the table.
//
// The events' AHRS and GNSS blocks are not parsed: the GNSS block is this
// INS position, rounded, with the receiver's single-point std devs, and the
// AHRS00 files log the AHRS at full rate.
func INSPVA(path string) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildINSPVA(sv3.INSPVA(events), datamodels.LogINSPVAA)
}

// parseEvents reads the events of the DFOP00 file at path, logging how many
// lines were not valid JSON.
func parseEvents(path string) ([]sv3.Event, error) {
	events, bad, err := sv3.ReadJSONL(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed DFOP00", "file", filepath.Base(path), "events", len(events), "bad_lines", bad)
	return events, nil
}
