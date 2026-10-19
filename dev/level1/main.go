// Command level1 is a dev harness: it builds the Level 1 Iceberg catalog in a
// local warehouse, creates every Level 1 table, and appends every table each
// file parser in fileparse can produce from one campaign's raw directory.
//
// Usage:
//
//	go run . [-data DIR] [-warehouse DIR] [-max-files N] [-v]
//
// DIR is .../<network>/<station>/<campaign>/raw, which sets the row scope.
// Re-running appends duplicate rows; delete the warehouse to start over.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/catalog"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse"
)

const (
	defaultData      = "/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/SFGMain2/cascadia-gorda/SLT1/2026_A_1126/raw"
	defaultWarehouse = "/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/ICEBERG"
)

func main() {
	dataPath := flag.String("data", defaultData, "campaign raw directory, .../<network>/<station>/<campaign>/raw")
	warehousePath := flag.String("warehouse", defaultWarehouse, "Iceberg warehouse directory")
	maxFiles := flag.Int("max-files", 0, "parse at most this many files of each type (0 = all)")
	verbose := flag.Bool("v", false, "log per-file parser details")
	flag.Parse()
	if *verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	if err := run(context.Background(), *dataPath, *warehousePath, *maxFiles); err != nil {
		fmt.Fprintln(os.Stderr, "level1:", err)
		os.Exit(1)
	}
}

func glob(dir, pattern string) []string {
	paths, _ := filepath.Glob(filepath.Join(dir, pattern))
	sort.Strings(paths)
	return paths
}

func run(ctx context.Context, dataPath, warehousePath string, maxFiles int) error {
	scope := schemas.ScopeFromRawDir(dataPath)
	slog.Info("scope", "network", scope.Network, "station", scope.Station, "campaign", scope.Campaign)

	cat, err := catalog.Open(warehousePath)
	if err != nil {
		return err
	}
	defer cat.Close()

	for _, def := range schemas.Level1Tables {
		if _, err := cat.Table(ctx, def); err != nil {
			return err
		}
	}

	totals := map[string]int64{}
	for _, ft := range fileparse.FileTypes {
		files := glob(dataPath, ft.Pattern)
		if maxFiles > 0 && len(files) > maxFiles {
			files = files[:maxFiles]
		}
		for _, path := range files {
			for _, def := range ft.Tables {
				rows, err := ft.Parse(path, def, scope)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				n, err := cat.Append(ctx, def, rows)
				rows.Release()
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				totals[def.Name] += n
				slog.Info("appended", "file", filepath.Base(path), "table", def.Name, "rows", n)
			}
		}
	}
	for _, def := range schemas.Level1Tables {
		slog.Info("total", "table", def.Namespace()+"."+def.Name, "rows", totals[def.Name])
	}
	return nil
}
