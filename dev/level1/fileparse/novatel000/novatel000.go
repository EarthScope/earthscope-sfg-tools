// Package novatel000 parses Sonardyne NOV000.bin logs: NovAtel ASCII logs
// (RANGEA, INSPVAA, INSSTDEVA) carried in GPSA binary packets.
package novatel000

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/novatel"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/tablebuild"
)

const (
	// FileType names the file type in errors and stream EDIDs.
	FileType = "NOV000"
	// LogSource is the Log_Source value for INSPVA and INSSTDEV rows.
	LogSource = FileType
	// GNSSLogSource is the Log_Source value for GNSSObs rows (RANGEA logs).
	GNSSLogSource = "RANGE"
)

// Tables are the tables NOV000.bin files produce.
var Tables = []schemas.TableDef{schemas.GNSSObs, schemas.INSPVA, schemas.INSSTDEV}

// logFor is the NovAtel ASCII log each table is built from.
var logFor = map[string]string{
	schemas.GNSSObs.Name:  "RANGEA",
	schemas.INSPVA.Name:   "INSPVAA",
	schemas.INSSTDEV.Name: "INSSTDEVA",
}

// ParseFile parses the NOV000.bin file at path into def (one of Tables) for
// scope. Logs whose header time status is UNKNOWN (no time fix) are skipped.
// The caller releases the table.
func ParseFile(path string, def schemas.TableDef, scope schemas.Scope) (arrow.Table, error) {
	want, ok := logFor[def.Name]
	if !ok {
		return nil, tablebuild.Unsupported(FileType, def, Tables...)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		epochs   []observation.Epoch
		inspva   []novatel.INSPVA
		insstdev []novatel.INSSTDEV
		failed   int
		skipped  int
	)
	s := novatel.NewGPSAASCIIScanner(f)
	for {
		msg, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failed++
			continue
		}
		m, ok := msg.(novatelascii.LongMessage)
		if !ok || m.Msg != want {
			continue
		}
		if m.TimeStatus == "UNKNOWN" {
			skipped++
			continue
		}
		if err := decode(m, &epochs, &inspva, &insstdev); err != nil {
			failed++
			slog.Debug("bad NovAtel log", "file", filepath.Base(path), "log", m.Msg, "error", err)
		}
	}
	slog.Debug("parsed NOV000", "file", filepath.Base(path), "log", want,
		"epochs", len(epochs), "inspva", len(inspva), "insstdev", len(insstdev),
		"failed", failed, "unknown_time", skipped)

	switch def.Name {
	case schemas.GNSSObs.Name:
		return novatel.GNSSObsTable(scope, FileType, novatel.GNSSObsSource{LogSource: GNSSLogSource, Epochs: epochs})
	case schemas.INSPVA.Name:
		return novatel.INSPVATable(inspva, scope, LogSource)
	default:
		return novatel.INSSTDEVTable(insstdev, scope, LogSource)
	}
}

func decode(m novatelascii.LongMessage, epochs *[]observation.Epoch, inspva *[]novatel.INSPVA, insstdev *[]novatel.INSSTDEV) error {
	switch m.Msg {
	case "RANGEA":
		rangea, err := novatelascii.DeserializeRANGEA(m.Data)
		if err != nil {
			return err
		}
		epoch, err := rangea.SerializeGNSSEpoch(m.Time())
		if err != nil {
			return err
		}
		if len(epoch.Satellites) == 0 {
			return fmt.Errorf("RANGEA with no satellites")
		}
		*epochs = append(*epochs, epoch)
	case "INSPVAA":
		r, err := novatel.ParseINSPVA(m.Data, m.Time())
		if err != nil {
			return err
		}
		*inspva = append(*inspva, r)
	case "INSSTDEVA":
		r, err := novatel.ParseINSSTDEV(m.Data, m.Time())
		if err != nil {
			return err
		}
		*insstdev = append(*insstdev, r)
	}
	return nil
}
