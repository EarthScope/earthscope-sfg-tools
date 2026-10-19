package main

import (
	"fmt"
	"path/filepath"
	"time"

	sfg_utils "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/sfg_utils"
	"github.com/apache/arrow-go/v18/arrow"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

func runNov2level1(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	outDir, _ := cmd.Flags().GetString("out")
	antenna, _ := cmd.Flags().GetUint8("antenna")
	if outDir == "" {
		return fmt.Errorf("--out is required")
	}

	file := args[0]
	var (
		epochs []observation.Epoch
		pva    []sfg_utils.InspvaaRecord
		std    []sfg_utils.INSSTDEVARecord
		fails  int
		err    error
	)
	switch format {
	case "nov000":
		epochs, pva, std, fails = sfg_utils.ScanFileNOV000(file, true)
	case "novb":
		epochs, fails, err = sfg_utils.ProcessFileNOVB(file, antenna, true)
	case "nova":
		epochs, fails, err = sfg_utils.ProcessFileNOVASCII(file, true)
	default:
		return fmt.Errorf("unknown --format %q (want nov000, novb or nova)", format)
	}
	if err != nil {
		return fmt.Errorf("parsing %s: %w", file, err)
	}
	log.Infof("%s: %d GNSS epochs, %d INSPVAA, %d INSSTDEVA, %d failed messages",
		file, len(epochs), len(pva), len(std), fails)

	batches := map[string]arrow.RecordBatch{
		"gnss_observations": sfg_utils.GNSSObservationsBatch(epochs, filepath.Base(file), time.Now()),
	}
	if format == "nov000" {
		batches["inspva"] = sfg_utils.INSPVABatch(pva)
		batches["insstdev"] = sfg_utils.INSSTDEVBatch(std)
	}
	for name, batch := range batches {
		path := filepath.Join(outDir, name+".arrow")
		err := sfg_utils.WriteArrowIPC(path, batch)
		batch.Release()
		if err != nil {
			return err
		}
	}
	return nil
}

var nov2level1Cmd = &cobra.Command{
	Use:   "nov2level1 [file]",
	Short: "Parse a NovAtel log into Level 1 Arrow IPC tables",
	Long: "Parse a NovAtel log into Arrow IPC files in --out, one per Level 1 table: " +
		"gnss_observations.arrow, plus inspva.arrow and insstdev.arrow for nov000. " +
		"Logs whose header time status is UNKNOWN (no time fix yet) are skipped.",
	Args: cobra.ExactArgs(1),
	RunE: runNov2level1,
}

func init() {
	nov2level1Cmd.Flags().String("format", "nov000", "input format: nov000 (NOV000.bin), novb (NovAtel binary, e.g. NOV770) or nova (NovAtel ASCII)")
	nov2level1Cmd.Flags().String("out", "", "output directory for the Arrow IPC files (required)")
	nov2level1Cmd.Flags().Uint8("antenna", 0, "measurement-source (antenna) index for novb")
}
