package fileparse

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go/table"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"

	"github.com/EarthScope/es_sfgtools/src/golangtools/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse/novatel770"
)

// TestBuildGNSSObsMatchesGnsstools guards datamodels.BuildGNSSObs, a copy of
// gnsstools' gnssiceberg observations writer, plus Parser.Parse's stamping,
// against drifting from it: the same epochs written by both must scan back as
// the same rows (write_time and arrival_time aside).
func TestBuildGNSSObsMatchesGnsstools(t *testing.T) {
	ctx := context.Background()
	gpst := time.Date(2023, 6, 21, 17, 11, 1, 50_000_123, time.UTC)
	lock := 12.5

	gps := observation.NewSatellite(gnss.GPS, 30)
	gps.Observations = append(gps.Observations,
		observation.Observation{Code: obsKey(t, gnss.GPS, "1C"), Pseudorange: 21007753.481,
			Phase: 110396605.995, Doppler: -1631.778, SNR: 51.5, LockTime: &lock,
			Flags: observation.FlagLossOfLock},
		observation.Observation{Code: obsKey(t, gnss.GPS, "2W"), Pseudorange: 21007760.712},
	)
	glo := observation.NewSatellite(gnss.GLONASS, 2)
	glo.Fcn = -3
	glo.Observations = append(glo.Observations,
		observation.Observation{Code: obsKey(t, gnss.GLONASS, "1C"), Pseudorange: 19660946.553,
			Phase: 105283811.1, SNR: 44})
	epochs := []observation.Epoch{
		{Time: gpst, EpochPico: 456, ClockSteeringIndicator: 1,
			Satellites: observation.Satellites{gps}},
		{Time: gpst.Add(time.Second), ReceiverClockOffset: 1e-7, EpochFlag: 1,
			Satellites: observation.Satellites{gps, glo}},
	}
	scope := datamodels.Scope{Network: "n", Station: "s", Campaign: "c"}
	p := parserFor(t, novatel770.FileType, datamodels.GNSSObs.Name)
	edid := p.EDID(scope, novatel770.LogRANGECMP5B)

	// Ours: BuildGNSSObs -> Parser stamping -> Catalog.Append.
	parsed, err := datamodels.BuildGNSSObs(p.FileType, datamodels.GNSSObsSource{LogSource: novatel770.LogRANGECMP5B, Epochs: epochs})
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Release()
	ours, err := p.stamp(parsed, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer ours.Release()
	cat, err := catalog.Open(ctx, catalog.Config{Type: catalog.TypeSQL, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if _, err := cat.Append(ctx, datamodels.GNSSObs, ours); err != nil {
		t.Fatal(err)
	}
	oursTbl, err := cat.Table(ctx, datamodels.GNSSObs)
	if err != nil {
		t.Fatal(err)
	}

	// gnsstools: its own writer into its own SQL catalog.
	dir := t.TempDir()
	catCfg := gnssiceberg.CatalogConfig{Type: gnssiceberg.CatalogTypeSQL, URI: "file:" + filepath.Join(dir, "catalog.db")}
	tblCfg := gnssiceberg.TableConfig{DatabaseName: "probe", TableName: "observations", WarehouseLocation: dir}
	w, err := gnssiceberg.NewObservationsIcebergWriter(ctx, catCfg, tblCfg, gnssiceberg.WriteConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteEpochs(ctx, epochs, edid); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(ctx); err != nil {
		t.Fatal(err)
	}
	theirsTbl, err := gnssiceberg.LoadIcebergTable(ctx, catCfg, tblCfg)
	if err != nil {
		t.Fatal(err)
	}

	got, want := scanRows(t, ctx, oursTbl), scanRows(t, ctx, theirsTbl)
	if len(want) != 5 {
		t.Fatalf("gnsstools wrote %d rows, want 5", len(want))
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows differ from gnsstools:\n got: %s\nwant: %s",
			strings.Join(got, "\n      "), strings.Join(want, "\n      "))
	}
}

// scanRows scans tbl and returns each row as "col=value|..." over the
// columns both writers set, sorted.
func scanRows(t *testing.T, ctx context.Context, tbl *table.Table) []string {
	t.Helper()
	at, err := tbl.Scan().ToArrowTable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer at.Release()
	rdr := array.NewTableReader(at, -1)
	defer rdr.Release()
	var rows []string
	for rdr.Next() {
		rec := rdr.RecordBatch()
		for i := range int(rec.NumRows()) {
			var b strings.Builder
			for c, f := range rec.Schema().Fields() {
				if f.Name == gnssiceberg.ObsColWriteTime || f.Name == gnssiceberg.ObsColArrivalTime {
					continue
				}
				b.WriteString(f.Name + "=" + valueStr(rec.Column(c), i) + "|")
			}
			rows = append(rows, b.String())
		}
	}
	slices.Sort(rows)
	return rows
}

func valueStr(a arrow.Array, i int) string {
	if a.IsNull(i) {
		return "NULL"
	}
	return a.ValueStr(i)
}

func obsKey(t *testing.T, sys gnss.System, code string) observation.ObservationKey {
	t.Helper()
	k, err := observation.ParseObservationKey(sys, code)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// parserFor returns the parser in Parsers for fileType and table.
func parserFor(t *testing.T, fileType, table string) Parser {
	t.Helper()
	for _, p := range Parsers {
		if p.FileType == fileType && p.Table.Name == table {
			return p
		}
	}
	t.Fatalf("no parser for %s %s", fileType, table)
	return Parser{}
}
