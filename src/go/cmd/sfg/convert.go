package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Move rows from one source to one sink, every part stated explicitly",
	Long: "Read rows from --source at --source-uri and write them to --sink at --sink-uri. " +
		"Nothing is inferred: scope, tables, catalog types and formats are all given as flags, " +
		"and a flag the chosen source and sink don't take is an error.\n\n" +
		"--source iceberg reads --table (a name such as inspva, or namespace.name such as " +
		"gnssa_l2.garpos when the name exists at more than one level) from the --source-catalog " +
		"warehouse at --source-uri (a directory for sql, s3://... for glue). --scope NET[/STA[/CAMP]] " +
		"bounds the rows. --source-type and --log-source pick a file type (all its log sources) " +
		"or one stream, and need the full scope; --log-source needs --source-type. " +
		"In Level 2/3 tables --source-type is the product (PRIDE, SMOOTHED, GNSSA) and --log-source " +
		"its input, e.g. NOV000:RANGEA. --start/--end bound the table's time column, [start, end), " +
		"as a date (UTC midnight) or RFC 3339 time; for gnss_observations that column is GPS time.\n\n" +
		"--source raw parses raw files of each --source-type through the Level 1 parsers, stamped " +
		"with the full --scope NET/STA/CAMP. Each --source-uri is a file or glob (one --source-type), " +
		"or a campaign raw directory, where each type's files are found by its filename pattern " +
		"(QC PIN files under ../qc/). --source-uri can be repeated, and arguments after the flags are " +
		"more source URIs, so a shell glob such as --source-uri raw/*DFOP00.raw names every match. " +
		"--max-files caps the files of each type. --table picks tables " +
		"(default: every table the types feed) and --log-source one log source's rows. A file that " +
		"fails to parse is reported and skipped.\n\n" +
		"--sink files writes <table>.<table-format> files into the --sink-uri directory.\n\n" +
		"--sink iceberg appends to the --sink-catalog warehouse at --sink-uri (a directory for sql, " +
		"s3://... for glue), creating missing tables. Each parsed file is one snapshot that replaces " +
		"its streams' rows over its time span, so re-running doesn't add duplicates. From --source " +
		"iceberg, the selected rows are copied one stream and one hour at a time, each hour " +
		"replacing that stream's rows over its span: a sync of what was selected, not a mirror. " +
		"A table's snapshots are committed to the catalog together, or --files-per-commit N at a " +
		"time (a copy's hours count as files); a failed commit loses only its own snapshots. " +
		"--columns doesn't apply, and the same warehouse on both sides is an error.\n\n" +
		"--result PATH (or - for standard output) writes a JSON record of the run. Exit codes: " +
		"0 ok, 1 fatal, 2 usage, 3 partial.",
	Args:         cobra.ArbitraryArgs, // more source URIs
	RunE:         runConvert,
	SilenceUsage: true, // a failed run isn't a usage problem; usage errors say what's wrong

}

func runConvert(cmd *cobra.Command, args []string) error {
	resultPath, _ := cmd.Flags().GetString("result")
	var res *convert.Result
	spec, err := convertSpec(cmd.Flags(), args)
	if err == nil {
		var c *convert.Conversion
		if c, err = convert.New(spec); err == nil {
			res, err = c.Run(cmd.Context())
		}
	}
	if res == nil {
		res = convert.UsageResult(err)
	}
	for _, e := range res.Errors {
		slog.Error(e.Message, "stage", e.Stage, "subject", e.Subject)
	}
	for _, o := range res.Outputs {
		slog.Info("wrote", "kind", o.Kind, "path", o.Path, "table", o.Table, "rows", o.Rows)
	}
	if resultPath != "" {
		if werr := writeResult(resultPath, res); werr != nil {
			slog.Error("writing result", "path", resultPath, "error", werr)
			if err == nil {
				err = werr
			}
		}
	}
	if code := convert.ExitCode(res, err); code != convert.ExitOK {
		return exitError{code}
	}
	return nil
}

// convertSpec checks which flags were given, then reads them and args
// (more source URIs) into a Spec.
func convertSpec(f *pflag.FlagSet, args []string) (convert.Spec, error) {
	var given []string
	f.Visit(func(fl *pflag.Flag) { given = append(given, fl.Name) })
	var s convert.Spec
	s.Source, _ = f.GetString("source")
	s.Sink, _ = f.GetString("sink")
	if err := convert.CheckFlags(s.Source, s.Sink, given); err != nil {
		return s, err
	}
	s.SourceURIs, _ = f.GetStringArray("source-uri")
	s.SourceURIs = append(s.SourceURIs, args...)
	s.SourceCatalog, _ = f.GetString("source-catalog")
	s.SinkURI, _ = f.GetString("sink-uri")
	s.SinkCatalog, _ = f.GetString("sink-catalog")
	s.TableFormat, _ = f.GetString("table-format")
	s.Region, _ = f.GetString("region")
	s.Tables, _ = f.GetStringSlice("table")
	s.SourceTypes, _ = f.GetStringSlice("source-type")
	s.LogSource, _ = f.GetString("log-source")
	s.Columns, _ = f.GetStringSlice("columns")
	s.MaxFiles, _ = f.GetInt("max-files")
	s.FilesPerCommit, _ = f.GetInt("files-per-commit")
	scope, _ := f.GetString("scope")
	var err error
	if s.Scope, err = convert.ParseScope(scope); err != nil {
		return s, err
	}
	for _, b := range []struct {
		flag string
		t    *time.Time
	}{{"start", &s.Start}, {"end", &s.End}} {
		v, _ := f.GetString(b.flag)
		if v == "" {
			continue
		}
		if *b.t, err = parseTime(v); err != nil {
			return s, &convert.UsageError{Err: fmt.Errorf("--%s: %w", b.flag, err)}
		}
	}
	return s, nil
}

// parseTime parses a date (UTC midnight) or an RFC 3339 time.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func writeResult(path string, res *convert.Result) error {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func init() {
	f := convertCmd.Flags()
	f.String("source", "", "source kind: iceberg or raw")
	f.StringArray("source-uri", nil, "source location: an Iceberg warehouse, a directory (sql) or s3:// URI (glue); raw files: a file, glob or directory (repeatable)")
	f.String("source-catalog", "", "source catalog type: sql or glue")
	f.String("sink", "", "sink kind: files or iceberg")
	f.String("sink-uri", "", "sink location: the output directory; or an Iceberg warehouse, a directory (sql) or s3:// URI (glue)")
	f.String("sink-catalog", "", "sink catalog type: sql or glue")
	f.String("table-format", "", "files sink format: parquet, csv or arrow")
	f.String("scope", "", "NET[/STA[/CAMP]]")
	f.StringSlice("table", nil, "tables: name or namespace.name, comma-separated")
	f.StringSlice("source-type", nil, "raw file types, comma-separated; for iceberg, one file type, or product for Level 2/3 tables (needs the full scope)")
	f.Int("max-files", 0, "raw: parse at most this many files of each type (0 = all)")
	f.Int("files-per-commit", 0, "iceberg sink: commit a table's snapshots this many at a time (0 = all at once)")
	f.String("log-source", "", "log source, or product input for Level 2/3 tables")
	f.String("start", "", "earliest time, inclusive: YYYY-MM-DD or RFC 3339")
	f.String("end", "", "latest time, exclusive: YYYY-MM-DD or RFC 3339")
	f.StringSlice("columns", nil, "columns to write, in this order (default: all)")
	f.String("region", "", "AWS region for glue (default: AWS config)")
	f.String("result", "", "write the JSON result here, or - for standard output")
}
