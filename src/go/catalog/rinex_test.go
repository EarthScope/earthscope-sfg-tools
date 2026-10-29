package catalog_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse"
)

// epochs is n one-satellite (G05, L1C) epochs every 30 s from start (GPST).
func epochs(t *testing.T, start time.Time, n int, code float64) []observation.Epoch {
	t.Helper()
	key, err := observation.ParseObservationKey(gnss.GPS, "1C")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]observation.Epoch, n)
	for i := range out {
		e := observation.NewEpoch()
		e.Time = start.Add(time.Duration(i) * 30 * time.Second)
		sat := &observation.Satellite{SatelliteKey: observation.SatelliteKey{System: gnss.GPS, SatelliteID: 5}}
		sat.AddObservation(observation.Observation{Code: key, Pseudorange: code + float64(i), Phase: 1e8 + float64(i), SNR: 45})
		e.AddSatellite(sat)
		out[i] = e
	}
	return out
}

// appendObs writes epochs to the catalog as fileType's logSource stream of scope.
func appendObs(t *testing.T, cat *catalog.Catalog, scope datamodels.Scope, fileType, logSource string, eps []observation.Epoch) {
	t.Helper()
	p := fileparse.Parser{
		FileType: fileType,
		Table:    datamodels.GNSSObs,
		Parsefunc: func(string) (arrow.Table, error) {
			return datamodels.BuildGNSSObs(fileType, datamodels.GNSSObsSource{LogSource: logSource, Epochs: eps})
		},
		EDID: func(s datamodels.Scope, ls string) string { return datamodels.StreamEDID(s, fileType, ls) },
	}
	rows, err := p.Parse("", scope)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Release()
	if _, err := cat.Append(context.Background(), datamodels.GNSSObs, rows); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRINEX(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(ctx, catalog.Config{Type: catalog.TypeSQL, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	scope := datamodels.Scope{Network: "net", Station: "STA1", Campaign: "2025_A_1126"}
	day := time.Date(2025, 9, 15, 0, 0, 0, 0, time.UTC)

	// NOV000 RANGEA spans midnight (10 epochs on the 15th, 5 on the 16th);
	// QCPIN RANGEA shares the log source and day but is another stream.
	appendObs(t, cat, scope, "NOV000", "RANGEA", epochs(t, day.Add(24*time.Hour-5*time.Minute), 15, 2.1e7))
	appendObs(t, cat, scope, "QCPIN", "RANGEA", epochs(t, day.Add(12*time.Hour), 3, 2.3e7))

	settings := rinex.NewSettings()
	settings.MarkerName = scope.Station
	var buf bytes.Buffer
	n, err := cat.WriteRINEX(ctx, &buf, scope, "NOV000", "RANGEA", day.Add(13*time.Hour), settings)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Errorf("%d epochs, want 10 (NOV000 RANGEA on 2025-09-15 only)", n)
	}
	out := buf.String()
	if got := strings.Count(out, "\n> "); got != 10 {
		t.Errorf("%d epoch records, want 10", got)
	}
	if !strings.Contains(out, "STA1") || !strings.Contains(out, "G05") {
		t.Errorf("RINEX lacks marker STA1 or satellite G05:\n%s", out)
	}
	if strings.Contains(out, "2025 09 16") || strings.Contains(out, "23000000.000") {
		t.Errorf("RINEX holds another day's or stream's epochs:\n%s", out)
	}

	if _, err := cat.WriteRINEX(ctx, &buf, scope, "NOV000", "RANGEA", day.AddDate(0, 0, 2), settings); !errors.Is(err, catalog.ErrNoObservations) {
		t.Errorf("empty day: err = %v, want ErrNoObservations", err)
	}
}
