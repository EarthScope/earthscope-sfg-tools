package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

func runNova2rnx(cmd *cobra.Command, args []string) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	metaPath, _ := cmd.Flags().GetString("settings")
	modulo, _ := cmd.Flags().GetInt64("modulo")

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

	epochs := []observation.Epoch{}
	for _, filename := range args {
		file_epochs, fails, err := sfg_utils.ProcessFileNOVASCII(filename)
		if err != nil {
			slog.Error("Error processing file", "filename", filename, "error", err)
		}
		epochs = append(epochs, file_epochs...)
		slog.Info("Processed file", "filename", filename, "num_epochs", len(file_epochs), "num_fails", fails)
	}

	slog.Info("Total epochs processed", "count", len(epochs))

	if modulo > 0 {
		epochs = sfg_utils.DecimateEpochs(epochs, modulo)
	}

	batchedEpochs, err := sfg_utils.BatchEpochsByDay(epochs)
	if err != nil {
		return fmt.Errorf("batching epochs by day: %w", err)
	}

	for dayKey, dayEpochs := range batchedEpochs {
		slog.Info("Writing RINEX for day", "day", dayKey, "num_epochs", len(dayEpochs))
		if err := sfg_utils.WriteEpochs(dayEpochs, settings); err != nil {
			slog.Error("Error writing epochs", "error", err)
		}
	}

	return nil
}

var nova2rnxCmd = &cobra.Command{
	Use:   "nova2rnx [files...]",
	Short: "Convert NovAtel ASCII logs to RINEX",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runNova2rnx,
}

func init() {
	nova2rnxCmd.Flags().String("settings", "", "settings file (required)")
	nova2rnxCmd.Flags().Int64("modulo", 0, "decimation modulo in milliseconds (e.g., 1000 for 1 Hz, 15000 for 15s). 0 disables decimation.")
}
