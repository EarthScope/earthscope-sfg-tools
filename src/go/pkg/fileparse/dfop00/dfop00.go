// Package dfop00 parses Sonardyne SV3 DFOP00 logs: one JSON event per line,
// "interrogation" (a ping) or "range" (a transponder reply). Event decoding
// and the rows built from events are shared with QCPIN (see internal/sv3).
package dfop00

import (
	"log/slog"
	"path/filepath"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/sv3"
)

// ParseTWTT parses the events of the DFOP00 file at path into
// travel_times rows:
// each range event pairs with the most recent preceding interrogation, and a
// zero range (no reply detected) yields no row.
func ParseTWTT(path string, src edid.LogSource) ([]datamodels.TWTTRow, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	rs := sv3.TravelTimes(events)
	for i := range rs {
		rs[i].LogSource = datamodels.DFOP00INTGRPLY
	}
	return rowutil.Filter(rs, src, func(r datamodels.TWTTRow) edid.LogSource { return r.LogSource }), nil
}

// ParseINSPVA parses the NOV_INS blocks of the DFOP00 file at path into inspva
// rows: one row per distinct block, the NovAtel
// INS solution at the event.
//
// The events' AHRS and GNSS blocks are not parsed: the GNSS block is this
// INS position, rounded, with the receiver's single-point std devs, and the
// AHRS00 files log the AHRS at full rate.
func ParseINSPVA(path string, src edid.LogSource) ([]datamodels.INSPVARow, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	rs := sv3.INSPVA(events)
	for i := range rs {
		rs[i].LogSource = datamodels.DFOP00INSPVAA
	}
	return rowutil.Filter(rs, src, func(r datamodels.INSPVARow) edid.LogSource { return r.LogSource }), nil
}

// ParseCommunityStandard parses the events of the DFOP00 file at path into
// community_standard rows: one GNSS-A
// shot per ping/reply pair that passes the SV3 pairing rules (see
// sv3.Shots), with the travel time corrected for TAT and trigger delay, the
// antenna position from each event's GNSS block and the attitude from its
// NOV_INS block. Transducer positions are null.
func ParseCommunityStandard(path string, src edid.LogSource) ([]datamodels.CommunityStandardRow, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	rs := sv3.Shots(events)
	for i := range rs {
		rs[i].LogSource = datamodels.DFOP00INTGRPLY
	}
	return rowutil.Filter(rs, src, func(r datamodels.CommunityStandardRow) edid.LogSource { return r.LogSource }), nil
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
