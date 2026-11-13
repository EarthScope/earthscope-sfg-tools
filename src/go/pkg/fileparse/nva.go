// nva: parses plain NovAtel ASCII logs (RANGEA, INSPVAA, INSSTDEVA), as a
// receiver writes them with no Sonardyne GPSA wrapping.

package fileparse

import (
	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// FileTypeNVA names the file type in errors and stream EDIDs.
const FileTypeNVA = "NVA"

// parseNVAGNSSObs parses the RANGEA logs of the NovAtel ASCII file at path
// into GNSSObs rows (stamped with stream_edid for scope). Logs are skipped as
// by scanNovatelASCII. The caller releases the table.
func parseNVAGNSSObs(path string, scope edid.Scope) (arrow.Table, error) {
	r, err := scanNovatelASCII(path, FileTypeNVA, newNovatelASCIIScanner, LogRANGEA)
	if err != nil {
		return nil, err
	}
	return BuildGNSSObs(FileTypeNVA, GNSSObsSource{EDID: edid.Stream(scope, FileTypeNVA, LogRANGEA), Epochs: r.epochs})
}

// parseNVAINSPVA parses the INSPVAA logs of the NovAtel ASCII file at path
// into inspva rows (stamped with stream_edid for scope). The caller releases
// the table.
func parseNVAINSPVA(path string, scope edid.Scope) (arrow.Table, error) {
	r, err := scanNovatelASCII(path, FileTypeNVA, newNovatelASCIIScanner, LogINSPVAA)
	if err != nil {
		return nil, err
	}
	return BuildINSPVA(r.inspva, edid.Stream(scope, FileTypeNVA, LogINSPVAA))
}

// parseNVAINSSTDEV parses the INSSTDEVA logs of the NovAtel ASCII file at
// path into insstdev rows (stamped with stream_edid for scope). The caller
// releases the table.
func parseNVAINSSTDEV(path string, scope edid.Scope) (arrow.Table, error) {
	r, err := scanNovatelASCII(path, FileTypeNVA, newNovatelASCIIScanner, LogINSSTDEVA)
	if err != nil {
		return nil, err
	}
	return BuildINSSTDEV(r.insstdev, edid.Stream(scope, FileTypeNVA, LogINSSTDEVA))
}
