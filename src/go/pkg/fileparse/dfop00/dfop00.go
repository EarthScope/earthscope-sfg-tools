// Package dfop00 parses Sonardyne SV3 DFOP00 logs: one JSON event per line,
// "interrogation" (a ping) or "range" (a transponder reply). Event decoding
// and the rows built from events are shared with QCPIN (see package sv3).
package dfop00

import (
	"log/slog"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/sv3"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/rows"
)

// FileType names the file type in stream EDIDs.
const FileType = "DFOP00"

// TWTT parses the events of the DFOP00 file at path into
// travel_times rows (stamped with stream_edid for scope):
// each range event pairs with the most recent preceding interrogation, and a
// zero range (no reply detected) yields no row. The caller releases the
// table.
func TWTT(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return rows.BuildTWTT(sv3.TravelTimes(events), edid.Stream(scope, FileType, edid.LogINTGRPLY))
}

// INSPVA parses the NOV_INS blocks of the DFOP00 file at path into inspva
// rows (stamped with stream_edid for scope): one row per distinct block, the NovAtel
// INS solution at the event. The caller releases the table.
//
// The events' AHRS and GNSS blocks are not parsed: the GNSS block is this
// INS position, rounded, with the receiver's single-point std devs, and the
// AHRS00 files log the AHRS at full rate.
func INSPVA(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return rows.BuildINSPVA(sv3.INSPVA(events), edid.Stream(scope, FileType, edid.LogINSPVAA))
}

// CommunityStandard parses the events of the DFOP00 file at path into
// community_standard rows (stamped with stream_edid for scope): one GNSS-A
// shot per ping/reply pair that passes the SV3 pairing rules (see
// sv3.Shots), with the travel time corrected for TAT and trigger delay, the
// antenna position from each event's GNSS block and the attitude from its
// NOV_INS block. Transducer positions are null. The caller releases the
// table.
func CommunityStandard(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return rows.BuildCommunityStandard(sv3.Shots(events), edid.Stream(scope, FileType, edid.LogINTGRPLY))
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
