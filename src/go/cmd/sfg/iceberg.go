package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
)

// The iceberg commands are convert's raw-to-Iceberg and Iceberg-to-files
// paths with the catalog's configuration filled in: the catalog defaults to
// the iceberg-go default-catalog, its warehouse to the configured one, and
// ingest tells each raw file's type by its name.

var icebergCmd = &cobra.Command{
	Use:   "iceberg",
	Short: "Ingest raw files into, or export tables from, an Iceberg catalog",
	Long: "Ingest raw files into the Level 0 tables of an Iceberg catalog, or export a table's rows to files. " +
		"--catalog is local (a warehouse directory with an SQLite catalog, given by --warehouse) or a catalog " +
		"of the iceberg-go configuration (.iceberg-go.yaml in $GOICEBERG_HOME or the home directory), by default " +
		"its default-catalog; --warehouse overrides the configured warehouse. Cloud credentials and the AWS " +
		"region come from each cloud's default chain. Use sfg convert to state every part of a conversion.",
}

var icebergIngestCmd = &cobra.Command{
	Use:   "ingest --scope NET/STA/CAMP FILE|GLOB|DIR...",
	Short: "Parse raw files into the Level 0 tables of an Iceberg catalog",
	Long: "Parse raw files of any type through the Level 0 parsers, stamped with --scope, and append them to the " +
		"catalog, creating missing tables. Each file's type is told by its name (e.g. *NOV000.bin, *DFOP00.raw); " +
		"a file whose name matches no type is listed as skipped. A directory is read as a campaign raw " +
		"directory (QC PIN files under ../qc/). --source-type limits the types read. Files are parsed --workers " +
		"at a time, and up to --files-per-snapshot of them written as one snapshot, in which each file replaces " +
		"its own streams' rows over its own time span, so re-running doesn't add duplicates. A file that " +
		"overlaps one already in the snapshot starts the next, so the later file wins as if written alone.\n\n" +
		"Exit codes: 0 ok, 1 fatal, 2 usage, 3 partial (some files failed).",
	Args:         cobra.MinimumNArgs(1),
	RunE:         runIcebergIngest,
	SilenceUsage: true,
}

var icebergExportCmd = &cobra.Command{
	Use:   "export --scope NET[/STA[/CAMP]] --table TABLE --out DIR",
	Short: "Export rows of Iceberg tables to parquet, csv or arrow files",
	Long: "Write the rows --scope selects from each --table (a name such as inspva, or namespace.name such as " +
		"gnssa_l1.garpos when the name exists at more than one level) to <table>.<format> files in --out. " +
		"--source-type and --log-source pick a file type or one stream and need the full scope; in Level 1/2 " +
		"tables --source-type is the product and --log-source its input. --start/--end bound the table's time " +
		"column, [start, end), as a date (UTC midnight) or RFC 3339 time.\n\n" +
		"Exit codes: 0 ok, 1 fatal, 2 usage.",
	Args:         cobra.NoArgs,
	RunE:         runIcebergExport,
	SilenceUsage: true,
}

func runIcebergIngest(cmd *cobra.Command, args []string) error {
	f := cmd.Flags()
	s := convert.Spec{Source: convert.SourceRaw, SourceURIs: args, Sink: convert.SinkIceberg, TypesFromNames: true}
	s.SinkCatalog, _ = f.GetString("catalog")
	s.SinkURI, _ = f.GetString("warehouse")
	s.SourceTypes, _ = f.GetStringSlice("source-type")
	s.Tables, _ = f.GetStringSlice("table")
	s.LogSource, _ = f.GetString("log-source")
	s.MaxFiles, _ = f.GetInt("max-files")
	s.FilesPerCommit, _ = f.GetInt("files-per-commit")
	s.FilesPerSnapshot, _ = f.GetInt("files-per-snapshot")
	s.Workers, _ = f.GetInt("workers")
	err := required(f, "scope")
	if err == nil {
		err = readScope(f, &s)
	}
	return runSpec(cmd, s, err)
}

func runIcebergExport(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	s := convert.Spec{Source: convert.SourceIceberg, Sink: convert.SinkFiles}
	s.SourceCatalog, _ = f.GetString("catalog")
	if wh, _ := f.GetString("warehouse"); wh != "" {
		s.SourceURIs = []string{wh}
	}
	s.SinkURI, _ = f.GetString("out")
	s.TableFormat, _ = f.GetString("format")
	s.Tables, _ = f.GetStringSlice("table")
	s.SourceTypes, _ = f.GetStringSlice("source-type")
	s.LogSource, _ = f.GetString("log-source")
	s.Columns, _ = f.GetStringSlice("columns")
	err := required(f, "scope", "table", "out")
	if err == nil {
		err = readScope(f, &s)
	}
	for _, b := range []struct {
		flag string
		t    *time.Time
	}{{"start", &s.Start}, {"end", &s.End}} {
		if v, _ := f.GetString(b.flag); v != "" && err == nil {
			if *b.t, err = parseTime(v); err != nil {
				err = &convert.UsageError{Err: fmt.Errorf("--%s: %w", b.flag, err)}
			}
		}
	}
	return runSpec(cmd, s, err)
}

// required is a usage error naming the flags of names not given.
func required(f *pflag.FlagSet, names ...string) error {
	var missing []string
	for _, n := range names {
		if !f.Changed(n) {
			missing = append(missing, "--"+n)
		}
	}
	if len(missing) > 0 {
		return &convert.UsageError{Err: fmt.Errorf("%s needs %s", f.Name(), strings.Join(missing, ", "))}
	}
	return nil
}

func readScope(f *pflag.FlagSet, s *convert.Spec) error {
	scope, _ := f.GetString("scope")
	var err error
	s.Scope, err = convert.ParseScope(scope)
	return err
}

func init() {
	for _, c := range []*cobra.Command{icebergIngestCmd, icebergExportCmd} {
		f := c.Flags()
		f.String("catalog", "", "catalog: local, or a catalog of the iceberg-go configuration (default: its default-catalog)")
		f.String("warehouse", "", "warehouse: overrides the catalog's configured one; a directory for local")
		f.String("scope", "", "NET/STA/CAMP (ingest), NET[/STA[/CAMP]] (export)")
		f.StringSlice("source-type", nil, "file types (ingest: limits the types read), or for export one file type or product")
		f.StringSlice("table", nil, "tables: name or namespace.name, comma-separated (ingest default: every table the files feed)")
		f.String("log-source", "", "only this log source's rows (export: or product input for Level 1/2 tables)")
		f.String("result", "", "write the JSON result here, or - for standard output")
	}
	fi := icebergIngestCmd.Flags()
	fi.Int("max-files", 0, "parse at most this many files of each type (0 = all)")
	fi.Int("files-per-commit", 0, "commit a table's snapshots this many at a time (0 = all at once)")
	fi.Int("files-per-snapshot", 32, "merge up to this many files into one snapshot, each replacing only its own span")
	fi.Int("workers", 0, "files parsed at once (0 = one per CPU)")

	fe := icebergExportCmd.Flags()
	fe.String("out", "", "output directory")
	fe.String("format", convert.FormatParquet, "file format: parquet, csv or arrow")
	fe.String("start", "", "earliest time, inclusive: YYYY-MM-DD or RFC 3339")
	fe.String("end", "", "latest time, exclusive: YYYY-MM-DD or RFC 3339")
	fe.StringSlice("columns", nil, "columns to write, in this order (default: all)")

	icebergCmd.AddCommand(icebergIngestCmd, icebergExportCmd)
}
