// Package qcpin parses Sonardyne QC PIN files (*.pin, from the qc/ tarballs):
// one ping's SV3 interrogation event and its range events, as one JSON
// object. The events are the ones DFOP00 logs (see internal/sv3), so a QCPIN
// file duplicates a ping already in that campaign's DFOP00 files.
//
// Besides travel times, attitude and antenna positions, each event's
// NOV_RANGE block carries a NovAtel RANGEA log, parsed into GNSSObs.
package qcpin

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strings"

	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/sv3"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel"
)

// QCPIN's NOV_RANGE blocks are log type RANGEA (as in NOV000), the log
// source of their GNSSObs rows. The events' rows use the shared log sources
// edid.LogINTGRPLY, LogAHRS and LogGNSSINS.

// ParseTWTT parses the QCPIN file at path into travel_times rows: each reply paired with the ping, a zero range
// (no reply detected) yielding no row.
func ParseTWTT(path string, src edid.LogSource) ([]datamodels.TWTTRow, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	rs := sv3.TravelTimes(events)
	for i := range rs {
		rs[i].LogSource = datamodels.QCPININTGRPLY
	}
	return rowutil.Filter(rs, src, func(r datamodels.TWTTRow) edid.LogSource { return r.LogSource }), nil
}

// ParseAttitude parses the QCPIN file at path into attitude_ahrs rows, one per
// distinct AHRS block.
func ParseAttitude(path string, src edid.LogSource) ([]datamodels.AttitudeAHRSRow, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	rs := sv3.Attitudes(events)
	for i := range rs {
		rs[i].LogSource = datamodels.QCPINAHRS
	}
	return rowutil.Filter(rs, src, func(r datamodels.AttitudeAHRSRow) edid.LogSource { return r.LogSource }), nil
}

// ParseAntennaPositions parses the QCPIN file at path into gnss_antenna_positions
// rows, one per distinct GNSS block, in ECEF.
func ParseAntennaPositions(path string, src edid.LogSource) ([]datamodels.GNSSAntennaPositionRow, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	rs := sv3.AntennaPositions(events)
	for i := range rs {
		rs[i].LogSource = datamodels.QCPINGNSSINS
	}
	return rowutil.Filter(rs, src, func(r datamodels.GNSSAntennaPositionRow) edid.LogSource { return r.LogSource }), nil
}

// ParseGNSSObs parses the RANGEA logs in the QCPIN file's NOV_RANGE blocks into
// GNSSObs rows, one epoch per distinct log
// time. Logs with no time fix (time status UNKNOWN, or week 0), or whose epoch time fails
// gnssiceberg.ValidateEpoch, are skipped.
func ParseGNSSObs(path string, src edid.LogSource) ([]datamodels.GNSSObsRow, error) {
	epochs, err := Epochs(path)
	if err != nil {
		return nil, err
	}
	return rowutil.Filter(novatel.ObsRows(datamodels.QCPINRANGEA, epochs), src, func(r datamodels.GNSSObsRow) edid.LogSource { return r.LogSource }), nil
}

// Epochs decodes the RANGEA epochs of the QC PIN file at path, skipping
// the logs ParseGNSSObs describes.
func Epochs(path string) ([]observation.Epoch, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	var (
		epochs                   []observation.Epoch
		seen                     = map[string]bool{}
		failed, skipped, badTime int
	)
	for i := range events {
		raw, err := events[i].RangeLog()
		if err != nil {
			failed++
			continue
		}
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		m, err := rangeLog(raw)
		if err != nil {
			failed++
			continue
		}
		if m.TimeStatus == "UNKNOWN" || m.Week == 0 {
			skipped++
			continue
		}
		epoch, err := novatel.RANGEAEpoch(m)
		switch {
		case errors.Is(err, novatel.ErrBadEpochTime):
			badTime++
		case err != nil:
			failed++
		default:
			epochs = append(epochs, epoch)
		}
	}
	slog.Debug("parsed QCPIN RANGEA", "file", filepath.Base(path), "epochs", len(epochs),
		"failed", failed, "unknown_time", skipped, "bad_time", badTime)
	return epochs, nil
}

// rangeLog decodes raw, a NOV_RANGE block's log, as a RANGEA long message.
func rangeLog(raw string) (novatelascii.LongMessage, error) {
	msg, err := novatel.NewASCIIScanner(strings.NewReader(strings.TrimSpace(raw) + "\r\n")).Next()
	if err != nil {
		return novatelascii.LongMessage{}, err
	}
	m, ok := msg.(novatelascii.LongMessage)
	if !ok || m.Msg != novatel.LogRANGEA {
		return novatelascii.LongMessage{}, errors.New("NOV_RANGE block is not a RANGEA long log")
	}
	return m, nil
}

// readEvents reads the QCPIN file at path.
func readEvents(path string) ([]sv3.Event, error) {
	events, bad, err := sv3.ReadQCPIN(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed QCPIN", "file", filepath.Base(path), "events", len(events), "bad_blocks", bad)
	return events, nil
}
