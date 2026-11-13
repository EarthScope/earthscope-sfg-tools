// Command build_GNSSA_L0 is a dev harness that parses every raw file of a
// campaign's raw directory with the fileparse parsers and appends the rows
// to the Level 0 Iceberg database (gnssa_l0) in a local warehouse.
//
// Each file's type comes from its name (fileparse.FileTypeOf); files of no
// parsed type (e.g. INOUT0) are skipped. Every parser of the type runs on the
// file and its rows are appended to its table as one snapshot. Parsing runs
// on -workers goroutines; appends are serial, since a catalog Table is not
// safe for concurrent use. Append only adds rows, so the warehouse is new
// unless -warehouse names one, and re-running into it duplicates rows.
//
// From src/go:
//
//	go run ./dev/build_GNSSA_L0 -max 2           # a quick run: two files per type
//	go run ./dev/build_GNSSA_L0 -warehouse /tmp/l0 -types NOV000,DFOP00
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
)

const (
	defaultRaw       = "/Volumes/DunbarSSD/Project/SeafloorGeodesy/SFGMain/cascadia-gorda/GCC1/2025_A_1126/raw"
	defaultScope     = "cascadia-gorda/GCC1/2025_A_1126"
	defaultWarehouse = "/Volumes/DunbarSSD/Project/SeafloorGeodesy/SFGMain/Iceberg"
)

func main() {
	raw := flag.String("raw", defaultRaw, "campaign raw directory")
	scopeFlag := flag.String("scope", defaultScope, "network/station/campaign of the rows")
	warehouse := flag.String("warehouse", defaultWarehouse, "local warehouse directory (default: a new temporary one)")
	types := flag.String("types", "", "comma-separated file types to parse (default: every type with parsers)")
	maxFiles := flag.Int("max", 0, "parse at most this many files per type, spread over the directory (0: all)")
	workers := flag.Int("workers", runtime.NumCPU(), "files parsed at once")
	flag.Parse()

	scope, err := parseScope(*scopeFlag)
	if err != nil {
		log.Fatal(err)
	}
	if err := run(context.Background(), *raw, scope, *warehouse, *types, *maxFiles, *workers); err != nil {
		log.Fatal(err)
	}
}

// job is one parser to run on one file.
type job struct {
	path   string
	parser fileparse.Parser
}

// result is a job's rows, or why it has none.
type result struct {
	job
	rows arrow.Table
	err  error
}

func run(ctx context.Context, raw string, scope edid.Scope, warehouse, typeList string, maxFiles, workers int) error {
	files, skipped, err := listFiles(raw, typeList, maxFiles)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s: no files of a parsed type", raw)
	}
	for _, name := range skipped {
		log.Printf("skipping %s: no parsers for its type", name)
	}

	if warehouse == "" {
		if warehouse, err = os.MkdirTemp("", "build_GNSSA_L0-warehouse-"); err != nil {
			return err
		}
	}
	log.Printf("warehouse: %s", warehouse)
	cat, err := catalog.Build(ctx, catalog.LocalConfig(warehouse), datamodels.SFG)
	if err != nil {
		return fmt.Errorf("building catalog: %w", err)
	}
	defer cat.Close()
	db, err := cat.Database(datamodels.GNSSA_L0.Name)
	if err != nil {
		return err
	}

	var jobs []job
	for _, f := range files {
		for _, p := range fileparse.ForFileType(fileparse.FileTypeOf(f)) {
			jobs = append(jobs, job{path: f, parser: p})
		}
	}
	log.Printf("parsing %d files (%d parser runs) on %d workers", len(files), len(jobs), workers)

	results := parseAll(jobs, scope, max(workers, 1))
	start := time.Now()
	stats := map[string]*tableStats{}
	var failures []string
	done := 0
	for r := range results {
		done++
		st := stats[r.parser.Table.Name]
		if st == nil {
			st = &tableStats{}
			stats[r.parser.Table.Name] = st
		}
		if r.err != nil {
			failures = append(failures, fmt.Sprintf("%s -> %s: %v", filepath.Base(r.path), r.parser.Table.Name, r.err))
			continue
		}
		n, err := appendRows(ctx, db, r, scope)
		r.rows.Release()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s -> %s: %v", filepath.Base(r.path), r.parser.Table.Name, err))
			continue
		}
		st.files++
		st.rows += n
		if done%25 == 0 || done == len(jobs) {
			log.Printf("%d/%d parser runs done (%s)", done, len(jobs), time.Since(start).Round(time.Second))
		}
	}

	report(stats, failures, warehouse)
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d parser runs failed", len(failures), len(jobs))
	}
	return nil
}

// parseAll runs jobs on workers goroutines and sends each result; it closes
// the channel when every job is done. At most workers results wait unsent,
// which bounds the parsed rows held in memory.
func parseAll(jobs []job, scope edid.Scope, workers int) <-chan result {
	in := make(chan job)
	out := make(chan result)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				rows, err := j.parser.Parse(j.path, scope)
				out <- result{job: j, rows: rows, err: err}
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			in <- j
		}
		close(in)
		wg.Wait()
		close(out)
	}()
	return out
}

// appendRows appends r's rows to their gnssa_l0 table, recording the source
// file and scope in the snapshot summary.
func appendRows(ctx context.Context, db *catalog.Database, r result, scope edid.Scope) (int64, error) {
	tbl, err := db.Table(r.parser.Table.Name)
	if err != nil {
		return 0, err
	}
	return tbl.Append(ctx, r.rows, catalog.WithProperties(map[string]string{
		"sfg.source": filepath.Base(r.path),
		"sfg.scope":  fmt.Sprintf("%s/%s/%s", scope.Network, scope.Station, scope.Campaign),
	}))
}

// listFiles lists raw's files of the parsed types in typeList (all when
// empty), in name order, at most maxFiles per type (0: all) spread evenly
// over the type's files; skipped are the files of no parsed type.
func listFiles(raw, typeList string, maxFiles int) (files, skipped []string, err error) {
	entries, err := os.ReadDir(raw)
	if err != nil {
		return nil, nil, err
	}
	var want []string
	if typeList != "" {
		for _, t := range strings.Split(typeList, ",") {
			t = strings.ToUpper(strings.TrimSpace(t))
			if len(fileparse.ForFileType(t)) == 0 {
				return nil, nil, fmt.Errorf("-types: %s has no parsers", t)
			}
			want = append(want, t)
		}
	}
	byType := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ft := fileparse.FileTypeOf(e.Name())
		if ft == "" || len(fileparse.ForFileType(ft)) == 0 {
			skipped = append(skipped, e.Name())
			continue
		}
		if want != nil && !slices.Contains(want, ft) {
			continue
		}
		byType[ft] = append(byType[ft], filepath.Join(raw, e.Name()))
	}
	for _, ft := range slices.Sorted(maps.Keys(byType)) {
		fs := byType[ft]
		if maxFiles > 0 && len(fs) > maxFiles {
			fs = spread(fs, maxFiles)
		}
		log.Printf("%s: %d files", ft, len(fs))
		files = append(files, fs...)
	}
	return files, skipped, nil
}

// spread picks n of fs (len(fs) > n > 0) evenly spaced from first to last.
func spread(fs []string, n int) []string {
	out := make([]string, n)
	for i := range n {
		idx := 0
		if n > 1 {
			idx = i * (len(fs) - 1) / (n - 1)
		}
		out[i] = fs[idx]
	}
	return out
}

type tableStats struct {
	files int
	rows  int64
}

func report(stats map[string]*tableStats, failures []string, warehouse string) {
	names := make([]string, 0, len(stats))
	for n := range stats {
		names = append(names, n)
	}
	sort.Strings(names)
	log.Printf("gnssa_l0 in %s:", warehouse)
	for _, n := range names {
		log.Printf("  %-30s %6d files %12d rows", n, stats[n].files, stats[n].rows)
	}
	for _, f := range failures {
		log.Printf("FAILED %s", f)
	}
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
