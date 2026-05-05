package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)


func runNovb2rnx(cmd *cobra.Command, args []string) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	metaPath, _ := cmd.Flags().GetString("settings")
	modulo, _ := cmd.Flags().GetInt64("modulo")
	numRoutines, _ := cmd.Flags().GetInt("numroutines")
	antIndex, _ := cmd.Flags().GetInt("antindex")

	if metaPath == "" {
		return fmt.Errorf("--settings is required")
	}

	metaFile, err := os.Open(metaPath)
	if err != nil {
		return fmt.Errorf("opening settings file: %w", err)
	}
	defer metaFile.Close()

	metaBytes, err := io.ReadAll(metaFile)
	if err != nil {
		return fmt.Errorf("reading settings file: %w", err)
	}

	settings := rinex.NewSettings()
	if err := json.Unmarshal(metaBytes, &settings); err != nil {
		return fmt.Errorf("parsing settings JSON: %w", err)
	}

	filename_times, err := sfg_utils.SortFilesByFirstEpochNOVB(args)
	if err != nil {
		return fmt.Errorf("sorting files by first epoch: %w", err)
	}

	filename_times_batched := make(map[string][]sfg_utils.FileTime)
	for _, fileTime := range filename_times {
		daykey := sfg_utils.GetYMDKey(fileTime.Time)
		filename_times_batched[daykey] = append(filename_times_batched[daykey], fileTime)
	}

	epoch_count := 0
	var wg sync.WaitGroup
	sem := make(chan struct{}, numRoutines)
	batched_epochs := make(map[string][]observation.Epoch)
	mu := sync.Mutex{}

	for YMD_KEY, fileTimes := range filename_times_batched {
		wg.Add(1)
		go func(fileNameTimes []sfg_utils.FileTime) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			for _, fileNameTime := range fileNameTimes {
				file_epochs, fails, err := sfg_utils.ProcessFileNOVB(fileNameTime.Filename, uint8(antIndex))
				if err != nil {
					slog.Error("Error processing file", "filename", fileNameTime.Filename, "error", err)
					return
				}
				if len(file_epochs) == 0 {
					slog.Warn("No epochs found in file", "filename", fileNameTime.Filename)
					return
				}
				if modulo > 0 {
					file_epochs = sfg_utils.DecimateEpochs(file_epochs, modulo)
				}
				num_epochs := len(file_epochs)
				epoch_count += num_epochs

				batched_epochs_sub, err := sfg_utils.BatchEpochsByDay(file_epochs)
				if err != nil {
					slog.Error("Error batching epochs by day", "error", err)
					return
				}
				file_epochs = nil

				for key, day_epoch_batch := range batched_epochs_sub {
					mu.Lock()
					batched_epochs[key] = append(batched_epochs[key], day_epoch_batch...)
					delete(batched_epochs_sub, key)
					mu.Unlock()
				}

				startYear, startMonth, startDay := fileNameTime.Time.Date()
				currentDate := time.Date(startYear, startMonth, startDay, 0, 0, 0, 0, time.UTC)
				slog.Info("Processed file", "filename", fileNameTime.Filename, "Year", startYear, "Day of Year", currentDate.YearDay(), "num_epochs", num_epochs, "num_fails", fails)
			}

			if batched_epochs[YMD_KEY] != nil {
				if err := sfg_utils.WriteEpochs(batched_epochs[YMD_KEY], settings); err != nil {
					slog.Error("Error writing epochs", "error", err)
				}
				delete(batched_epochs, YMD_KEY)
			}
		}(fileTimes)
	}
	wg.Wait()

	slog.Info("Total epochs processed", "count", epoch_count)
	return nil
}

var novb2rnxCmd = &cobra.Command{
	Use:   "novb2rnx [files...]",
	Short: "Convert NovAtel binary logs to RINEX",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runNovb2rnx,
}

func init() {
	novb2rnxCmd.Flags().String("settings", "", "settings file (required)")
	novb2rnxCmd.Flags().Int64("modulo", 0, "decimation modulo in milliseconds (e.g., 100 for 10 Hz, 1000 for 1 Hz, 15000 for 15s). 0 disables decimation.")
	novb2rnxCmd.Flags().Int("numroutines", 1, "number of concurrent goroutines for processing files")
	novb2rnxCmd.Flags().Int("antindex", 0, "index of antenna to use for position and clock when multiple antennas are present")
}
