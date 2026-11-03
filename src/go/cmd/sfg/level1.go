package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"

	"github.com/EarthScope/es_sfgtools/src/golangtools/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/fileparse"
	"github.com/EarthScope/es_sfgtools/src/golangtools/sink"
)

var level1Cmd = &cobra.Command{
	Use:   "level1",
	Short: "Parse raw files into Level 1 tables, or ingest them into Iceberg",
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		if v, _ := cmd.Flags().GetBool("verbose"); v {
			slog.SetLogLoggerLevel(slog.LevelDebug)
		}
	},
}

// level1Result is the result.json `level1 parse` writes beside the tables.
type level1Result struct {
	File     string        `json:"file"`
	FileType string        `json:"file_type"`
	Scope    level1Scope   `json:"scope"`
	Tables   []level1Table `json:"tables"`
	Errors   []level1Error `json:"errors"`
}

type level1Scope struct {
	Network  string `json:"network"`
	Station  string `json:"station"`
	Campaign string `json:"campaign"`
}

type level1Table struct {
	Table   string           `json:"table"`
	File    string           `json:"file"` // Arrow IPC file in --out
	Rows    int64            `json:"rows"`
	Streams map[string]int64 `json:"streams"` // rows per log source
}

type level1Error struct {
	Table string `json:"table"`
	Error string `json:"error"`
}

var level1ParseCmd = &cobra.Command{
	Use:   "parse FILE",
	Short: "Parse one raw file into Level 1 Arrow IPC tables",
	Long: "Parse one raw file of --type into every Level 1 table the file type feeds. " +
		"Each table is written to --out as <table>.arrow (Arrow IPC file) with its full " +
		"schema, stamped with the scope and stream EDIDs. result.json lists the rows " +
		"per table and per log source, and the tables that failed. The command fails " +
		"only if every table failed.",
	Args: cobra.ExactArgs(1),
	RunE: runLevel1Parse,
}

func runLevel1Parse(cmd *cobra.Command, args []string) error {
	fileType, _ := cmd.Flags().GetString("type")
	outDir, _ := cmd.Flags().GetString("out")
	scope := datamodels.Scope{}
	scope.Network, _ = cmd.Flags().GetString("network")
	scope.Station, _ = cmd.Flags().GetString("station")
	scope.Campaign, _ = cmd.Flags().GetString("campaign")

	parsers := fileparse.ForFileType(fileType)
	if len(parsers) == 0 {
		return fmt.Errorf("no parsers for file type %q", fileType)
	}
	path := args[0]
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	res := level1Result{File: path, FileType: parsers[0].FileType,
		Scope:  level1Scope{scope.Network, scope.Station, scope.Campaign},
		Tables: []level1Table{}, Errors: []level1Error{}}
	for _, p := range parsers {
		t, err := parseToIPC(p, path, scope, outDir)
		if err != nil {
			res.Errors = append(res.Errors, level1Error{Table: p.Table.Name, Error: err.Error()})
			slog.Error("parse failed", "file", filepath.Base(path), "table", p.Table.Name, "error", err)
			continue
		}
		res.Tables = append(res.Tables, t)
		slog.Info("parsed", "file", filepath.Base(path), "table", t.Table, "rows", t.Rows)
	}

	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "result.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	if len(res.Tables) == 0 {
		return fmt.Errorf("%s: every table failed (see result.json)", path)
	}
	return nil
}

// parseToIPC parses path with p and writes the table to outDir/<table>.arrow.
func parseToIPC(p fileparse.Parser, path string, scope datamodels.Scope, outDir string) (level1Table, error) {
	tbl, err := p.Parse(path, scope)
	if err != nil {
		return level1Table{}, err
	}
	defer tbl.Release()

	name := p.Table.Name + ".arrow"
	if err := writeIPC(filepath.Join(outDir, name), tbl); err != nil {
		return level1Table{}, err
	}
	return level1Table{Table: p.Table.Name, File: name, Rows: tbl.NumRows(), Streams: streamCounts(tbl)}, nil
}

// writeIPC writes tbl to path as an Arrow IPC file.
func writeIPC(path string, tbl arrow.Table) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w, err := ipc.NewFileWriter(f, ipc.WithSchema(tbl.Schema()))
	if err != nil {
		f.Close()
		return err
	}
	r := array.NewTableReader(tbl, -1)
	defer r.Release()
	for r.Next() {
		if err := w.Write(r.RecordBatch()); err != nil {
			w.Close()
			f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// streamCounts is the rows of tbl per log source: the last part of each
// row's stream EDID.
func streamCounts(tbl arrow.Table) map[string]int64 {
	counts := map[string]int64{}
	idx := tbl.Schema().FieldIndices(datamodels.StreamEDIDColumn.Name)
	if len(idx) != 1 {
		return counts
	}
	for _, chunk := range tbl.Column(idx[0]).Data().Chunks() {
		s := chunk.(*array.String)
		for i := 0; i < s.Len(); i++ {
			e := s.Value(i)
			counts[e[strings.LastIndex(e, ":")+1:]]++
		}
	}
	return counts
}

var level1IngestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Parse a campaign's raw files into the Level 1 Iceberg catalog",
	Long: "Create every Level 1 table in the catalog, then parse each file of the selected " +
		"file types found in --data (.../<network>/<station>/<campaign>/raw, which sets the " +
		"scope) and append its tables. Re-running replaces each stream's rows over the " +
		"re-parsed time range rather than adding duplicates. --catalog sql writes a local " +
		"SQLite catalog in --warehouse; --catalog glue writes AWS Glue with --warehouse an " +
		"s3:// URI, using AWS credentials from the environment.",
	Args: cobra.NoArgs,
	RunE: runLevel1Ingest,
}

// optIn are the file types ingest runs only when --types names them. QCPIN
// files are QC copies of pings the campaign's DFOP00 files already log, so
// running both would store those pings twice. GNSS00 is on hold: its GGA
// fixes are sparse, and its packet times sit a fixed 100 ms before the fix
// times they carry, which is unexplained.
var optIn = []string{"QCPIN", "GNSS00"}

// patterns are the filename globs of each file type, relative to a campaign
// raw directory (QC PIN files are unpacked under the campaign's qc/). Finding
// files is the caller's job, not fileparse's.
var patterns = map[string]string{
	"AHRS00":  "*AHRS00.bin",
	"NOV770":  "*NOV770.raw",
	"NOV000":  "*NOV000.bin",
	"DFOP00":  "*DFOP00.raw",
	"GNSS00":  "*GNSS00.bin",
	"TCVR00":  "*TCVR00.bin",
	"CTD":     "*CTD*_fit",
	"SEABIRD": "*.cnv",
	"QCPIN":   "../qc/*/*.pin",
}

func runLevel1Ingest(cmd *cobra.Command, _ []string) error {
	dataPath, _ := cmd.Flags().GetString("data")
	catalogType, _ := cmd.Flags().GetString("catalog")
	region, _ := cmd.Flags().GetString("region")
	warehouse, _ := cmd.Flags().GetString("warehouse")
	types, _ := cmd.Flags().GetStringSlice("types")
	maxFiles, _ := cmd.Flags().GetInt("max-files")

	var parsers []fileparse.Parser
	if len(types) == 0 {
		for _, p := range fileparse.Parsers {
			if !slices.Contains(optIn, p.FileType) {
				parsers = append(parsers, p)
			}
		}
	}
	for _, ft := range types {
		ps := fileparse.ForFileType(strings.TrimSpace(ft))
		if len(ps) == 0 {
			return fmt.Errorf("no parsers for file type %q", ft)
		}
		parsers = append(parsers, ps...)
	}
	cfg := catalog.Config{Type: catalogType, Warehouse: warehouse, Region: region}
	return ingest(cmd.Context(), dataPath, cfg, parsers, maxFiles)
}

// glob returns the files matching pattern under dir, sorted, without the
// "._" AppleDouble files macOS writes beside files on non-Mac volumes.
func glob(dir, pattern string) []string {
	paths, _ := filepath.Glob(filepath.Join(dir, pattern))
	paths = slices.DeleteFunc(paths, func(p string) bool {
		return strings.HasPrefix(filepath.Base(p), "._")
	})
	sort.Strings(paths)
	return paths
}

func ingest(ctx context.Context, dataPath string, cfg catalog.Config, parsers []fileparse.Parser, maxFiles int) error {
	scope := datamodels.ScopeFromRawDir(dataPath)
	slog.Info("scope", "network", scope.Network, "station", scope.Station, "campaign", scope.Campaign)

	out, err := sink.OpenIceberg(ctx, cfg)
	if err != nil {
		return err
	}
	defer out.Close()

	for _, def := range datamodels.Level1Tables {
		if _, err := out.Catalog().Table(ctx, def); err != nil {
			return err
		}
	}

	totals := map[string]int64{}
	for _, p := range parsers {
		pattern, ok := patterns[p.FileType]
		if !ok {
			return fmt.Errorf("no filename pattern for file type %s", p.FileType)
		}
		files := glob(dataPath, pattern)
		if maxFiles > 0 && len(files) > maxFiles {
			files = files[:maxFiles]
		}
		for _, path := range files {
			rows, err := p.Parse(path, scope)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			n, err := out.Write(ctx, p.Table, rows)
			rows.Release()
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			totals[p.Table.Name] += n
			slog.Info("appended", "file", filepath.Base(path), "table", p.Table.Name, "rows", n)
		}
	}
	for _, def := range datamodels.Level1Tables {
		slog.Info("total", "table", def.Namespace()+"."+def.Name, "rows", totals[def.Name])
	}
	return nil
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

	f := level1ParseCmd.Flags()
	f.String("type", "", "file type, e.g. NOV000, DFOP00 (required)")
	f.String("network", "", "network of the rows' scope (required)")
	f.String("station", "", "station of the rows' scope (required)")
	f.String("campaign", "", "campaign of the rows' scope (required)")
	f.String("out", "", "output directory for the Arrow IPC files and result.json (required)")
	for _, name := range []string{"type", "network", "station", "campaign", "out"} {
		_ = level1ParseCmd.MarkFlagRequired(name)
	}

	f = level1IngestCmd.Flags()
	f.String("data", "", "campaign raw directory, .../<network>/<station>/<campaign>/raw (required)")
	f.String("warehouse", "", "Iceberg warehouse: local directory (sql) or s3:// URI (glue) (required)")
	f.String("catalog", catalog.TypeSQL, "catalog type: sql (local) or glue")
	f.String("region", "", "AWS region for --catalog glue (default: AWS config)")
	f.StringSlice("types", nil, "file types to run (default: all but "+strings.Join(optIn, ", ")+")")
	f.Int("max-files", 0, "parse at most this many files of each type (0 = all)")
	for _, name := range []string{"data", "warehouse"} {
		_ = level1IngestCmd.MarkFlagRequired(name)
	}

	f = level1RINEXCmd.Flags()
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

	level1Cmd.AddCommand(level1ParseCmd, level1IngestCmd, level1RINEXCmd)
}
