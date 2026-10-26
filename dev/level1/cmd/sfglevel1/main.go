// Command sfglevel1 parses one raw GNSS-A log into a full-row Level 1 Arrow
// IPC file per table. It is a debug/export tool: Level 1 ingest writes to
// Iceberg from Go through package sink (see dev/level1/main.go), not through
// these files and the Python ingest layer (dev/iceberg_level1.py).
//
// Usage:
//
//	sfglevel1 FILETYPE -out DIR [-table NAME] [-log LOGTYPE] FILE
//
// FILETYPE is a file type in fileparse.Parsers (AHRS00, NOV770, NOV000,
// DFOP00). Every (log type, table) the file type produces is written as
// DIR/<table>_<logtype>.arrow, or only those matching -table and -log. The scope is read from FILE's
// .../<network>/<station>/<campaign>/raw directory unless -network, -station
// and -campaign override it.
package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/catalog"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse"
)

const usage = "usage: sfglevel1 FILETYPE -out DIR [-table NAME] [-log LOGTYPE] FILE"

func parseFiles(ctx context.Context, cat catalog.Catalog, parsers []fileparse.Parser, files []string, scope datamodels.Scope) {
	for _, parser := range parsers {
		for _, file := range files {
			arrowTbl, err := parser.Parse(file, scope)
			if err != nil {
				slog.Error("failed to parse file", "file", file, "error", err)
				os.Exit(1)
			}
			slog.Info("parsed file successfully", "file", filepath.Base(file), "rows", arrowTbl.NumRows())
			// ingest the parsed arrow table into the catalog
			numRows, err := cat.Append(ctx, parser.Table, arrowTbl, catalog.WithSource(file))
			arrowTbl.Release()
			if err != nil {
				slog.Error("failed to ingest arrow table", "file", filepath.Base(file), "table", parser.Table.Name, "error", err)
				os.Exit(1)
			}
			slog.Info("ingested arrow table successfully", "numRows", numRows)
		}
	}
}

func main() {
	data_path, err := filepath.Abs("/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/SFGMain2/cascadia-gorda/SLT1/2026_A_1126/raw")
	if err != nil {
		slog.Error("failed to get absolute path", "error", err)
		os.Exit(1)
	}
	warehouse_path, err := filepath.Abs("/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/ICEBERG_L1")
	if err != nil {
		slog.Error("failed to get absolute path", "error", err)
		os.Exit(1)
	}
	ahrs, err := filepath.Glob(filepath.Join(data_path, "*AHRS00.bin"))
	if err != nil {
		slog.Error("failed to glob AHRS00 files", "error", err)
		os.Exit(1)
	}
	nov770, err := filepath.Glob(filepath.Join(data_path, "*NOV770.raw"))
	if err != nil {
		slog.Error("failed to glob NOV770 files", "error", err)
		os.Exit(1)
	}
	nov000, err := filepath.Glob(filepath.Join(data_path, "*NOV000.bin"))
	if err != nil {
		slog.Error("failed to glob NOV000 files", "error", err)
		os.Exit(1)
	}
	dfop00, err := filepath.Glob(filepath.Join(data_path, "*DFOP00.raw"))
	if err != nil {
		slog.Error("failed to glob DFOP00 files", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Build local iceberg catalog
	cfg := catalog.Config{
		Warehouse: warehouse_path,
		Type:      catalog.TypeSQL,
	}
	cat, err := catalog.Open(ctx, cfg)
	if err != nil {
		slog.Error("failed to open catalog", "warehouse", warehouse_path, "error", err)
		os.Exit(1)
	}
	defer cat.Close()

	// build the scope, then parse the nov000 files across all log types
	scope := datamodels.Scope{
		Network:  "cascadia-gorda",
		Station:  "SLT1",
		Campaign: "2026_A_1126",
	}
	// load the parsers for nov000 files
	parsers := fileparse.ForFileType("NOV000")
	parseFiles(ctx, *cat, parsers, nov000, scope)

	parsers = fileparse.ForFileType("AHRS00")
	parseFiles(ctx, *cat, parsers, ahrs, scope)

	parsers = fileparse.ForFileType("DFOP00")
	parseFiles(ctx, *cat, parsers, dfop00, scope)

	parsers = fileparse.ForFileType("NOV770")
	parseFiles(ctx, *cat, parsers, nov770, scope)

}
