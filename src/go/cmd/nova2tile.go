// Author: Franklyn Dunbar  | Contact franklyn.dunbar@earthscope.org | Dec 2024
package main

import (
	"context"
	"fmt"
	"sync"

	sfg_utils "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/geodata/gnsstiledb"
)


func runNova2tile(cmd *cobra.Command, args []string) error {
	sfg_utils.LoadEnv()

	tdbPath, _ := cmd.Flags().GetString("tdb")
	numProcs, _ := cmd.Flags().GetInt("procs")

	if tdbPath == "" {
		return fmt.Errorf("--tdb is required")
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

	var wg sync.WaitGroup
	sem := make(chan struct{}, numProcs)

	for _, filename := range args {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			epochs, failCounter, err := sfg_utils.ProcessFileNOVASCII(filename)
			if err != nil {
				log.Errorf("error processing file %s: %v", filename, err)
				return
			}
			if len(epochs) == 0 {
				log.Warnf("no epochs found in file %s", filename)
				return
			}
			log.Infof("processed %d epochs from file %s", len(epochs), filename)
			log.Infof("Total Attempts: %d, Successes: %d, Failures: %d", len(epochs)+failCounter, len(epochs), failCounter)

			if err := client.WriteObservations(ctx, tdbPath, epochs); err != nil {
				log.Errorf("error writing epochs to array: %v", err)
			}
			epochs = nil
		}(filename)
	}
	wg.Wait()
	return nil
}

var nova2tileCmd = &cobra.Command{
	Use:   "nova2tile [files...]",
	Short: "Write NovAtel ASCII observations to a TileDB array",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runNova2tile,
}

func init() {
	nova2tileCmd.Flags().String("tdb", "", "path to the TileDB array (required)")
	nova2tileCmd.Flags().Int("procs", 10, "number of concurrent processes")
}
