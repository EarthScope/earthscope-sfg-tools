// Command level1 is a dev harness: it builds the Level 1 Iceberg catalog in a
// local warehouse, creates every Level 1 table, and runs every parser in
// fileparse.Parsers over one campaign's raw directory, appending each result.
//
// Usage:
//
//	go run . [-data DIR] [-catalog sql|glue] [-region R] [-warehouse DIR|s3://...] [-types T,...] [-max-files N] [-v]
//
// DIR is .../<network>/<station>/<campaign>/raw, which sets the row scope.
// -catalog sql (default) writes a local SQLite catalog in -warehouse;
// -catalog glue writes AWS Glue with -warehouse an s3:// URI, using AWS
// credentials from the environment. -types limits the run to those file
// types (e.g. CTD or NOV000,NOV770); QCPIN and GNSS00 run only when named
// (see optIn).
// Re-running replaces each stream's rows over the re-parsed time range
// (see catalog.Append) rather than adding duplicates.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/catalog"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/sink"
)

const (
	defaultData      = "/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/SFGMain2/cascadia-gorda/SLT1/2026_A_1126/raw"
	defaultWarehouse = "/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/ICEBERG"
)

func main() {
	dataPath := flag.String("data", defaultData, "campaign raw directory, .../<network>/<station>/<campaign>/raw")
	catalogType := flag.String("catalog", catalog.TypeSQL, "catalog type: sql (local) or glue")
	region := flag.String("region", "", "AWS region for -catalog glue (default: AWS config)")
	warehousePath := flag.String("warehouse", defaultWarehouse, "Iceberg warehouse: local directory (sql) or s3:// URI (glue)")
	types := flag.String("types", "", "comma-separated file types to run (default: all but "+strings.Join(optIn, ", ")+")")
	maxFiles := flag.Int("max-files", 0, "parse at most this many files of each type (0 = all)")
	verbose := flag.Bool("v", false, "log per-file parser details")
	flag.Parse()
	if *verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	var parsers []fileparse.Parser
	for _, p := range fileparse.Parsers {
		if !slices.Contains(optIn, p.FileType) {
			parsers = append(parsers, p)
		}
	}
	if *types != "" {
		parsers = nil
		for _, ft := range strings.Split(*types, ",") {
			ps := fileparse.ForFileType(strings.TrimSpace(ft))
			if len(ps) == 0 {
				fmt.Fprintf(os.Stderr, "level1: no parsers for file type %q\n", ft)
				os.Exit(2)
			}
			parsers = append(parsers, ps...)
		}
	}
	cfg := catalog.Config{Type: *catalogType, Warehouse: *warehousePath, Region: *region}
	if err := run(context.Background(), *dataPath, cfg, parsers, *maxFiles); err != nil {
		fmt.Fprintln(os.Stderr, "level1:", err)
		os.Exit(1)
	}
}

// optIn are the file types run only when -types names them. QCPIN files are
// QC copies of pings the campaign's DFOP00 files already log, so running
// both would store those pings twice. GNSS00 is on hold: its GGA fixes are
// sparse, and its packet times sit a fixed 100 ms before the fix times they
// carry, which is unexplained.
var optIn = []string{"QCPIN", "GNSS00"}

// patterns are the filename globs of each file type, relative to a campaign
// raw directory (QC PIN files are unpacked under the campaign's qc/). Finding
// files is this harness's job, not fileparse's.
var patterns = map[string]string{
	"AHRS00":  "*AHRS00.bin",
	"NOV770":  "*NOV770.raw",
	"NOV000":  "*NOV000.bin",
	"DFOP00":  "*DFOP00.raw",
	"GNSS00":  "*GNSS00.bin",
	"CTD":     "*CTD*_fit",
	"SEABIRD": "*.cnv",
	"QCPIN":   "../qc/*/*.pin",
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

func run(ctx context.Context, dataPath string, cfg catalog.Config, parsers []fileparse.Parser, maxFiles int) error {
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
			slog.Info("appended", "file", filepath.Base(path), "log", p.LogType, "table", p.Table.Name, "rows", n)
		}
	}
	for _, def := range datamodels.Level1Tables {
		slog.Info("total", "table", def.Namespace()+"."+def.Name, "rows", totals[def.Name])
	}
	return nil
}
