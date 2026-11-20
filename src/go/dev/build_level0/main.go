// Command build_level0 is a dev harness that parses a campaign's raw
// directory with the Level 0 parsers and writes the rows to the Level 0
// Iceberg database (gnssa_l0) in a local warehouse.
//
// It is the three layers of `sfg convert --source raw --sink iceberg` put
// together by hand: a raw.Reader, an iceberg.Iceberg writer and convert.Run.
// Each file's type comes from its directory pattern; every table its types
// feed is written, one snapshot per file that replaces its streams' rows over
// its span.
//
// From src/go:
//
//	go run ./dev/build_level0 -max 2           # a quick run: two files per type
//	go run ./dev/build_level0 -warehouse /tmp/l0 -types NOV000,DFOP00
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/iceberg"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/raw"
)

const (
	defaultRaw       = "/Volumes/DunbarSSD/Project/SeafloorGeodesy/SFGMain/cascadia-gorda/GCC1/2025_A_1126/raw"
	defaultScope     = "cascadia-gorda/GCC1/2025_A_1126"
	defaultWarehouse = "/Volumes/DunbarSSD/Project/SeafloorGeodesy/SFGMain/Iceberg"
)

func main() {
	rawDir := flag.String("raw", defaultRaw, "campaign raw directory")
	scopeFlag := flag.String("scope", defaultScope, "network/station/campaign of the rows")
	warehouse := flag.String("warehouse", defaultWarehouse, "local warehouse directory (empty: a new temporary one)")
	types := flag.String("types", "", "comma-separated file types to parse (default: every type with parsers)")
	maxFiles := flag.Int("max", 0, "parse at most this many files per type (0: all)")
	workers := flag.Int("workers", runtime.NumCPU(), "files parsed at once")
	flag.Parse()

	scope, err := parseScope(*scopeFlag)
	if err != nil {
		log.Fatal(err)
	}
	if err := run(context.Background(), *rawDir, scope, *warehouse, *types, *maxFiles, *workers); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, rawDir string, scope edid.Scope, warehouse, typeList string, maxFiles, workers int) error {
	opts := []raw.Option{raw.MaxFiles(maxFiles), raw.Workers(max(workers, 1))}
	if typeList != "" {
		opts = append(opts, raw.Types(strings.Split(typeList, ",")...))
	}
	rd, err := raw.NewReader([]string{rawDir}, opts...)
	if err != nil {
		return err
	}
	tables := rd.Tables()
	if len(tables) == 0 {
		return errors.New("no tables to write")
	}
	if warehouse == "" {
		if warehouse, err = os.MkdirTemp("", "build_level0-warehouse-"); err != nil {
			return err
		}
	}
	log.Printf("warehouse: %s", warehouse)
	w, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG.Only(tables...), iceberg.Create())
	if err != nil {
		return err
	}
	res, err := convert.Run(ctx, rd, w, storage.Selection{Scope: scope, Tables: tables})
	if res != nil {
		for _, o := range res.Outputs {
			log.Printf("  %-30s %12d rows", o.Table, o.Rows)
		}
		for _, e := range res.Errors {
			log.Printf("FAILED %s %s: %s", e.Stage, e.Subject, e.Message)
		}
	}
	if err != nil {
		return err
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitOK {
		return errors.New("some files failed")
	}
	return nil
}

// parseScope parses network/station/campaign.
func parseScope(s string) (edid.Scope, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 || slices.Contains(parts, "") {
		return edid.Scope{}, errors.New("-scope: want network/station/campaign")
	}
	scope := edid.Scope{Network: parts[0], Station: parts[1], Campaign: parts[2]}
	return scope, edid.CheckScope(scope)
}
