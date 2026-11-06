package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

var level1Cmd = &cobra.Command{
	Use:   "level1",
	Short: "Write RINEX from the Level 1 catalog",
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		if v, _ := cmd.Flags().GetBool("verbose"); v {
			slog.SetLogLoggerLevel(slog.LevelDebug)
		}
	},
}

var level1RINEXCmd = &cobra.Command{
	Use:   "rinex",
	Short: "Write one day of a gnss_observations stream from the catalog as RINEX",
	Long: "Read the gnss_observations stream of --log-source from --type files in the " +
		"--network/--station/--campaign scope for one GPS day (--day, 00:00-24:00 GPST) " +
		"and write it to --out as RINEX observations. --settings is a rinex.Settings JSON " +
		"file for the header, as nov0002rnx takes; without it the marker name is the station.",
	Args: cobra.NoArgs,
	RunE: runLevel1RINEX,
}

func runLevel1RINEX(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	var scope datamodels.Scope
	scope.Network, _ = f.GetString("network")
	scope.Station, _ = f.GetString("station")
	scope.Campaign, _ = f.GetString("campaign")
	fileType, _ := f.GetString("type")
	logSource, _ := f.GetString("log-source")
	dayStr, _ := f.GetString("day")
	settingsPath, _ := f.GetString("settings")
	outPath, _ := f.GetString("out")
	catalogType, _ := f.GetString("catalog")
	region, _ := f.GetString("region")
	warehouse, _ := f.GetString("warehouse")

	day, err := time.Parse(time.DateOnly, dayStr)
	if err != nil {
		return fmt.Errorf("--day: %w", err)
	}
	settings := rinex.NewSettings()
	settings.MarkerName = scope.Station
	if settingsPath != "" {
		b, err := os.ReadFile(settingsPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, settings); err != nil {
			return fmt.Errorf("parsing %s: %w", settingsPath, err)
		}
	}

	ctx := cmd.Context()
	cat, err := catalog.Open(ctx, catalog.Config{Type: catalogType, Warehouse: warehouse, Region: region})
	if err != nil {
		return err
	}
	defer cat.Close()
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	n, err := cat.WriteRINEX(ctx, w, scope, fileType, logSource, day, settings)
	if err == nil {
		err = w.Flush()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(outPath)
		return err
	}
	slog.Info("wrote RINEX", "file", outPath, "epochs", n,
		"stream", datamodels.StreamEDID(scope, fileType, logSource), "day", dayStr)
	return nil
}

func init() {
	level1Cmd.PersistentFlags().BoolP("verbose", "v", false, "log per-file parser details")

	f := level1RINEXCmd.Flags()
	f.String("warehouse", "", "Iceberg warehouse: local directory (sql) or s3:// URI (glue) (required)")
	f.String("catalog", catalog.TypeSQL, "catalog type: sql (local) or glue")
	f.String("region", "", "AWS region for --catalog glue (default: AWS config)")
	f.String("network", "", "network of the stream's scope (required)")
	f.String("station", "", "station of the stream's scope (required)")
	f.String("campaign", "", "campaign of the stream's scope (required)")
	f.String("type", "", "file type of the stream, e.g. NOV000, NOV770 (required)")
	f.String("log-source", "", "log source of the stream, e.g. RANGEA, RANGECMP5B_1 (required)")
	f.String("day", "", "GPS day, YYYY-MM-DD (required)")
	f.String("settings", "", "rinex.Settings JSON file for the header")
	f.String("out", "", "output RINEX file (required)")
	for _, name := range []string{"warehouse", "network", "station", "campaign", "type", "log-source", "day", "out"} {
		_ = level1RINEXCmd.MarkFlagRequired(name)
	}

	level1Cmd.AddCommand(level1RINEXCmd)
}
