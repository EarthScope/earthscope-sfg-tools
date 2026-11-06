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

var level0Cmd = &cobra.Command{
	Use:   "level0",
	Short: "Write RINEX from the Level 0 catalog",
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		if v, _ := cmd.Flags().GetBool("verbose"); v {
			slog.SetLogLoggerLevel(slog.LevelDebug)
		}
	},
}

var level0RINEXCmd = &cobra.Command{
	Use:   "rinex",
	Short: "Write one day of a gnss_observations stream from the catalog as RINEX",
	Long: "Read the gnss_observations stream of --log-source from --type files in the " +
		"--network/--station/--campaign scope for one GPS day (--day, 00:00-24:00 GPST) " +
		"and write it to --out as RINEX observations. --settings is a rinex.Settings JSON " +
		"file for the header, as nov0002rnx takes; without it the marker name is the station. " +
		"--catalog is local (--warehouse is then a directory) or a catalog of the iceberg-go " +
		"configuration (.iceberg-go.yaml in $GOICEBERG_HOME or the home directory), whose " +
		"warehouse --warehouse overrides.",
	Args: cobra.NoArgs,
	RunE: runLevel0RINEX,
}

func runLevel0RINEX(cmd *cobra.Command, _ []string) error {
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
	catalogName, _ := f.GetString("catalog")
	warehouse, _ := f.GetString("warehouse")

	if err := datamodels.CheckScope(scope); err != nil {
		return err
	}
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
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalogName, Warehouse: warehouse})
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
	level0Cmd.PersistentFlags().BoolP("verbose", "v", false, "log per-file parser details")

	f := level0RINEXCmd.Flags()
	f.String("warehouse", "", "Iceberg warehouse: the directory for --catalog local (required then), else overrides the catalog's")
	f.String("catalog", catalog.Local, "catalog: local, or a catalog of the iceberg-go configuration")
	f.String("network", "", "network of the stream's scope (required)")
	f.String("station", "", "station of the stream's scope (required)")
	f.String("campaign", "", "campaign of the stream's scope (required)")
	f.String("type", "", "file type of the stream, e.g. NOV000, NOV770 (required)")
	f.String("log-source", "", "log source of the stream, e.g. RANGEA, RANGECMP5B_1 (required)")
	f.String("day", "", "GPS day, YYYY-MM-DD (required)")
	f.String("settings", "", "rinex.Settings JSON file for the header")
	f.String("out", "", "output RINEX file (required)")
	for _, name := range []string{"network", "station", "campaign", "type", "log-source", "day", "out"} {
		_ = level0RINEXCmd.MarkFlagRequired(name)
	}

	level0Cmd.AddCommand(level0RINEXCmd)
}
