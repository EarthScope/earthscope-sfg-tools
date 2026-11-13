// dfop00: parses Sonardyne SV3 DFOP00 logs: one JSON event per line,
// "interrogation" (a ping) or "range" (a transponder reply). Event decoding
// and the rows built from events are shared with QCPIN (see sv3.go).

package fileparse

import (
	"log/slog"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// FileTypeDFOP00 names the file type in stream EDIDs.
const FileTypeDFOP00 = "DFOP00"

// parseDFOP00TWTT parses the events of the DFOP00 file at path into
// travel_times rows (stamped with stream_edid for scope):
// each range event pairs with the most recent preceding interrogation, and a
// zero range (no reply detected) yields no row. The caller releases the
// table.
func parseDFOP00TWTT(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return BuildTWTT(sv3TravelTimes(events), edid.Stream(scope, FileTypeDFOP00, edid.LogINTGRPLY))
}

// parseDFOP00INSPVA parses the NOV_INS blocks of the DFOP00 file at path into inspva
// rows (stamped with stream_edid for scope): one row per distinct block, the NovAtel
// INS solution at the event. The caller releases the table.
//
// The events' AHRS and GNSS blocks are not parsed: the GNSS block is this
// INS position, rounded, with the receiver's single-point std devs, and the
// AHRS00 files log the AHRS at full rate.
func parseDFOP00INSPVA(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return BuildINSPVA(sv3INSPVA(events), edid.Stream(scope, FileTypeDFOP00, edid.LogINSPVAA))
}

// parseDFOP00CommunityStandard parses the events of the DFOP00 file at path into
// community_standard rows (stamped with stream_edid for scope): one GNSS-A
// shot per ping/reply pair that passes the SV3 pairing rules (see
// sv3Shots), with the travel time corrected for TAT and trigger delay, the
// antenna position from each event's GNSS block and the attitude from its
// NOV_INS block. Transducer positions are null. The caller releases the
// table.
func parseDFOP00CommunityStandard(path string, scope edid.Scope) (arrow.Table, error) {
	events, err := parseEvents(path)
	if err != nil {
		return nil, err
	}
	return BuildCommunityStandard(sv3Shots(events), edid.Stream(scope, FileTypeDFOP00, edid.LogINTGRPLY))
}

// parseEvents reads the events of the DFOP00 file at path, logging how many
// lines were not valid JSON.
func parseEvents(path string) ([]sv3Event, error) {
	events, bad, err := readSV3JSONL(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed DFOP00", "file", filepath.Base(path), "events", len(events), "bad_lines", bad)
	return events, nil
}

// --- Community standard rows ---

// BuildCommunityStandard builds community_standard rows of stream edid.
func BuildCommunityStandard(rs []CommunityStandardRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.CommunityStandard)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol    = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		mtID       = col[*array.StringBuilder](b, "MT_ID")
		travelTime = col[*array.Float64Builder](b, "TravelTime")
		tTransmit  = col[*array.TimestampBuilder](b, "T_transmit")
		tReceive   = col[*array.TimestampBuilder](b, "T_receive")
		tdcID      = col[*array.StringBuilder](b, "TDC_ID")
		aSNR       = col[*array.Float64Builder](b, "aSNR")
		acc        = col[*array.Int32Builder](b, "acc")
		dbV        = col[*array.Float32Builder](b, "dbV")
		quality    = col[*array.StringBuilder](b, "quality_flag")
	)
	transducer := func(suffix string) [3]*array.Float64Builder {
		return [3]*array.Float64Builder{
			col[*array.Float64Builder](b, "X_"+suffix),
			col[*array.Float64Builder](b, "Y_"+suffix),
			col[*array.Float64Builder](b, "Z_"+suffix),
		}
	}
	xdTransmit, xdReceive := transducer("transmit"), transducer("receive")
	epoch := func(suffix string) [9]*array.Float64Builder {
		var out [9]*array.Float64Builder
		for i, name := range []string{"ant_X", "ant_Y", "ant_Z", "ant_sigX", "ant_sigY", "ant_sigZ", "heading", "pitch", "roll"} {
			out[i] = col[*array.Float64Builder](b, name+suffix)
		}
		return out
	}
	ep0, ep1 := epoch("0"), epoch("1")
	appendEpoch := func(cols [9]*array.Float64Builder, e AntennaEpoch) {
		for i, v := range []*float64{e.X, e.Y, e.Z, e.SigX, e.SigY, e.SigZ, e.Heading, e.Pitch, e.Roll} {
			appendOpt(cols[i], v)
		}
	}
	for _, r := range rs {
		edidCol.Append(edid)
		mtID.Append(r.MTID)
		travelTime.Append(r.TravelTime)
		tTransmit.Append(micros(r.TTransmit))
		tReceive.Append(micros(r.TReceive))
		for i, v := range []*float64{r.XTransmit, r.YTransmit, r.ZTransmit} {
			appendOpt(xdTransmit[i], v)
		}
		for i, v := range []*float64{r.XReceive, r.YReceive, r.ZReceive} {
			appendOpt(xdReceive[i], v)
		}
		appendOpt(tdcID, r.TDCID)
		appendOpt(aSNR, r.ASNR)
		appendOpt(acc, r.Acc)
		appendOpt(dbV, r.DBV)
		appendOpt(quality, r.QualityFlag)
		appendEpoch(ep0, r.Transmit)
		appendEpoch(ep1, r.Receive)
	}
	return b.Table()
}
