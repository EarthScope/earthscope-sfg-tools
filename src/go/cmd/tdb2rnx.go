// Author: Franklyn Dunbar  | Contact franklyn.dunbar@earthscope.org | Dec 2024
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnsstiledb"
)

type tdb2rnxBodyParameters struct {
	URI         string                    `json:"uri"`
	Region      string                    `json:"region"`
	QueryParams gnsstiledb.ObsQueryParams `json:"query"`
}

func parseTdb2rnxSettings(path string) (*rinex.Settings, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed opening settings file: %w", err)
	}
	defer file.Close()
	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed reading settings file: %w", err)
	}
	settings := rinex.NewSettings()
	if err := json.Unmarshal(bytes, settings); err != nil {
		return nil, fmt.Errorf("failed parsing settings file: %w", err)
	}
	if settings.ObservationsBySystem == nil {
		settings.ObservationsBySystem = observation.NewObservationsBySystem()
	}
	return settings, nil
}

func getHourSlice(daySlice gnsstiledb.TimeRange, interval int) []gnsstiledb.TimeRange {
	if interval < 1 {
		log.Warn("Invalid interval, defaulting to 1 hour")
		interval = 1
	} else if interval > 24 {
		log.Warn("Invalid interval, defaulting to 24 hours")
		interval = 24
	}
	hourSlices := []gnsstiledb.TimeRange{}
	prevTime := daySlice.Start
	for i := interval; i <= 24; i += interval {
		endTime := prevTime.Add(time.Duration(interval) * time.Hour)
		if endTime.After(daySlice.End) {
			endTime = daySlice.End
		}
		hourSlices = append(hourSlices, gnsstiledb.TimeRange{Start: prevTime, End: endTime})
		prevTime = endTime
	}
	return hourSlices
}

func filterDaySlices(daySlices []gnsstiledb.TimeRange, year int) ([]gnsstiledb.TimeRange, error) {
	if len(daySlices) == 0 {
		return nil, fmt.Errorf("no day slices found")
	}
	if year <= 0 {
		log.Warn("Year not specified, generating daily RINEX for all years")
		return daySlices, nil
	}
	filtered := []gnsstiledb.TimeRange{}
	for _, slice := range daySlices {
		if slice.Start.Year() == year {
			filtered = append(filtered, slice)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no day slices found for year %d", year)
	}
	return filtered, nil
}

func processDaySlice(ctx context.Context, client *gnsstiledb.Client, daySlice gnsstiledb.TimeRange, tdbPath string, interval int, settings *rinex.Settings, moduloMillis int64) {
	hourSlices := getHourSlice(daySlice, interval)
	batchNum := 0
	var obsWriter *rinex.ObsWriter
	for _, hourSlice := range hourSlices {
		queryParams := gnsstiledb.ObsQueryParams{
			Time: []gnsstiledb.TimeRange{hourSlice},
		}
		epochs, err := client.ReadObservations(ctx, tdbPath, queryParams)
		if err != nil {
			log.Debug("Error Reading TDB: ", err)
		}
		if len(epochs) == 0 {
			log.Debug("No epochs found for the given time slice")
			continue
		}
		log.Infof("Found %d Epochs From Array Within Timespan: %s", len(epochs), hourSlice)

		if moduloMillis > 0 {
			epochs = sfg_utils.DecimateEpochs(epochs, moduloMillis)
			if len(epochs) == 0 {
				log.Debug("No epochs remaining after decimation")
				continue
			}
		}

		if batchNum == 0 {
			settings.TimeOfFirst = epochs[0].Time
			settings.TimeOfLast = daySlice.End
			if settings.RinexVersion.Major == rinex.MajorVersion3 || settings.RinexVersion.Major == rinex.MajorVersion4 {
				for _, epoch := range epochs {
					settings.ObservationsBySystem.AddEpoch(epoch)
				}
			}

			startYear, startMonth, startDay := epochs[0].Time.Date()
			filename := sfg_utils.BuildV3ObsFilename(settings, epochs)
			log.Infof("Generating Daily RINEX File For Year %d, Month %d, Day %d To %s", startYear, startMonth, startDay, filename)

			if _, err := os.Stat(filename); err == nil {
				log.Warnf("File Already Exists: %s", filename)
				if err := os.Remove(filename); err != nil {
					log.Errorf("failed deleting existing file: %s", err)
				}
			}
			outFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				log.Errorf("failed creating output file: %s", err)
				continue
			}
			defer outFile.Close()
			writer := bufio.NewWriter(outFile)
			defer writer.Flush()
			obsWriter = rinex.NewObsWriter(writer, settings)
		}

		if obsWriter == nil {
			log.Warn("obsWriter not initialized for this batch, skipping")
			continue
		}
		for _, epoch := range epochs {
			if _, err := obsWriter.Write(epoch); err != nil {
				log.Warnf("failed writing observation: %s", err)
			}
		}
		batchNum++
	}

	// Drain the ObsWriter's internal buffer: writes the RINEX header (derived
	// from all buffered epochs) followed by the epoch data to the underlying
	// bufio.Writer. The deferred bufio.Flush() then flushes that to disk.
	if obsWriter != nil {
		if err := obsWriter.Flush(); err != nil {
			log.Errorf("failed flushing RINEX output: %s", err)
		}
	}
	log.Infof("==================== COMPLETE ====================")
}

var tdb2rnxCmd = &cobra.Command{
	Use:   "tdb2rnx",
	Short: "Export observations from a TileDB array to RINEX files",
	RunE:  runTdb2rnx,
}

func runTdb2rnx(cmd *cobra.Command, args []string) error {
	log.Println("Starting tdb2rnx")
	sfg_utils.LoadEnv()

	tdbPath, _ := cmd.Flags().GetString("tdb")
	metaPath, _ := cmd.Flags().GetString("settings")
	timeInterval, _ := cmd.Flags().GetInt("timeint")
	processingYear, _ := cmd.Flags().GetInt("year")
	modulo, _ := cmd.Flags().GetInt64("modulo")

	if tdbPath == "" {
		return fmt.Errorf("--tdb is required")
	}
	if metaPath == "" {
		return fmt.Errorf("--settings is required")
	}

	log.SetOutput(os.Stdout)

	if modulo > 0 {
		slog.Info("Decimation enabled", "modulo_ms", modulo)
	}

	// Validate settings file early; each goroutine will re-parse its own copy.
	if _, err := parseTdb2rnxSettings(metaPath); err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	if _, err := os.Stat(tdbPath); err != nil {
		return fmt.Errorf("TileDB array not found at %s: %w", tdbPath, err)
	}

	ctx := context.Background()
	client, err := gnsstiledb.NewClient(ctx, nil, "us-east-2")
	if err != nil {
		return fmt.Errorf("creating gnsstiledb client: %w", err)
	}
	defer client.Close()

	timeStart, timeEnd, err := client.NonEmptyTimeDomain(ctx, tdbPath)
	if err != nil {
		log.Warnf("TileDB array has no data (empty time domain) at %s: %s", tdbPath, err)
		return nil
	}
	log.Infof("Time Range: %s - %s Found At %s", timeStart, timeEnd, tdbPath)

	daySlices := gnsstiledb.SplitTimeRangeCalendar(timeStart, timeEnd, gnsstiledb.PeriodDaily)
	daySlices, err = filterDaySlices(daySlices, processingYear)
	if err != nil {
		log.Warnf("Error filtering day slices: %s", err)
		return nil
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for _, daySlice := range daySlices {
		wg.Add(1)
		go func(daySlice gnsstiledb.TimeRange) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// Re-parse settings for each goroutine to avoid data races on shared
			// mutable fields (TimeOfFirst, TimeOfLast, ObservationsBySystem).
			s, err := parseTdb2rnxSettings(metaPath)
			if err != nil {
				log.Errorf("failed parsing settings for day slice: %s", err)
				return
			}
			processDaySlice(ctx, client, daySlice, tdbPath, timeInterval, s, modulo)
		}(daySlice)
	}
	wg.Wait()
	return nil
}

func init() {
	tdb2rnxCmd.Flags().String("tdb", "", "path to the TileDB array (required)")
	tdb2rnxCmd.Flags().String("settings", "", "settings file (required)")
	tdb2rnxCmd.Flags().Int("timeint", 1, "break array queries into intervals of N hours")
	tdb2rnxCmd.Flags().Int("year", 0, "if set, only process data for the given year")
	tdb2rnxCmd.Flags().Int64("modulo", 0, "decimation modulo in milliseconds (e.g., 1000 for 1 Hz, 15000 for 15s). 0 disables decimation.")
}
