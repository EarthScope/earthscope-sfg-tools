package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"
	"gitlab.com/earthscope/gnsstools/geodata/gnssparquet/schema"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// ErrNoObservations is returned by WriteRINEX when the stream has no
// observations on the requested day.
var ErrNoObservations = errors.New("no observations")

// WriteRINEX writes one GPS day of the gnss_observations stream of logSource
// from fileType files in scope (datamodels.StreamEDID) to w as RINEX
// observations, and returns the number of epochs written. The file type is
// part of the stream: a log source such as RANGEA can come from more than one.
//
// The day is the GPS calendar day holding day's date, [00:00, 24:00) GPST,
// matching the table's day(time_gpst) partitions. settings supplies the
// header (marker, receiver, antenna, RINEX version); its observation types
// and first/last times come from the epochs. The day's epochs are held in
// memory, since a RINEX 3/4 header needs every epoch before the first is
// written.
func (c *Catalog) WriteRINEX(ctx context.Context, w io.Writer, scope datamodels.Scope, fileType, logSource string, day time.Time, settings *rinex.Settings) (int, error) {
	tbl, err := c.Table(ctx, datamodels.GNSSObs)
	if err != nil {
		return 0, err
	}
	edid := datamodels.StreamEDID(scope, fileType, logSource)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	// Rows are grouped into epochs batch by batch, so only the epochs are
	// held; an epoch split across batches is merged by time.
	byTime := map[time.Time]*observation.Epoch{}
	err = gnssiceberg.ReadObservationRows(ctx, tbl, gnssiceberg.ScanOptions{}, edid, start, start.AddDate(0, 0, 1),
		func(batch []schema.ObservationRow) error {
			eps, err := schema.GroupToEpochs(batch, nil)
			if err != nil {
				return err
			}
			for i := range eps {
				if e, ok := byTime[eps[i].Time]; ok {
					for _, sat := range eps[i].Satellites {
						e.AddSatellite(sat)
					}
					continue
				}
				byTime[eps[i].Time] = &eps[i]
			}
			return nil
		})
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", edid, err)
	}
	if len(byTime) == 0 {
		return 0, fmt.Errorf("%s on %s: %w", edid, start.Format(time.DateOnly), ErrNoObservations)
	}
	times := slices.SortedFunc(maps.Keys(byTime), time.Time.Compare)

	ow := rinex.NewObsWriter(w, settings)
	for _, t := range times {
		if _, err := ow.Write(*byTime[t]); err != nil {
			return 0, err
		}
	}
	if err := ow.Flush(); err != nil {
		return 0, err
	}
	return len(times), nil
}
