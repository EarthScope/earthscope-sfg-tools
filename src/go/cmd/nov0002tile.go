// Author: Franklyn Dunbar  | Contact franklyn.dunbar@earthscope.org | Dec 2024
package main

import (
	"context"
	"fmt"

	sfg_utils "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnsstiledb"
)

func runNov0002tile(cmd *cobra.Command, args []string) error {
	sfg_utils.LoadEnv()

	tdbPath, _ := cmd.Flags().GetString("tdb")
	numProcs, _ := cmd.Flags().GetInt("procs")
	tdbPositionPath, _ := cmd.Flags().GetString("tdbpos")
	batchSize, _ := cmd.Flags().GetInt("batch-size")

	if tdbPath == "" {
		return fmt.Errorf("--tdb is required")
	}
	if batchSize < 1 {
		return fmt.Errorf("--batch-size must be at least 1")
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

	if numProcs != 1 {
		log.Warn("--procs is ignored for NOV000 observation ingestion; chronological processing is required for continuous LLI tracking")
	}
	fileTimes, err := sfg_utils.SortFilesByFirstEpochNOV000(args)
	if err != nil {
		return fmt.Errorf("sorting NOV000 files: %w", err)
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
		insCompleteRecords, failures, err := sfg_utils.StreamFileNOV000(fileTime.Filename, processor.Add)
		totalFailures += failures
		if err != nil {
			return fmt.Errorf("processing NOV000 file %s: %w", fileTime.Filename, err)
		}
		if tdbPositionPath != "" && len(insCompleteRecords) > 0 {
			if err := sfg_utils.WriteINSPOSRecordToTileDB(tdbPositionPath, "us-east-2", insCompleteRecords); err != nil {
				return fmt.Errorf("writing INS records from %s: %w", fileTime.Filename, err)
			}
		}
	}
	if err := processor.Flush(); err != nil {
		return fmt.Errorf("flushing final NOV000 epoch: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("flushing NOV000 TileDB writer: %w", err)
	}
	log.Infof("processed %d GNSS epochs with %d failures", epochCount, totalFailures)
	return nil
}

var nov0002tileCmd = &cobra.Command{
	Use:   "nov0002tile [files...]",
	Short: "Write NovAtel 000-format observations to a TileDB array",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runNov0002tile,
}

func init() {
	nov0002tileCmd.Flags().String("tdb", "", "path to the TileDB GNSS array (required)")
	nov0002tileCmd.Flags().Int("procs", 1, "deprecated; observation decoding is chronological")
	nov0002tileCmd.Flags().String("tdbpos", "", "path to the TileDB position array")
	nov0002tileCmd.Flags().Int("batch-size", 10000, "number of chronologically tracked epochs per TileDB write")
}
