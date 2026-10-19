// Package novatel770 parses NOV770 logs: NovAtel binary RANGECMP (ID 140)
// and RANGECMP5 (ID 2537) from a dual-antenna receiver.
package novatel770

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

	"github.com/apache/arrow-go/v18/arrow"
	novatelbinary "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_binary"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/novatel"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/tablebuild"
)

// FileType names the file type in errors and stream EDIDs.
const FileType = "NOV770"

// Tables are the tables NOV770 files produce.
var Tables = []schemas.TableDef{schemas.GNSSObs}

// logNames are the Log_Source names of the NovAtel range logs, by message ID.
var logNames = map[uint16]string{140: "RANGECMPB", 2537: "RANGECMP5B"}

// LogSource is the Log_Source value for rows from message msgID on one
// antenna (measurement source): the log name, with NovAtel's _<antenna>
// suffix for antennas after the primary, e.g. RANGECMP5B, RANGECMP5B_1.
func LogSource(msgID uint16, antenna uint8) string {
	name := logNames[msgID]
	if antenna > 0 {
		name += fmt.Sprintf("_%d", antenna)
	}
	return name
}

// timeStatusUnknown is the NovAtel time status UNKNOWN: no time fix, week 0.
const timeStatusUnknown uint8 = 20

// ParseFile parses the NOV770 file at path into def (GNSSObs) for scope. Each
// (log, antenna) gets its own Log_Source; see LogSource. Messages whose time status is
// UNKNOWN are skipped. The caller releases the table.
func ParseFile(path string, def schemas.TableDef, scope schemas.Scope) (arrow.Table, error) {
	if def.Name != schemas.GNSSObs.Name {
		return nil, tablebuild.Unsupported(FileType, def, Tables...)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bySource := map[string][]observation.Epoch{}
	failed, skipped := 0, 0
	r := bufio.NewReader(f)
	for {
		msg, err := novatelbinary.DeserializeMessage(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failed++
			if errors.Is(err, bufio.ErrBufferFull) {
				r.Reset(f)
			}
			continue
		}
		if msg.MessageID != 140 && msg.MessageID != 2537 {
			continue
		}
		if msg.TimeStatus == timeStatusUnknown {
			skipped++
			continue
		}
		epoch, err := decode(&msg)
		if err != nil {
			failed++
			slog.Debug("bad NovAtel message", "file", filepath.Base(path), "id", msg.MessageID, "error", err)
			continue
		}
		src := LogSource(msg.MessageID, msg.MeasurementSource())
		bySource[src] = append(bySource[src], epoch)
	}

	names := slices.Sorted(maps.Keys(bySource))
	sources := make([]novatel.GNSSObsSource, len(names))
	for i, name := range names {
		sources[i] = novatel.GNSSObsSource{LogSource: name, Epochs: bySource[name]}
		slog.Debug("parsed NOV770", "file", filepath.Base(path), "log_source", name, "epochs", len(bySource[name]))
	}
	slog.Debug("parsed NOV770", "file", filepath.Base(path), "failed", failed, "unknown_time", skipped)
	return novatel.GNSSObsTable(scope, FileType, sources...)
}

func decode(msg *novatelbinary.Message) (observation.Epoch, error) {
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
