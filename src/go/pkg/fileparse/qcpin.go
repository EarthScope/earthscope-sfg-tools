// qcpin: parses Sonardyne QC PIN files (*.pin, from the qc/ tarballs):
// one ping's SV3 interrogation event and its range events, as one JSON
// object. The events are the ones DFOP00 logs (see sv3.go), so a QCPIN
// file duplicates a ping already in that campaign's DFOP00 files.
//
// Besides travel times, attitude and antenna positions, each event's
// NOV_RANGE block carries a NovAtel RANGEA log, parsed into GNSSObs.

package fileparse

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// FileTypeQCPIN names the file type in stream EDIDs.
const FileTypeQCPIN = "QCPIN"

// QCPIN's NOV_RANGE blocks are log type LogRANGEA (as in NOV000), the log
// source of their GNSSObs rows. The events' rows use the shared log sources
// edid.LogINTGRPLY, LogAHRS and LogGNSSINS.

// parseQCPINTWTT parses the QCPIN file at path into travel_times rows (stamped with
// stream_edid for scope): each reply paired with the ping, a zero range
// (no reply detected) yielding no row. The caller releases the table.
func parseQCPINTWTT(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	return BuildTWTT(sv3TravelTimes(events), edid.Stream(scope, FileTypeQCPIN, edid.LogINTGRPLY))
}

// parseQCPINAttitudeAHRS parses the QCPIN file at path into attitude_ahrs rows (stamped with
// stream_edid for scope), one per distinct AHRS block. The caller
// releases the table.
func parseQCPINAttitudeAHRS(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	return BuildAttitudeAHRS(sv3Attitudes(events), edid.Stream(scope, FileTypeQCPIN, edid.LogAHRS))
}

// parseQCPINAntennaPositions parses the QCPIN file at path into gnss_antenna_positions
// rows (stamped with stream_edid for scope), one per distinct GNSS block, in ECEF.
// The caller releases the table.
func parseQCPINAntennaPositions(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	return BuildGNSSAntennaPositions(sv3AntennaPositions(events), edid.Stream(scope, FileTypeQCPIN, edid.LogGNSSINS))
}

// parseQCPINGNSSObs parses the RANGEA logs in the QCPIN file's NOV_RANGE blocks into
// GNSSObs rows (stamped with stream_edid for scope), one epoch per distinct log
// time. Logs with no time fix (time status UNKNOWN, or week 0), or whose epoch time fails
// gnssiceberg.ValidateEpoch, are skipped. The caller releases the table.
func parseQCPINGNSSObs(path string, scope edid.Scope) (arrow.Table, error) {
	epochs, err := qcpinEpochs(path)
	if err != nil {
		return nil, err
	}
	return BuildGNSSObs(FileTypeQCPIN, GNSSObsSource{EDID: edid.Stream(scope, FileTypeQCPIN, LogRANGEA), Epochs: epochs})
}

// qcpinEpochs decodes the RANGEA epochs of the QC PIN file at path, skipping
// the logs parseQCPINGNSSObs describes.
func qcpinEpochs(path string) ([]observation.Epoch, error) {
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
		epoch, err := novatelRANGEAEpoch(m)
		switch {
		case errors.Is(err, errNovatelBadEpochTime):
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
	msg, err := newNovatelASCIIScanner(strings.NewReader(strings.TrimSpace(raw) + "\r\n")).Next()
	if err != nil {
		return novatelascii.LongMessage{}, err
	}
	m, ok := msg.(novatelascii.LongMessage)
	if !ok || m.Msg != LogRANGEA {
		return novatelascii.LongMessage{}, errors.New("NOV_RANGE block is not a RANGEA long log")
	}
	return m, nil
}

// readEvents reads the QCPIN file at path.
func readEvents(path string) ([]sv3Event, error) {
	events, bad, err := readSV3QCPIN(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed QCPIN", "file", filepath.Base(path), "events", len(events), "bad_blocks", bad)
	return events, nil
}
