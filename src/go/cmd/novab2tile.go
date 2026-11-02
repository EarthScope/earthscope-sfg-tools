// Author: Franklyn Dunbar  | Contact franklyn.dunbar@earthscope.org | Dec 2024
package main

import (
	"context"
	"fmt"
	"time"

	sfg_utils "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnsstiledb"
)

func runNovab2tile(cmd *cobra.Command, args []string) error {
	sfg_utils.LoadEnv()

	tdbPath, _ := cmd.Flags().GetString("tdb")
	numProcs, _ := cmd.Flags().GetInt("procs")
	antIndex, _ := cmd.Flags().GetInt("antindex")
	batchSize, _ := cmd.Flags().GetInt("batch-size")

	if tdbPath == "" {
		return fmt.Errorf("--tdb is required")
	}
	if batchSize < 1 {
		return fmt.Errorf("--batch-size must be at least 1")
	}

	if numProcs != 1 {
		log.Warn("--procs is ignored for NOV770 observation ingestion; chronological processing is required for continuous LLI tracking")
	}

	ctx := context.Background()
	client, err := gnsstiledb.NewClient(ctx, nil, "us-east-2")
	if err != nil {
		return fmt.Errorf("creating gnsstiledb client: %w", err)
	}
	defer client.Close()

	if !client.ArrayExists(tdbPath) {
		if err := client.CreateArray(ctx, "s3://earthscope-tiledb-schema-dev-us-east-2-ebamji/GNSS_OBS_SCHEMA_V3.tdb/", tdbPath); err != nil {
			log.Errorf("error creating array: %v", err)
		}
	} else {
		log.Infof("array %s already exists", tdbPath)
	}

	startTime := time.Now()
	fileTimes, err := sfg_utils.SortFilesByFirstEpochNOVB(args)
	if err != nil {
		return fmt.Errorf("sorting NOV770 files: %w", err)
	}
	writer := client.NewObsWriter(ctx, tdbPath, gnsstiledb.WithObsBatchSize(batchSize))
	epochCount := 0
	processor := sfg_utils.NewContinuousEpochProcessor(func(epoch observation.Epoch) error {
		epochCount++
		_, err := writer.Write(epoch)
		return err
	})
	totalFailures := 0
	for _, fileTime := range fileTimes {
		failures, err := sfg_utils.StreamFileNOVB(fileTime.Filename, uint8(antIndex), processor.Add)
		totalFailures += failures
		if err != nil {
			return fmt.Errorf("processing NOV770 file %s: %w", fileTime.Filename, err)
		}
	}
	if err := processor.Flush(); err != nil {
		return fmt.Errorf("flushing final NOV770 epoch: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("flushing NOV770 TileDB writer: %w", err)
	}
	log.Infof("processed %d epochs with %d failures", epochCount, totalFailures)
	log.Infof("processed %d files in %s", len(args), time.Since(startTime))
	return nil
}

var novab2tileCmd = &cobra.Command{
	Use:   "novab2tile [files...]",
	Short: "Write NovAtel binary observations to a TileDB array",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runNovab2tile,
}

func init() {
	novab2tileCmd.Flags().String("tdb", "", "path to the TileDB array (required)")
	novab2tileCmd.Flags().Int("procs", 1, "deprecated; observation decoding is chronological")
	novab2tileCmd.Flags().Int("antindex", 0, "antenna index to filter on (0 or 1)")
	novab2tileCmd.Flags().Int("batch-size", 10000, "number of chronologically tracked epochs per TileDB write")
}
