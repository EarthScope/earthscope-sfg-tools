package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	gnssrinex "gitlab.com/earthscope/gnsstools/codecs/rinex"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/iceberg"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/raw"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/rinex"
)

var rinexCmd = &cobra.Command{
	Use:   "rinex [files...]",
	Short: "Write daily RINEX observation files from raw NovAtel files or an Iceberg catalog",
	Long: "Reads the gnss_observations of one stream and writes one RINEX observation file per day to --out, named by RINEX 3 convention. " +
		"--source raw (the default) decodes the raw files given as arguments, all of one --type (NOV000, NOV770, NVA or QCPIN); " +
		"--source iceberg reads the Level 0 gnss_observations table of the --catalog/--warehouse catalog for --scope. " +
		"Epochs from all files are merged, ordered and de-duplicated by time, cycle slips are flagged from the receiver lock times, then epochs are optionally decimated (slips on dropped epochs carry to the next kept one). " +
		"Logs with no time fix (status UNKNOWN or GPS week 0) are dropped. " +
		"A RINEX file is one antenna's stream: when the files or the scope hold several (NOV770: RANGECMPB, RANGECMP5B, and _1 for the second antenna) " +
		"pick one with --type and --log-source, or the command exits 2 naming the streams found and writes nothing.\n\n" +
		"Exit codes: 0 ok, 1 fatal, 2 usage (including several streams), 3 partial (some files failed).",
	Args:         cobra.ArbitraryArgs,
	RunE:         runRINEX,
	SilenceUsage: true,
}

// rinexRun is what a RINEX command asks for.
type rinexRun struct {
	source       string
	files        []string // raw
	fileType     string
	logSource    string // the name within fileType
	scope        edid.Scope
	scopeGiven   bool
	catalog      string // iceberg
	warehouse    string
	prefix       string
	settingsPath string
	modulo       int64
	out          string
	day          string // iceberg: one GPS day
}

func init() {
	f := rinexCmd.Flags()
	addRINEXFlags(f, "")
	f.String("source", "raw", "where the observations come from: raw (the files given as arguments) or iceberg (a catalog)")
	f.String("type", "", "file type of the stream: NOV000, NOV770, NVA or QCPIN (raw: required)")
	f.String("scope", "", "NET/STA/CAMP of the stream (iceberg: required; raw: names the rows, default none; NET[/STA[/CAMP]] needs the full scope with --log-source)")
	rootCmd.AddCommand(rinexCmd)
}

// addRINEXFlags adds the flags sfg rinex and sfg level0 rinex share.
func addRINEXFlags(f *pflag.FlagSet, defaultCatalog string) {
	f.String("log-source", "", "log source of --type to write, e.g. RANGEA, RANGECMP5B, RANGECMP5B_1 (default: the only one found)")
	f.String("settings", "", "rinex.Settings JSON file for the header: required for --source raw; for --source iceberg the marker defaults to the station")
	f.Int64("modulo", 0, "decimation interval in milliseconds, e.g. 1000 for 1 Hz, 15000 for 15 s; 0 keeps every epoch")
	f.String("out", ".", "output directory")
	f.String("day", "", "iceberg: only this GPS day, YYYY-MM-DD (00:00-24:00 GPST)")
	f.String("catalog", defaultCatalog, "iceberg: catalog: local, glue, or a catalog of the iceberg-go configuration")
	f.String("warehouse", "", "iceberg: warehouse: the directory for --catalog local (required then), the S3 prefix for glue, else overrides the catalog's")
	f.String("database-prefix", "", "iceberg: prefix of the catalog's database names, e.g. dev_ for dev_gnssa_l0 (default none)")
}

func runRINEX(cmd *cobra.Command, files []string) error {
	f := cmd.Flags()
	r := rinexRun{files: files}
	r.source, _ = f.GetString("source")
	r.fileType, _ = f.GetString("type")
	scope, _ := f.GetString("scope")
	if scope != "" {
		var err error
		if r.scope, err = convert.ParseScope(scope); err != nil {
			return err
		}
		r.scopeGiven = true
	}
	readRINEXFlags(f, &r)
	return r.run(cmd.Context())
}

func readRINEXFlags(f *pflag.FlagSet, r *rinexRun) {
	r.logSource, _ = f.GetString("log-source")
	r.settingsPath, _ = f.GetString("settings")
	r.modulo, _ = f.GetInt64("modulo")
	r.out, _ = f.GetString("out")
	r.day, _ = f.GetString("day")
	r.catalog, _ = f.GetString("catalog")
	r.warehouse, _ = f.GetString("warehouse")
	r.prefix, _ = f.GetString("database-prefix")
}

// run checks the request, connects a Reader to a RINEX Writer with
// convert.Run and reports. A Writer conflict (several streams) is a usage
// error; otherwise the exit code is convert.ExitCode's.
func (r rinexRun) run(ctx context.Context) error {
	rdr, sel, err := r.reader(ctx)
	if err != nil {
		return err
	}
	if cl, ok := rdr.(io.Closer); ok {
		defer cl.Close()
	}
	settings, err := r.settings()
	if err != nil {
		return err
	}
	w := rinex.NewWriter(r.out, settings, r.modulo)
	res, err := convert.Run(ctx, rdr, w, sel)
	if res == nil {
		return err // the Writer refused a table: a usage error
	}
	if conflict := w.Conflict(); errors.Is(conflict, rinex.ErrMultipleStreams) {
		return &convert.UsageError{Err: fmt.Errorf("%w: choose one with --type and --log-source", conflict)}
	}
	for _, e := range res.Errors {
		slog.Error(e.Message, "stage", e.Stage, "subject", e.Subject)
	}
	for _, o := range res.Outputs {
		slog.Info("wrote RINEX", "file", o.Path, "epochs", o.Rows)
	}
	if code := convert.ExitCode(res, err); code != convert.ExitOK {
		return exitError{code}
	}
	return nil
}

// reader checks the request and makes the Reader and Selection of its source.
func (r rinexRun) reader(ctx context.Context) (storage.Reader, storage.Selection, error) {
	usage := func(format string, args ...any) (storage.Reader, storage.Selection, error) {
		return nil, storage.Selection{}, &convert.UsageError{Err: fmt.Errorf(format, args...)}
	}
	if r.modulo < 0 {
		return usage("--modulo %d: want 0 or more", r.modulo)
	}
	sel := storage.Selection{Scope: r.scope, Tables: append([]catalog.TableRef(nil), datamodels.L0GNSSObs)}
	if r.logSource != "" {
		if r.fileType == "" {
			return usage("--log-source %s needs --type", r.logSource)
		}
		ls := edid.LogSource{FileType: r.fileType, Name: r.logSource}
		if err := convert.CheckLogSource(ls, datamodels.L0GNSSObs); err != nil {
			return nil, sel, err
		}
		sel.LogSources = []edid.LogSource{ls}
	} else if r.fileType != "" && r.source == "iceberg" {
		sel.LogSources = []edid.LogSource{{FileType: r.fileType}}
	}
	switch r.source {
	case "raw":
		if r.fileType == "" {
			return usage("--source raw needs --type")
		}
		if len(r.files) == 0 {
			return usage("--source raw needs the raw files as arguments")
		}
		if r.settingsPath == "" {
			return usage("--source raw needs --settings")
		}
		if !r.scopeGiven {
			// The raw rows are stamped with a scope that RINEX never shows.
			sel.Scope = edid.Scope{Network: "rinex", Station: "rinex", Campaign: "rinex"}
		} else if r.scope.Campaign == "" {
			return usage("--scope: --source raw wants NET/STA/CAMP")
		}
		rdr, err := raw.NewReader(r.files, raw.Types(r.fileType), raw.AsGiven())
		if err != nil {
			return usage("%w", err)
		}
		return rdr, sel, nil
	case "iceberg":
		if len(r.files) > 0 {
			return usage("--source iceberg takes no file arguments")
		}
		if !r.scopeGiven {
			return usage("--source iceberg needs --scope")
		}
		if r.day != "" {
			day, err := time.Parse(time.DateOnly, r.day)
			if err != nil {
				return usage("--day: %w", err)
			}
			sel.Window = &catalog.Window{Min: arrow.Timestamp(day.UnixMicro()),
				Max: arrow.Timestamp(day.AddDate(0, 0, 1).UnixMicro() - 1)}
		}
		cfg, err := catalog.ParseConfig(r.catalog, r.warehouse)
		if err != nil {
			return usage("--catalog: %w", err)
		}
		cfg = cfg.WithDatabasePrefix(r.prefix)
		if _, err := sel.Filter(datamodels.L0GNSSObs.Def); err != nil {
			return usage("%w", err)
		}
		rdr, err := iceberg.New(ctx, cfg, datamodels.SFG.Only(datamodels.L0GNSSObs))
		if err != nil {
			return nil, sel, err
		}
		return rdr, sel, nil
	default:
		return usage("--source %q: want raw or iceberg", r.source)
	}
}

// settings reads the --settings file over the rinex.Settings defaults; with
// none the marker name is the scope's station.
func (r rinexRun) settings() (*gnssrinex.Settings, error) {
	settings := gnssrinex.NewSettings()
	settings.MarkerName = r.scope.Station
	if r.settingsPath == "" {
		return settings, nil
	}
	b, err := os.ReadFile(r.settingsPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", r.settingsPath, err)
	}
	return settings, nil
}
