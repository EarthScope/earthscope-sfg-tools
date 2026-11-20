package main

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/iceberg"
)

var icebergBuildCmd = &cobra.Command{
	Use:   "build [--catalog NAME] [--warehouse DIR|URI] [--database NAME...]",
	Short: "Create the SFG databases and tables in a catalog",
	Long: "Create every database and table the SFG catalog definition declares (datamodels.SFG: " +
		"gnssa_l0, gnssa_l1, gnssa_l2), or only those of --database, in the catalog --catalog names: " +
		"local (a warehouse directory with an SQLite catalog, given by --warehouse) or a catalog of the " +
		"iceberg-go configuration, such as AWS Glue. Missing databases and tables are created and " +
		"existing tables gain columns their definitions added; nothing is dropped, so running it again " +
		"changes nothing.",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runIcebergBuild,
}

func runIcebergBuild(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	name, _ := f.GetString("catalog")
	warehouse, _ := f.GetString("warehouse")
	names, _ := f.GetStringSlice("database")
	def := datamodels.SFG
	if len(names) > 0 {
		def.Databases = nil
		for _, n := range names {
			db, ok := datamodels.SFG.Database(n)
			if !ok {
				return &convert.UsageError{Err: fmt.Errorf("--database %q: not an SFG database", n)}
			}
			def.Databases = append(def.Databases, db)
		}
	}
	cfg, err := catalog.ParseConfig(name, warehouse)
	if err != nil {
		return &convert.UsageError{Err: fmt.Errorf("--catalog: %w", err)}
	}
	prefix, _ := f.GetString("database-prefix")
	ice, err := iceberg.New(cmd.Context(), cfg, def, iceberg.Create(), iceberg.DatabasePrefix(prefix))
	if err != nil {
		return err
	}
	defer ice.Close()
	cfg = cfg.WithDatabasePrefix(prefix)
	for _, db := range def.Databases {
		slog.Info("built database", "catalog", cfg.String(), "database", cfg.Namespace(db.Name), "tables", len(db.Tables))
	}
	return nil
}

func init() {
	f := icebergBuildCmd.Flags()
	f.String("catalog", "", "catalog: local, glue, or a catalog of the iceberg-go configuration (default: its default-catalog)")
	f.String("warehouse", "", "warehouse: a directory for local, an S3 prefix for glue (s3://bucket/prefix), else overrides the catalog's configured one")
	f.String("database-prefix", "", "prefix of the catalog's database names, e.g. dev_ builds dev_gnssa_l0 (development catalogs; default none)")
	f.StringSlice("database", nil, "databases to build, comma-separated (default: all of gnssa_l0, gnssa_l1, gnssa_l2)")
	icebergCmd.AddCommand(icebergBuildCmd)
}
