// Package novatel000 parses Sonardyne NOV000.bin logs: NovAtel ASCII logs
// (RANGEA, INSPVAA, INSSTDEVA) carried in GPSA binary packets.
package novatel000

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/internal/novatel"
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

// GNSSObs parses the RANGEA logs of the NOV000.bin file at path into GNSSObs
// rows (see datamodels.ParsedSchema). The caller releases the table.
func GNSSObs(path string) (arrow.Table, error) {
	r, err := scan(path, LogRANGEA)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildGNSSObs(FileType, datamodels.GNSSObsSource{LogSource: LogRANGEA, Epochs: r.epochs})
}

// INSPVA parses the INSPVAA logs of the NOV000.bin file at path into inspva
// rows (see datamodels.ParsedSchema). The caller releases the table.
func INSPVA(path string) (arrow.Table, error) {
	r, err := scan(path, LogINSPVAA)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildINSPVA(r.inspva, LogINSPVAA)
}

// INSSTDEV parses the INSSTDEVA logs of the NOV000.bin file at path into
// insstdev rows (see datamodels.ParsedSchema). The caller releases the table.
func INSSTDEV(path string) (arrow.Table, error) {
	r, err := scan(path, LogINSSTDEVA)
	if err != nil {
		return nil, err
	}
	return datamodels.BuildINSSTDEV(r.insstdev, LogINSSTDEVA)
}

// records are the decoded logs of one NOV000.bin scan.
type records struct {
	epochs   []observation.Epoch
	inspva   []datamodels.INSPVARow
	insstdev []datamodels.INSSTDEVRow
}

// scan decodes the want logs of the NOV000.bin file at path. Logs with no
// time fix (header time status UNKNOWN, or week 0, which a receiver still
// adjusting its clock logs under other statuses), and RANGEA logs whose epoch
// time fails gnssiceberg.ValidateEpoch, are skipped.
func scan(path, want string) (records, error) {
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
		if m.TimeStatus == "UNKNOWN" || m.Week == 0 {
			skipped++
			continue
		}
		if err := decode(m, &r.epochs, &r.inspva, &r.insstdev); err != nil {
			if errors.Is(err, novatel.ErrBadEpochTime) {
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

// timeStatus is m's header time status, or nil when it has none.
func timeStatus(m novatelascii.LongMessage) *string {
	if m.TimeStatus == "" {
		return nil
	}
	s := m.TimeStatus
	return &s
}

func decode(m novatelascii.LongMessage, epochs *[]observation.Epoch, inspva *[]datamodels.INSPVARow, insstdev *[]datamodels.INSSTDEVRow) error {
	switch m.Msg {
	case LogRANGEA:
		epoch, err := novatel.RANGEAEpoch(m)
		if err != nil {
			return err
		}
		*epochs = append(*epochs, epoch)
	case LogINSPVAA:
		r, err := novatel.ParseINSPVA(m.Data, m.Time())
		if err != nil {
			return err
		}
		r.TimeStatus = timeStatus(m)
		*inspva = append(*inspva, r)
	case LogINSSTDEVA:
		r, err := novatel.ParseINSSTDEV(m.Data, m.Time())
		if err != nil {
			return err
		}
		r.TimeStatus = timeStatus(m)
		*insstdev = append(*insstdev, r)
	}
	return nil
}
