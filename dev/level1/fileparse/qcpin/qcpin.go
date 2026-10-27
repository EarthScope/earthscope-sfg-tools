// Package qcpin parses Sonardyne QC PIN files (*.pin, from the qc/ tarballs):
// one ping's SV3 interrogation event and its range events, as one JSON
// object. The events are the ones DFOP00 logs (see package sv3), so a QCPIN
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

	"github.com/apache/arrow-go/v18/arrow"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/novatel"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/sv3"
)

// FileType names the file type in stream EDIDs.
const FileType = "QCPIN"

// The log types QCPIN files carry. Each is also the Log_Source value of its
// rows.
const (
	LogEvents = "QCPIN"  // SV3 events: travel_times, attitude_ahrs, gnss_antenna_positions
	LogRANGEA = "RANGEA" // NOV_RANGE blocks: GNSSObs
)

// TWTT parses the QCPIN file at path into travel_times rows (see
// datamodels.ParsedSchema): each reply paired with the ping, a zero range
// (no reply detected) yielding no row. logType must be LogEvents. The
// caller releases the table.
func TWTT(path, logType string) (arrow.Table, error) {
	events, err := readEvents(path, logType, LogEvents)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildTWTT(sv3.TravelTimes(events), LogEvents)
}

// AttitudeAHRS parses the QCPIN file at path into attitude_ahrs rows (see
// datamodels.ParsedSchema), one per distinct AHRS block. logType must be
// LogEvents. The caller releases the table.
func AttitudeAHRS(path, logType string) (arrow.Table, error) {
	events, err := readEvents(path, logType, LogEvents)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildAttitudeAHRS(sv3.Attitudes(events), LogEvents)
}

// AntennaPositions parses the QCPIN file at path into gnss_antenna_positions
// rows (see datamodels.ParsedSchema), one per distinct GNSS block, in ECEF.
// logType must be LogEvents. The caller releases the table.
func AntennaPositions(path, logType string) (arrow.Table, error) {
	events, err := readEvents(path, logType, LogEvents)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildGNSSAntennaPositions(sv3.AntennaPositions(events), LogEvents)
}

// GNSSObs parses the RANGEA logs in the QCPIN file's NOV_RANGE blocks into
// GNSSObs rows (see datamodels.ParsedSchema), one epoch per distinct log
// time. Logs whose time status is UNKNOWN, or whose epoch time fails
// gnssiceberg.ValidateEpoch, are skipped. logType must be LogRANGEA. The
// caller releases the table.
func GNSSObs(path, logType string) (arrow.Table, error) {
	events, err := readEvents(path, logType, LogRANGEA)
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
		if m.TimeStatus == "UNKNOWN" {
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
	return datamodels.BuildGNSSObs(FileType, datamodels.GNSSObsSource{LogSource: LogRANGEA, Epochs: epochs})
}

// rangeLog decodes raw, a NOV_RANGE block's log, as a RANGEA long message.
func rangeLog(raw string) (novatelascii.LongMessage, error) {
	msg, err := novatel.NewASCIIScanner(strings.NewReader(strings.TrimSpace(raw) + "\r\n")).Next()
	if err != nil {
		return novatelascii.LongMessage{}, err
	}
	m, ok := msg.(novatelascii.LongMessage)
	if !ok || m.Msg != LogRANGEA {
		return novatelascii.LongMessage{}, errors.New("NOV_RANGE block is not a RANGEA long log")
	}
	return m, nil
}

// readEvents reads the QCPIN file at path after checking logType is valid,
// the log type the calling table is built from.
func readEvents(path, logType, valid string) ([]sv3.Event, error) {
	if err := logtype.Check(FileType, logType, valid); err != nil {
		return nil, err
	}
	events, bad, err := sv3.ReadQCPIN(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed QCPIN", "file", filepath.Base(path), "events", len(events), "bad_blocks", bad)
	return events, nil
}
