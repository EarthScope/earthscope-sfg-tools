package main

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
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

// level0RINEXCmd is sfg rinex --source iceberg with the scope given as
// --network, --station and --campaign.
var level0RINEXCmd = &cobra.Command{
	Use:   "rinex",
	Short: "Write the RINEX files of a gnss_observations stream from the catalog (sfg rinex --source iceberg)",
	Long: "Read the gnss_observations stream of --log-source from --type files in the " +
		"--network/--station/--campaign scope (optionally one GPS day, --day, 00:00-24:00 GPST) " +
		"and write it to the --out directory as daily RINEX observation files, as sfg rinex " +
		"--source iceberg does. --settings is a rinex.Settings JSON file for the header, as sfg rinex takes; " +
		"without it the marker name is the station. " +
		"--catalog is local (--warehouse is then a directory) or a catalog of the iceberg-go " +
		"configuration (.iceberg-go.yaml in $GOICEBERG_HOME or the home directory), whose " +
		"warehouse --warehouse overrides.",
	Args:         cobra.NoArgs,
	RunE:         runLevel0RINEX,
	SilenceUsage: true,
}

func runLevel0RINEX(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	r := rinexRun{source: "iceberg", scopeGiven: true}
	r.scope.Network, _ = f.GetString("network")
	r.scope.Station, _ = f.GetString("station")
	r.scope.Campaign, _ = f.GetString("campaign")
	r.fileType, _ = f.GetString("type")
	readRINEXFlags(f, &r)
	if err := edid.CheckScope(r.scope); err != nil {
		return &convert.UsageError{Err: err}
	}
	return r.run(cmd.Context())
}

func init() {
	level0Cmd.PersistentFlags().BoolP("verbose", "v", false, "log per-file parser details")

	f := level0RINEXCmd.Flags()
	addRINEXFlags(f, string(catalog.Local))
	f.String("network", "", "network of the stream's scope (required)")
	f.String("station", "", "station of the stream's scope (required)")
	f.String("campaign", "", "campaign of the stream's scope (required)")
	f.String("type", "", "file type of the stream, e.g. NOV000, NOV770 (required)")
	for _, name := range []string{"network", "station", "campaign", "type", "log-source"} {
		_ = level0RINEXCmd.MarkFlagRequired(name)
	}

	level0Cmd.AddCommand(level0RINEXCmd)
}
