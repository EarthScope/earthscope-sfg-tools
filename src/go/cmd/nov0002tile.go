// Author: Franklyn Dunbar  | Contact franklyn.dunbar@earthscope.org | Dec 2024
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"

	sfg_utils "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/geodata/gnsstiledb"
)

type nov000Reader struct {
	Reader *bufio.Reader
}

func newNov000Reader(r io.Reader) nov000Reader {
	return nov000Reader{Reader: bufio.NewReader(r)}
}

func (reader nov000Reader) NextMessage() (message novatelascii.Message, err error) {
	message, err = sfg_utils.DeserializeNOV00bin(reader.Reader)
	if err != nil {
		if err == io.EOF {
			return message, err
		}
	}
	return message, nil
}


func runNov0002tile(cmd *cobra.Command, args []string) error {
	sfg_utils.LoadEnv()

	tdbPath, _ := cmd.Flags().GetString("tdb")
	numProcs, _ := cmd.Flags().GetInt("procs")
	tdbPositionPath, _ := cmd.Flags().GetString("tdbpos")

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

			epochs, insCompleteRecords, fails := sfg_utils.ProcessFileNOV000(filename)
			if len(epochs) == 0 {
				log.Warnf("no GNSS epochs found in file %s", filename)
				return
			}
			if len(insCompleteRecords) == 0 {
				log.Warnf("no INS records found in file %s", filename)
				return
			}
			if fails > 0 {
				log.Warnf("failed to process %d records in file %s", fails, filename)
			}
			log.Infof("Writing %d GNSS epochs from file %s to TileDB array %s", len(epochs), filename, tdbPath)
			log.Infof("Total Attempts: %d, Successes: %d, Failures: %d", len(epochs)+fails, len(epochs), fails)

			if err := client.WriteObservations(ctx, tdbPath, epochs); err != nil {
				log.Errorf("error writing epochs to array: %v", err)
			}
			if tdbPositionPath != "" {
				log.Infof("writing %d INS position records from file %s to TileDB array %s", len(insCompleteRecords), filename, tdbPositionPath)
				if err := sfg_utils.WriteINSPOSRecordToTileDB(tdbPositionPath, "us-east-2", insCompleteRecords); err != nil {
					log.Errorf("error writing INS position records to array: %v", err)
				}
			}
		}(filename)
	}
	wg.Wait()
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
	nov0002tileCmd.Flags().Int("procs", 10, "number of concurrent processes")
	nov0002tileCmd.Flags().String("tdbpos", "", "path to the TileDB position array")
	
}
