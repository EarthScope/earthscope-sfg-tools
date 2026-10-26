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
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/novatel"
)

// FileType names the file type in errors and stream EDIDs.
const FileType = "NOV000"

// The NovAtel ASCII log types NOV000.bin files carry, one per table. Each is
// also the Log_Source value of its rows.
const (
	LogRANGEA    = "RANGEA"    // GNSSObs
	LogINSPVAA   = "INSPVAA"   // INSPVA
	LogINSSTDEVA = "INSSTDEVA" // INSSTDEV
)

// errBadTime marks a RANGEA whose epoch time fails gnssiceberg.ValidateEpoch:
// a decode error (the ASCII CRC is not checked), not data.
var errBadTime = errors.New("bad epoch time")

// GNSSObs parses the logType (LogRANGEA) logs of the NOV000.bin file at path
// into GNSSObs rows (see datamodels.ParsedSchema). The caller releases the
// table.
func GNSSObs(path, logType string) (arrow.Table, error) {
	r, err := scan(path, logType, LogRANGEA)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildGNSSObs(FileType, datamodels.GNSSObsSource{LogSource: logType, Epochs: r.epochs})
}

// INSPVA parses the logType (LogINSPVAA) logs of the NOV000.bin file at path
// into inspva rows (see datamodels.ParsedSchema). The caller releases the
// table.
func INSPVA(path, logType string) (arrow.Table, error) {
	r, err := scan(path, logType, LogINSPVAA)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildINSPVA(r.inspva, logType)
}

// INSSTDEV parses the logType (LogINSSTDEVA) logs of the NOV000.bin file at
// path into insstdev rows (see datamodels.ParsedSchema). The caller releases
// the table.
func INSSTDEV(path, logType string) (arrow.Table, error) {
	r, err := scan(path, logType, LogINSSTDEVA)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildINSSTDEV(r.insstdev, logType)
}

// records are the decoded logs of one NOV000.bin scan.
type records struct {
	epochs   []observation.Epoch
	inspva   []datamodels.INSPVARow
	insstdev []datamodels.INSSTDEVRow
}

// scan decodes the want logs of the NOV000.bin file at path; want must be
// valid, the log type the calling table is built from. Logs whose header
// time status is UNKNOWN (no time fix), and RANGEA logs whose epoch time
// fails gnssiceberg.ValidateEpoch, are skipped.
func scan(path, want, valid string) (records, error) {
	if err := logtype.Check(FileType, want, valid); err != nil {
		return records{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return records{}, err
	}
	defer f.Close()

	var (
		r       records
		failed  int
		skipped int
		badTime int
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
		if err := decode(m, &r.epochs, &r.inspva, &r.insstdev); err != nil {
			if errors.Is(err, errBadTime) {
				badTime++
			} else {
				failed++
			}
			slog.Debug("bad NovAtel log", "file", filepath.Base(path), "log", m.Msg, "error", err)
		}
	}
	slog.Debug("parsed NOV000", "file", filepath.Base(path), "log", want,
		"epochs", len(r.epochs), "inspva", len(r.inspva), "insstdev", len(r.insstdev),
		"failed", failed, "unknown_time", skipped, "bad_time", badTime)
	return r, nil
}

func decode(m novatelascii.LongMessage, epochs *[]observation.Epoch, inspva *[]datamodels.INSPVARow, insstdev *[]datamodels.INSSTDEVRow) error {
	switch m.Msg {
	case LogRANGEA:
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
		if err := gnssiceberg.ValidateEpoch(epoch.Time); err != nil {
			return fmt.Errorf("%w: %v", errBadTime, err)
		}
		*epochs = append(*epochs, epoch)
	case LogINSPVAA:
		r, err := novatel.ParseINSPVA(m.Data, m.Time())
		if err != nil {
			return err
		}
		*inspva = append(*inspva, r)
	case LogINSSTDEVA:
		r, err := novatel.ParseINSSTDEV(m.Data, m.Time())
		if err != nil {
			return err
		}
		*insstdev = append(*insstdev, r)
	}
	return nil
}
