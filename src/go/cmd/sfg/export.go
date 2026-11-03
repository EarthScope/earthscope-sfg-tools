package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/EarthScope/es_sfgtools/src/golangtools/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export rows from the Iceberg catalog",
}

var exportTableCmd = &cobra.Command{
	Use:   "table",
	Short: "Write a table's rows for a scope, stream and time window to Parquet, CSV or Arrow",
	Long: "Write the rows of --table (a name such as inspva, or namespace.name such as " +
		"gnssa_l2.garpos when the name exists at more than one level) in the " +
		"--network[/--station[/--campaign]] scope to --out. --type and --log-source pick " +
		"streams: a file type (all its log sources) or one stream, and need the full scope; " +
		"--log-source alone matches the Log_Source column. In Level 2/3 tables --type is the " +
		"product (PRIDE, SMOOTHED, GNSSA) and --log-source its input, e.g. NOV000:RANGEA. " +
		"--start/--end bound the table's time column, [start, end), as a date (UTC midnight) " +
		"or RFC 3339 time; for gnss_observations that column is time_gpst, GPS time. " +
		"--format defaults to --out's extension; --out - writes to standard output.",
	Args: cobra.NoArgs,
	RunE: runExportTable,
}

func runExportTable(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	tableName, _ := f.GetString("table")
	var sel catalog.Selection
	sel.Scope.Network, _ = f.GetString("network")
	sel.Scope.Station, _ = f.GetString("station")
	sel.Scope.Campaign, _ = f.GetString("campaign")
	sel.FileType, _ = f.GetString("type")
	sel.LogSource, _ = f.GetString("log-source")
	sel.Columns, _ = f.GetStringSlice("columns")
	format, _ := f.GetString("format")
	outPath, _ := f.GetString("out")
	catalogType, _ := f.GetString("catalog")
	region, _ := f.GetString("region")
	warehouse, _ := f.GetString("warehouse")

	def, err := datamodels.TableByName(tableName)
	if err != nil {
		return err
	}
	for _, b := range []struct {
		flag string
		t    *time.Time
	}{{"start", &sel.Start}, {"end", &sel.End}} {
		s, _ := f.GetString(b.flag)
		if s == "" {
			continue
		}
		if *b.t, err = parseTime(s); err != nil {
			return fmt.Errorf("--%s: %w", b.flag, err)
		}
	}
	if format == "" {
		format = strings.TrimPrefix(filepath.Ext(outPath), ".")
		if format == "" {
			return fmt.Errorf("--format is required when --out has no parquet, csv or arrow extension")
		}
	}
	// Check the selection before creating the output file.
	if _, err := sel.Filter(def); err != nil {
		return err
	}

	ctx := cmd.Context()
	cat, err := catalog.Open(ctx, catalog.Config{Type: catalogType, Warehouse: warehouse, Region: region})
	if err != nil {
		return err
	}
	defer cat.Close()

	var out io.Writer = os.Stdout
	var file *os.File
	if outPath != "-" {
		if file, err = os.Create(outPath); err != nil {
			return err
		}
		out = file
	}
	w := bufio.NewWriterSize(out, 1<<20)
	n, err := cat.Export(ctx, w, def, sel, format)
	if err == nil {
		err = w.Flush()
	}
	if file != nil {
		if cerr := file.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(outPath)
		}
	}
	if err != nil {
		return err
	}
	slog.Info("exported", "table", def.Namespace()+"."+def.Name, "rows", n, "format", format, "out", outPath)
	return nil
}

// parseTime parses a date (UTC midnight) or an RFC 3339 time.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func init() {
	f := exportTableCmd.Flags()
	f.String("warehouse", "", "Iceberg warehouse: local directory (sql) or s3:// URI (glue) (required)")
	f.String("catalog", catalog.TypeSQL, "catalog type: sql (local) or glue")
	f.String("region", "", "AWS region for --catalog glue (default: AWS config)")
	f.String("table", "", "table name, or namespace.name (required)")
	f.String("network", "", "network (required)")
	f.String("station", "", "station")
	f.String("campaign", "", "campaign (needs --station)")
	f.String("type", "", "file type, or product for Level 2/3 tables (needs the full scope)")
	f.String("log-source", "", "log source, or product input for Level 2/3 tables")
	f.String("start", "", "earliest time, inclusive: YYYY-MM-DD or RFC 3339")
	f.String("end", "", "latest time, exclusive: YYYY-MM-DD or RFC 3339")
	f.StringSlice("columns", nil, "columns to write (default: all)")
	f.String("format", "", "parquet, csv or arrow (default: --out's extension)")
	f.String("out", "", "output file, or - for standard output (required)")
	for _, name := range []string{"warehouse", "table", "network", "out"} {
		_ = exportTableCmd.MarkFlagRequired(name)
	}
	exportCmd.AddCommand(exportTableCmd)
}
