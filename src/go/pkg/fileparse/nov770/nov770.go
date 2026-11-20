// Package nov770 parses NOV770 logs: NovAtel binary RANGECMP (ID 140)
// and RANGECMP5 (ID 2537) from a dual-antenna receiver.
package nov770

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"

	novatelbinary "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_binary"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel"
)

// The NovAtel binary log types NOV770 files carry; both produce GNSSObs.
const (
	logRANGECMPB  = "RANGECMPB"  // message ID 140
	logRANGECMP5B = "RANGECMP5B" // message ID 2537
)

// logTypes are the log types of the NovAtel message IDs GNSSObs reads.
var logTypes = map[uint16]string{140: logRANGECMPB, 2537: logRANGECMP5B}

// timeStatusUnknown is the NovAtel time status UNKNOWN: no time fix, week 0.
const timeStatusUnknown uint8 = 20

// readerSize bounds the largest message DeserializeMessage can peek whole;
// it is well above any RANGECMP/RANGECMP5 log.
const readerSize = 64 << 10

// ParseGNSSObs parses the RANGECMPB and RANGECMP5B logs of the NOV770 file at path
// into GNSSObs rows, in one pass. Each log type
// and antenna gets its own log source (edid.AntennaLogName), and so its own
// stream EDID: RANGECMP5B, RANGECMP5B_1. Messages whose time status is
// UNKNOWN or whose week is 0 (no time fix), or whose epoch time fails
// gnssiceberg.ValidateEpoch, are skipped. src selects one log source, or
// edid.AllLogSources.
func ParseGNSSObs(path string, src edid.LogSource) ([]datamodels.GNSSObsRow, error) {
	bySource, err := Epochs(path)
	if err != nil {
		return nil, err
	}
	names := slices.Sorted(maps.Keys(bySource))
	var rows []datamodels.GNSSObsRow
	for _, name := range names {
		// name is computed (log type and antenna): it equals one of
		// datamodels.NOV770RANGECMPB, NOV770RANGECMPB_1, NOV770RANGECMP5B, NOV770RANGECMP5B_1.
		rows = append(rows, novatel.ObsRows(edid.LogSource{FileType: datamodels.NOV770RANGECMPB.FileType, Name: name}, bySource[name])...)
	}
	return rowutil.Filter(rows, src, func(r datamodels.GNSSObsRow) edid.LogSource { return r.LogSource }), nil
}

// Epochs decodes the RANGECMPB and RANGECMP5B epochs of the NOV770 file
// at path, by log source (edid.AntennaLogName: log type and antenna), skipping the
// messages ParseGNSSObs describes.
func Epochs(path string) (map[string][]observation.Epoch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bySource := map[string][]observation.Epoch{}
	failed, skipped, badTime := 0, 0, 0
	r := bufio.NewReaderSize(f, readerSize)
	for {
		msg, err := novatelbinary.DeserializeMessage(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failed++
			// A header claiming more bytes than the buffer holds (a corrupt
			// length) leaves the reader where it was; step past its sync
			// byte and resync on the next one.
			if errors.Is(err, bufio.ErrBufferFull) {
				r.Discard(1)
			}
			continue
		}
		logType, ok := logTypes[msg.MessageID]
		if !ok {
			continue
		}
		if msg.TimeStatus == timeStatusUnknown || msg.Week == 0 {
			skipped++
			continue
		}
		epoch, err := decodeNOV770(&msg)
		if err != nil {
			failed++
			slog.Debug("bad NovAtel message", "file", filepath.Base(path), "id", msg.MessageID, "error", err)
			continue
		}
		if err := gnssiceberg.ValidateEpoch(epoch.Time); err != nil {
			badTime++
			slog.Debug("bad NovAtel epoch time", "file", filepath.Base(path), "id", msg.MessageID, "error", err)
			continue
		}
		src := edid.AntennaLogName(logType, msg.MeasurementSource())
		bySource[src] = append(bySource[src], epoch)
	}

	for name, epochs := range bySource {
		slog.Debug("parsed NOV770", "file", filepath.Base(path), "log_source", name, "epochs", len(epochs))
	}
	slog.Debug("parsed NOV770", "file", filepath.Base(path), "failed", failed, "unknown_time", skipped, "bad_time", badTime)
	return bySource, nil
}

func decodeNOV770(msg *novatelbinary.Message) (observation.Epoch, error) {
	var (
		epoch observation.Epoch
		err   error
	)
	switch msg.MessageID {
	case 140:
		m := msg.DeserializeMessage140()
		epoch, err = m.SerializeGNSSEpoch(msg.Time())
	case 2537:
		m, derr := msg.DeserializeMessage2537()
		if derr != nil {
			return epoch, derr
		}
		epoch, err = m.SerializeGNSSEpoch(msg.Time())
	}
	if err == nil && len(epoch.Satellites) == 0 {
		err = fmt.Errorf("message %d with no satellites", msg.MessageID)
	}
	return epoch, err
}
