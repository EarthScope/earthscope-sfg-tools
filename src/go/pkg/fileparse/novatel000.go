// novatel000: parses Sonardyne NOV000.bin logs: NovAtel ASCII logs
// (RANGEA, INSPVAA, INSSTDEVA) carried in GPSA binary packets.

package fileparse

import (
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// FileTypeNOV000 names the file type in errors and stream EDIDs.
const FileTypeNOV000 = "NOV000"

// parseNOV000GNSSObs parses the RANGEA logs of the NOV000.bin file at path into GNSSObs
// rows (stamped with stream_edid for scope). The caller releases the table.
func parseNOV000GNSSObs(path string, scope edid.Scope) (arrow.Table, error) {
	epochs, err := nov000Epochs(path)
	if err != nil {
		return nil, err
	}
	return BuildGNSSObs(FileTypeNOV000, GNSSObsSource{EDID: edid.Stream(scope, FileTypeNOV000, LogRANGEA), Epochs: epochs})
}

// nov000Epochs decodes the RANGEA epochs of the NOV000.bin file at path.
func nov000Epochs(path string) ([]observation.Epoch, error) {
	r, err := scanNovatelASCII(path, FileTypeNOV000, newNovatelGPSAASCIIScanner, LogRANGEA)
	return r.epochs, err
}

// parseNOV000INSPVA parses the INSPVAA logs of the NOV000.bin file at path into inspva
// rows (stamped with stream_edid for scope). The caller releases the table.
func parseNOV000INSPVA(path string, scope edid.Scope) (arrow.Table, error) {
	r, err := scanNovatelASCII(path, FileTypeNOV000, newNovatelGPSAASCIIScanner, LogINSPVAA)
	if err != nil {
		return nil, err
	}
	return BuildINSPVA(r.inspva, edid.Stream(scope, FileTypeNOV000, LogINSPVAA))
}

// parseNOV000INSSTDEV parses the INSSTDEVA logs of the NOV000.bin file at path into
// insstdev rows (stamped with stream_edid for scope). The caller releases the table.
func parseNOV000INSSTDEV(path string, scope edid.Scope) (arrow.Table, error) {
	r, err := scanNovatelASCII(path, FileTypeNOV000, newNovatelGPSAASCIIScanner, LogINSSTDEVA)
	if err != nil {
		return nil, err
	}
	return BuildINSSTDEV(r.insstdev, edid.Stream(scope, FileTypeNOV000, LogINSSTDEVA))
}

// --- INSSTDEV rows ---

// BuildINSSTDEV converts records to insstdev rows of stream edid; times
// become UTC. The cov_* columns hold the logged attitude std devs (see INSSTDEV).
func BuildINSSTDEV(records []INSSTDEVRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.INSSTDEV)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	edidCol := col[*array.StringBuilder](b, datamodels.StreamEDIDName)
	tm := col[*array.TimestampBuilder](b, "time")
	var cols []*array.Float64Builder
	for _, name := range []string{"lat_sig", "lon_sig", "alt_sig", "north_sig", "east_sig", "up_sig", "cov_rr", "cov_pp", "cov_hh"} {
		cols = append(cols, col[*array.Float64Builder](b, name))
	}
	extSolStat := col[*array.StringBuilder](b, "ext_sol_stat")
	sinceUpdate := col[*array.StringBuilder](b, "time_since_update")
	timeStatus := col[*array.StringBuilder](b, "time_status")
	for _, r := range records {
		edidCol.Append(edid)
		tm.Append(utcMicros(r.Time))
		for i, v := range []float64{r.LatSig, r.LonSig, r.AltSig, r.NorthSig, r.EastSig, r.UpSig, r.RollSig, r.PitchSig, r.AzimuthSig} {
			cols[i].Append(v)
		}
		appendOpt(extSolStat, r.ExtSolStat)
		appendOpt(sinceUpdate, r.TimeSinceUpdate)
		appendOpt(timeStatus, r.TimeStatus)
	}
	return b.Table()
}
