// Command build_level0 is a dev harness that builds the Level 0 Iceberg
// database (gnssa_l0) in a new local warehouse and appends the rows parsed
// from one raw file to it.
//
// Rows come from the fileparse package: one Parser per (file type, table),
// each producing an arrow table with the table's full schema. The catalog
// package builds the database and its tables from their definitions
// (Catalog -> Database -> Table, see .blueprint/catalog-database-table.md),
// then each parser's rows are appended to its table.
//
// Set rawFile and testScope below, then from src/go:
//
//	go run ./dev/build_level0
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
)

// level0 is the SFG catalog narrowed to its Level 0 database, gnssa_l0.
var level0 = datamodels.CatalogDef{
	Name:      datamodels.SFG.Name,
	Doc:       datamodels.SFG.Doc,
	Databases: []datamodels.DatabaseDef{datamodels.GNSSA_L0},
}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// run builds a new warehouse, parses rawFile with every NOV000 parser, and
// appends each parser's rows to its gnssa_l0 table.
func run(ctx context.Context) error {
	// rawFile is the NOV000 test file to parse, e.g. /path/to/NOV000.bin.
	const rawFile = "/Volumes/DunbarSSD/Project/SeafloorGeodesy/SFGMain/cascadia-gorda/GCC1/2025_A_1126/raw/341517_001_20250821_205746_00062_NOV000.bin"

	// testScope is the network, station and campaign the rows belong to.
	var testScope = datamodels.Scope{Network: "cascadia-gorda", Station: "GCC1", Campaign: "2025_A_1126"}

	if rawFile == "" {
		return errors.New("set rawFile in main.go to the NOV000 test file")
	}
	if _, err := os.Stat(rawFile); err != nil {
		return fmt.Errorf("rawFile: %w", err)
	}

	// A new warehouse each run, kept afterwards so it can be inspected.
	warehouse, err := os.MkdirTemp("", "build_level0-warehouse-")
	if err != nil {
		return err
	}
	log.Printf("warehouse: %s", warehouse)

	// The same definition builds Glue instead with catalog.Config{Name: "glue"}.
	cat, err := catalog.Build(ctx, catalog.Config{Name: catalog.Local, Warehouse: warehouse}, level0)
	if err != nil {
		return fmt.Errorf("building catalog: %w", err)
	}
	defer cat.Close()

	db, err := cat.Database(datamodels.GNSSA_L0.Name)
	if err != nil {
		return err
	}

	scope := fmt.Sprintf("%s/%s/%s", testScope.Network, testScope.Station, testScope.Campaign)
	parsers := fileparse.ForFileType("NOV000")
	if len(parsers) == 0 {
		return errors.New("no NOV000 parsers in fileparse")
	}
	for _, p := range parsers {
		tbl, err := db.Table(p.Table.Name)
		if err != nil {
			return err
		}
		rows, err := p.Parse(rawFile, testScope)
		if err != nil {
			return fmt.Errorf("parsing %s into %s: %w", rawFile, p.Table.Name, err)
		}
		if rows.NumRows() == 0 {
			log.Printf("%s: no rows", p.Table.Name)
			rows.Release()
			continue
		}
		n, err := tbl.Append(ctx, rows,
			catalog.WithProperties(map[string]string{"scope": scope}),
			catalog.WithSource(rawFile))
		rows.Release()
		if err != nil {
			return err
		}
		log.Printf("appended %d rows to %s.%s", n, db.Name(), p.Table.Name)
	}
	return nil
}
