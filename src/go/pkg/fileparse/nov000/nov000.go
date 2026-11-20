// Package nov000 parses Sonardyne NOV000.bin logs: NovAtel ASCII logs
// (RANGEA, INSPVAA, INSSTDEVA) carried in GPSA binary packets.
package nov000

import (
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/internal/rowutil"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/novatel"
)

// fileType names the file type in logs; it is the file type of the log
// sources in datamodels.
var fileType = datamodels.NOV000RANGEA.FileType

// ParseGNSSObs parses the RANGEA logs of the NOV000.bin file at path into GNSSObs
// rows. src selects one log source, or edid.AllLogSources.
func ParseGNSSObs(path string, src edid.LogSource) ([]datamodels.GNSSObsRow, error) {
	epochs, err := Epochs(path)
	if err != nil {
		return nil, err
	}
	rs := novatel.ObsRows(datamodels.NOV000RANGEA, epochs)
	return rowutil.Filter(rs, src, func(r datamodels.GNSSObsRow) edid.LogSource { return r.LogSource }), nil
}

// Epochs decodes the RANGEA epochs of the NOV000.bin file at path, skipping
// the logs ParseGNSSObs describes.
func Epochs(path string) ([]observation.Epoch, error) {
	r, err := novatel.Scan(path, fileType, novatel.NewGPSAASCIIScanner, novatel.LogRANGEA)
	return r.Epochs, err
}

// ParseINSPVA parses the INSPVAA logs of the NOV000.bin file at path into inspva
// rows. src selects one log source, or edid.AllLogSources.
func ParseINSPVA(path string, src edid.LogSource) ([]datamodels.INSPVARow, error) {
	r, err := novatel.Scan(path, fileType, novatel.NewGPSAASCIIScanner, novatel.LogINSPVAA)
	if err != nil {
		return nil, err
	}
	rs := r.INSPVA
	for i := range rs {
		rs[i].LogSource = datamodels.NOV000INSPVAA
	}
	return rowutil.Filter(rs, src, func(r datamodels.INSPVARow) edid.LogSource { return r.LogSource }), nil
}

// ParseINSSTDEV parses the INSSTDEVA logs of the NOV000.bin file at path into
// insstdev rows. src selects one log source, or edid.AllLogSources.
func ParseINSSTDEV(path string, src edid.LogSource) ([]datamodels.INSSTDEVRow, error) {
	r, err := novatel.Scan(path, fileType, novatel.NewGPSAASCIIScanner, novatel.LogINSSTDEVA)
	if err != nil {
		return nil, err
	}
	rs := r.INSSTDEV
	for i := range rs {
		rs[i].LogSource = datamodels.NOV000INSSTDEVA
	}
	return rowutil.Filter(rs, src, func(r datamodels.INSSTDEVRow) edid.LogSource { return r.LogSource }), nil
}
