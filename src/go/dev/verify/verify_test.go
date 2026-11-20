// Package verify is the end-to-end check of the library on the repo's raw
// fixtures: it drives fileparse, datamodels and catalog directly (verify.sh
// drives the sfg binary instead). From src/go:
//
//	go test ./dev/verify/ -count=1 -v
//	SFG_DEV_WAREHOUSE=s3://test-sfg-api/sfg-dev AWS_REGION=us-east-2 \
//	    go test ./dev/verify/ -count=1 -run Glue -glue -v -timeout 30m
//
// With -glue the Glue tests write to the dev_ databases under
// SFG_DEV_WAREHOUSE and never touch production.
package verify

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	gnssrinex "gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse/nov770"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/files"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/iceberg"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/raw"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage/rinex"
)

var glueFlag = flag.Bool("glue", false, "also run the Glue tests, against the dev_ databases under SFG_DEV_WAREHOUSE")

const (
	devPrefix        = "dev_"
	defaultWarehouse = "s3://test-sfg-api/sfg-dev"
	defaultRegion    = "us-east-2"
)

var scope = edid.Scope{Network: "verify", Station: "VRFY", Campaign: "2026_A_0001"}

// streamPrefix is the start of every stream_edid of scope.
var streamPrefix = scope.Network + "." + scope.Station + "." + scope.Campaign + "."

var fixtures = []string{
	"test_20260331_174553_00030_NOV770.raw",
	"test_gcc1_20250821_205746_00062_NOV000.bin",
	"test_gcc1_20250822_DFOP00.raw",
}

// counts is row counts by table name, then stream_edid.
type counts map[string]map[string]int64

func (c counts) lines() []string {
	var out []string
	for t, m := range c {
		for s, n := range m {
			out = append(out, fmt.Sprintf("%s\t%s\t%d", t, s, n))
		}
	}
	slices.Sort(out)
	return out
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "..", "tests", "data", name)
}

// parsed is one parser's output on one fixture.
type parsed struct {
	fixture string
	parser  fileparse.Parser
	rows    arrow.Table
}

// parseAll parses every fixture with every parser of its file type. Tables
// are released when the test ends.
func parseAll(t *testing.T) []parsed {
	t.Helper()
	var out []parsed
	for _, name := range fixtures {
		path := fixturePath(name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		ft := fileparse.FileTypeOf(path)
		ps := fileparse.ForFileType(ft)
		if len(ps) == 0 {
			t.Fatalf("%s: no parsers for file type %q", name, ft)
		}
		for _, p := range ps {
			rows, err := p.Parse(path, edid.AllLogSources, scope)
			if err != nil {
				t.Fatalf("%s: parsing %s: %v", name, p.Ref.Def.Name, err)
			}
			t.Cleanup(rows.Release)
			out = append(out, parsed{fixture: path, parser: p, rows: rows})
		}
	}
	return out
}

// streamCounts counts the rows of an Arrow table by stream_edid.
func streamCounts(tb arrow.Table) (map[string]int64, error) {
	idx := tb.Schema().FieldIndices(catalog.StreamEDIDName)
	if len(idx) != 1 {
		return nil, fmt.Errorf("want one %s column, have %d", catalog.StreamEDIDName, len(idx))
	}
	out := map[string]int64{}
	rdr := array.NewTableReader(tb, -1)
	defer rdr.Release()
	for rdr.Next() {
		col, ok := rdr.RecordBatch().Column(idx[0]).(*array.String)
		if !ok {
			return nil, fmt.Errorf("%s is not a string column", catalog.StreamEDIDName)
		}
		for i := range col.Len() {
			if !col.IsNull(i) {
				out[col.Value(i)]++
			}
		}
	}
	return out, nil
}

func parsedCounts(t *testing.T, ps []parsed) counts {
	t.Helper()
	c := counts{}
	for _, p := range ps {
		m, err := streamCounts(p.rows)
		if err != nil {
			t.Fatalf("%s: %v", p.parser.Ref.Def.Name, err)
		}
		for s, n := range m {
			if c[p.parser.Ref.Def.Name] == nil {
				c[p.parser.Ref.Def.Name] = map[string]int64{}
			}
			c[p.parser.Ref.Def.Name][s] += n
		}
	}
	return c
}

// baselineCounts reads the count lines of baseline.txt.
func baselineCounts(t *testing.T) counts {
	t.Helper()
	f, err := os.Open("baseline.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c := counts{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Split(sc.Text(), "\t")
		if fs[0] != "count" {
			continue
		}
		if len(fs) != 4 {
			t.Fatalf("baseline.txt: bad line %q", sc.Text())
		}
		n, err := strconv.ParseInt(fs[3], 10, 64)
		if err != nil {
			t.Fatalf("baseline.txt: %q: %v", sc.Text(), err)
		}
		if c[fs[1]] == nil {
			c[fs[1]] = map[string]int64{}
		}
		c[fs[1]][fs[2]] = n
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return c
}

func diffCounts(t *testing.T, what string, got, want counts) {
	t.Helper()
	g, w := got.lines(), want.lines()
	if slices.Equal(g, w) {
		return
	}
	var b strings.Builder
	for _, l := range g {
		if !slices.Contains(w, l) {
			fmt.Fprintf(&b, "\n  extra   %s", l)
		}
	}
	for _, l := range w {
		if !slices.Contains(g, l) {
			fmt.Fprintf(&b, "\n  missing %s", l)
		}
	}
	t.Errorf("%s differ from the expected counts:%s", what, b.String())
}

func TestParseMatchesBaseline(t *testing.T) {
	diffCounts(t, "parsed counts", parsedCounts(t, parseAll(t)), baselineCounts(t))
}

func TestFixturesYield(t *testing.T) {
	c := parsedCounts(t, parseAll(t))
	rowsOf := func(fileType string) int64 {
		var n int64
		for _, m := range c {
			for s, k := range m {
				if strings.Contains(s, "."+fileType+".") {
					n += k
				}
			}
		}
		return n
	}
	if rowsOf(fileparse.FileTypeDFOP00) == 0 {
		t.Error("DFOP00 fixture gave no rows")
	}
	if rowsOf(fileparse.FileTypeNOV770) == 0 {
		t.Error("NOV770 fixture gave no rows")
	}
	// Every record of the NOV000 fixture predates a time fix and is skipped
	// by design; it must still parse without error (parseAll checks).
	if n := rowsOf(fileparse.FileTypeNOV000); n != 0 {
		t.Errorf("NOV000 fixture gave %d rows, want 0", n)
	}
}

func TestDefinitions(t *testing.T) {
	if err := datamodels.CheckDefinitions(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "pkg", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	const open, closer = "```mermaid\n", "```"
	text := string(data)
	start := strings.Index(text, open)
	if start < 0 {
		t.Fatal("pkg/README.md: no mermaid block")
	}
	start += len(open)
	end := strings.Index(text[start:], closer)
	if end < 0 {
		t.Fatal("pkg/README.md: unterminated mermaid block")
	}
	if got := text[start : start+end]; got != datamodels.SourcesMermaid() {
		t.Error("pkg/README.md diagram is out of date; run go run ./dev/gen_readme")
	}
}

func TestIcebergLocal(t *testing.T) {
	dir := t.TempDir()
	testIceberg(t, "", func(ctx context.Context, opts ...iceberg.Option) (*iceberg.Iceberg, error) {
		return iceberg.NewLocal(ctx, dir, datamodels.SFG, opts...)
	})
}

func TestIcebergGlue(t *testing.T) {
	if !*glueFlag {
		t.Skip("pass -glue to run against AWS Glue (dev_ databases)")
	}
	warehouse := os.Getenv("SFG_DEV_WAREHOUSE")
	if warehouse == "" {
		warehouse = defaultWarehouse
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = defaultRegion
	}
	// Guard: before any connection, refuse anything but a dev warehouse.
	if !strings.HasPrefix(warehouse, "s3://") {
		t.Fatalf("SFG_DEV_WAREHOUSE %q is not an s3:// URI", warehouse)
	}
	if strings.Contains(strings.ToLower(warehouse), "gnssa-test") {
		t.Fatalf("SFG_DEV_WAREHOUSE %q looks like production; refusing", warehouse)
	}
	if bucket, path, _ := strings.Cut(strings.TrimPrefix(warehouse, "s3://"), "/"); bucket == "" || strings.Trim(path, "/") == "" {
		t.Fatalf("SFG_DEV_WAREHOUSE %q is a bucket root; want a prefix such as %s", warehouse, defaultWarehouse)
	}
	testIceberg(t, devPrefix, func(ctx context.Context, opts ...iceberg.Option) (*iceberg.Iceberg, error) {
		return iceberg.NewGlue(ctx, warehouse, datamodels.SFG,
			append([]iceberg.Option{iceberg.Region(region), iceberg.DatabasePrefix(devPrefix)}, opts...)...)
	})
}

// testIceberg ingests the fixtures twice through convert.Run (raw Reader to
// Iceberg Writer), reads every table back through convert.Run (Iceberg
// Reader to CSV Writer), and checks the counts, idempotence and the recorded
// source hashes. open connects to the catalog under test; each convert.Run
// closes its Writer, so each run opens its own. The protection against
// touching production is the opener passing DatabasePrefix(dev_); every
// database's namespace is also checked to start with prefix, right after
// Build, as a second line of defence.
func testIceberg(t *testing.T, prefix string, open func(context.Context, ...iceberg.Option) (*iceberg.Iceberg, error)) {
	ctx := context.Background()
	rows := parseAll(t)
	want := parsedCounts(t, rows)
	if len(want) == 0 {
		t.Fatal("the fixtures parsed to no rows; nothing to verify")
	}
	var inputs []string
	for _, name := range fixtures {
		inputs = append(inputs, fixturePath(name))
	}
	sel := storage.Selection{Scope: scope}

	ingest := func() {
		t.Helper()
		rdr, err := raw.NewReader(inputs)
		if err != nil {
			t.Fatal(err)
		}
		w, err := open(ctx, iceberg.Create())
		if err != nil {
			t.Fatalf("building the catalog: %v", err)
		}
		db, err := w.Catalog().Database(datamodels.GNSSA_L0.Name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(db.Namespace(), prefix) {
			w.Close()
			t.Fatalf("catalog uses namespace %q, want a %s* database", db.Namespace(), prefix)
		}
		res, err := convert.Run(ctx, rdr, w, sel)
		if err != nil {
			t.Fatal(err)
		}
		if code := convert.ExitCode(res, nil); code != convert.ExitOK {
			t.Fatalf("ingest: exit code %d, errors %+v", code, res.Errors)
		}
	}
	export := func() counts {
		t.Helper()
		r, err := open(ctx)
		if err != nil {
			t.Fatalf("loading the catalog: %v", err)
		}
		defer r.Close()
		out := t.TempDir()
		w, err := files.NewWriter(out, files.CSV)
		if err != nil {
			t.Fatal(err)
		}
		res, err := convert.Run(ctx, r, w, storage.Selection{Scope: scope, Tables: datamodels.GNSSA_L0.Refs()})
		if err != nil {
			t.Fatal(err)
		}
		if code := convert.ExitCode(res, nil); code != convert.ExitOK {
			t.Fatalf("export: exit code %d, errors %+v", code, res.Errors)
		}
		return csvCounts(t, out)
	}

	ingest()
	first := export()
	diffCounts(t, "exported counts after the first ingest", first, want)

	ingest()
	diffCounts(t, "exported counts after re-ingest", export(), first)

	check, err := open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()

	// Each written table's current snapshot records the SHA-256 of the
	// fixture that last wrote it.
	last := map[string]string{}
	for _, p := range rows {
		if p.rows.NumRows() == 0 {
			continue
		}
		src, err := catalog.HashFile(p.fixture)
		if err != nil {
			t.Fatal(err)
		}
		last[p.parser.Ref.Def.Name] = src.SHA256
	}
	if len(last) == 0 {
		t.Fatal("no table was written")
	}
	checked := 0
	for _, name := range slices.Sorted(maps.Keys(last)) {
		tbl, err := check.Catalog().TableOf(mustRef(t, name))
		if err != nil {
			t.Fatal(err)
		}
		snap := tbl.Iceberg().CurrentSnapshot()
		if snap == nil || snap.Summary == nil {
			t.Errorf("%s: no current snapshot summary", name)
			continue
		}
		checked++
		if got := snap.Summary.Properties[catalog.PropSourceSHA256]; got != last[name] {
			t.Errorf("%s: snapshot %s = %q, want %q", name, catalog.PropSourceSHA256, got, last[name])
		}
	}
	if checked == 0 {
		t.Fatal("no table's snapshot summary was checked for the source SHA-256")
	}
}

func mustRef(t *testing.T, name string) catalog.TableRef {
	t.Helper()
	ref, err := datamodels.SFG.Find(datamodels.GNSSA_L0.Name + "." + name)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// csvCounts counts the rows of every CSV under dir/<database>/<table>/ by
// table name and stream_edid.
func csvCounts(t *testing.T, dir string) counts {
	t.Helper()
	c := counts{}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "*", "*.csv"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		table := filepath.Base(filepath.Dir(path))
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		recs, err := csv.NewReader(f).ReadAll()
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(recs) == 0 {
			t.Fatalf("%s: no header", path)
		}
		col := slices.Index(recs[0], catalog.StreamEDIDName)
		if col < 0 {
			t.Fatalf("%s: no %s column", path, catalog.StreamEDIDName)
		}
		for _, rec := range recs[1:] {
			if c[table] == nil {
				c[table] = map[string]int64{}
			}
			c[table][rec[col]]++
		}
	}
	return c
}

// TestRawToFiles runs convert.Run from the raw Reader to the CSV Writer on
// the fixtures; the CSVs hold the baseline counts.
func TestRawToFiles(t *testing.T) {
	var inputs []string
	for _, name := range fixtures {
		inputs = append(inputs, fixturePath(name))
	}
	rdr, err := raw.NewReader(inputs)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	w, err := files.NewWriter(out, files.CSV)
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.Run(context.Background(), rdr, w, storage.Selection{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitOK {
		t.Fatalf("exit code %d, errors %+v", code, res.Errors)
	}
	if len(res.Inputs) != len(fixtures) {
		t.Errorf("inputs %+v, want the %d fixtures", res.Inputs, len(fixtures))
	}
	diffCounts(t, "CSV counts", csvCounts(t, out), baselineCounts(t))
}

// TestRawToFilesPartial: a file that fails to parse beside one that parses is
// a partial run: the bad input is failed and the good one is written.
func TestRawToFilesPartial(t *testing.T) {
	dir := t.TempDir()
	// The DFOP00 parser skips lines that are not JSON, so the garbage is one
	// line longer than its 16 MiB reader limit.
	bad := filepath.Join(dir, "bad_DFOP00.raw")
	if err := os.WriteFile(bad, []byte(strings.Repeat("x", 17<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	good := fixturePath("test_gcc1_20250822_DFOP00.raw")
	rdr, err := raw.NewReader([]string{good, bad})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	w, err := files.NewWriter(out, files.CSV)
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.Run(context.Background(), rdr, w, storage.Selection{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitPartial {
		t.Errorf("exit code %d, want %d (partial)", code, convert.ExitPartial)
	}
	status := map[string]string{}
	for _, in := range res.Inputs {
		status[filepath.Base(in.Path)] = in.Status
	}
	if status["bad_DFOP00.raw"] != "failed" || status["test_gcc1_20250822_DFOP00.raw"] != "ok" {
		t.Errorf("input statuses %v, want the bad file failed and the fixture ok", status)
	}
	if _, err := os.Stat(filepath.Join(out, "gnssa_l0", "inspva", "test_gcc1_20250822_DFOP00.csv")); err != nil {
		t.Errorf("the DFOP00 fixture's CSV: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "gnssa_l0", "inspva", "bad_DFOP00.csv")); err == nil {
		t.Error("the failed file left a CSV")
	}
}

// TestRawToFilesPartialNoOutput: a failed file beside one that parses to zero
// rows (so nothing is written) is still a partial run, not a fatal one.
func TestRawToFilesPartialNoOutput(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad_DFOP00.raw")
	if err := os.WriteFile(bad, []byte(strings.Repeat("x", 17<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := fixturePath("test_gcc1_20250821_205746_00062_NOV000.bin") // parses ok, no rows
	rdr, err := raw.NewReader([]string{empty, bad})
	if err != nil {
		t.Fatal(err)
	}
	w, err := files.NewWriter(filepath.Join(dir, "out"), files.CSV)
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.Run(context.Background(), rdr, w, storage.Selection{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outputs) != 0 {
		t.Errorf("outputs %+v, want none", res.Outputs)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitPartial {
		t.Errorf("exit code %d, want %d (partial)", code, convert.ExitPartial)
	}
}

// TestIcebergExportNoRows: exporting a scope that matches nothing is a fatal
// run with a "matched no rows" error, not a silent success.
func TestIcebergExportNoRows(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	w, err := iceberg.NewLocal(ctx, dir, datamodels.SFG, iceberg.Create())
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	r, err := iceberg.NewLocal(ctx, dir, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fw, err := files.NewWriter(t.TempDir(), files.CSV)
	if err != nil {
		t.Fatal(err)
	}
	sel := storage.Selection{Scope: edid.Scope{Network: "nomatch"}, Tables: datamodels.GNSSA_L0.Refs()}
	res, err := convert.Run(ctx, r, fw, sel)
	if err != nil {
		t.Fatal(err)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitFatal {
		t.Errorf("exit code %d, want %d (fatal)", code, convert.ExitFatal)
	}
	found := false
	for _, e := range res.Errors {
		found = found || (e.Stage == "read" && strings.Contains(e.Message, "selection matched no rows"))
	}
	if !found {
		t.Errorf("errors %+v, want a read error \"selection matched no rows\"", res.Errors)
	}
}

// TestFilesToIceberg round-trips the fixtures through each readable file
// format: raw -> files in dir A, files(A) -> a fresh local Iceberg warehouse
// (convert.Run with the files Reader), Iceberg -> CSV in dir B. B holds the
// baseline counts per table and stream, and A's reader found rows.
func TestFilesToIceberg(t *testing.T) {
	for _, format := range []files.Format{files.Parquet, files.CSV} {
		t.Run(string(format), func(t *testing.T) {
			ctx := context.Background()
			var inputs []string
			for _, name := range fixtures {
				inputs = append(inputs, fixturePath(name))
			}
			rdr, err := raw.NewReader(inputs)
			if err != nil {
				t.Fatal(err)
			}
			a := t.TempDir()
			w, err := files.NewWriter(a, format)
			if err != nil {
				t.Fatal(err)
			}
			res, err := convert.Run(ctx, rdr, w, storage.Selection{Scope: scope})
			if err != nil {
				t.Fatal(err)
			}
			if code := convert.ExitCode(res, nil); code != convert.ExitOK {
				t.Fatalf("raw to files: exit code %d, errors %+v", code, res.Errors)
			}

			fr, err := files.NewReader(a, format)
			if err != nil {
				t.Fatal(err)
			}
			if len(fr.Tables()) == 0 {
				t.Fatal("the files Reader found no tables")
			}
			warehouse := t.TempDir()
			iw, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG, iceberg.Create())
			if err != nil {
				t.Fatal(err)
			}
			res, err = convert.Run(ctx, fr, iw, storage.Selection{Scope: scope})
			if err != nil {
				t.Fatal(err)
			}
			if code := convert.ExitCode(res, nil); code != convert.ExitOK {
				t.Fatalf("files to iceberg: exit code %d, errors %+v", code, res.Errors)
			}
			// The inputs are the raw files the Parquet files record, and
			// none for CSV, which records no source.
			var fixtureInputs int
			for _, in := range res.Inputs {
				if slices.Contains(inputs, in.Path) {
					fixtureInputs++
				} else {
					t.Errorf("input %s is not a raw fixture", in.Path)
				}
			}
			if format == files.Parquet && fixtureInputs == 0 {
				t.Error("no raw input recorded from the Parquet files")
			}
			if format == files.CSV && len(res.Inputs) != 0 {
				t.Errorf("inputs %+v from CSV, want none", res.Inputs)
			}
			checkFilesProvenance(t, warehouse, format)
			if format == files.CSV {
				checkCSVRINEX(t, warehouse)
			}

			ir, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
			if err != nil {
				t.Fatal(err)
			}
			defer ir.Close()
			b := t.TempDir()
			cw, err := files.NewWriter(b, files.CSV)
			if err != nil {
				t.Fatal(err)
			}
			res, err = convert.Run(ctx, ir, cw, storage.Selection{Scope: scope, Tables: datamodels.GNSSA_L0.Refs()})
			if err != nil {
				t.Fatal(err)
			}
			if code := convert.ExitCode(res, nil); code != convert.ExitOK {
				t.Fatalf("iceberg to files: exit code %d, errors %+v", code, res.Errors)
			}
			got := csvCounts(t, b)
			if len(got) == 0 {
				t.Fatal("the round trip exported no rows")
			}
			diffCounts(t, "round-trip counts", got, baselineCounts(t))
		})
	}
}

// TestFilesReaderSelection: the files Reader applies a log source and
// columns, and a scope that matches nothing yields no batch.
func TestFilesReaderSelection(t *testing.T) {
	ctx := context.Background()
	var inputs []string
	for _, name := range fixtures {
		inputs = append(inputs, fixturePath(name))
	}
	rdr, err := raw.NewReader(inputs)
	if err != nil {
		t.Fatal(err)
	}
	a := t.TempDir()
	w, err := files.NewWriter(a, files.CSV)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := convert.Run(ctx, rdr, w, storage.Selection{Scope: scope}); err != nil {
		t.Fatal(err)
	}
	fr, err := files.NewReader(a, files.CSV)
	if err != nil {
		t.Fatal(err)
	}
	read := func(sel storage.Selection) (rows int64, cols int) {
		t.Helper()
		err := fr.Read(ctx, sel, func(b storage.Batch) error {
			if b.Err != nil {
				t.Fatal(b.Err)
			}
			defer b.Table.Release()
			if b.Source != nil {
				t.Errorf("a CSV batch has Source %+v; CSV cannot record one", *b.Source)
			}
			rows += b.Table.NumRows()
			cols = int(b.Table.NumCols())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return rows, cols
	}
	inspva := mustRef(t, "inspva")
	if n, _ := read(storage.Selection{Scope: scope, Tables: append([]catalog.TableRef(nil), inspva)}); n != 132 {
		t.Errorf("inspva rows %d, want 132", n)
	}
	obs := mustRef(t, "gnss_observations")
	one := storage.Selection{Scope: scope, Tables: append([]catalog.TableRef(nil), obs),
		LogSources: []edid.LogSource{{FileType: "NOV770", Name: "RANGECMP5B_1"}}, Columns: []string{catalog.StreamEDIDName}}
	if n, c := read(one); n != 2518 || c != 1 {
		t.Errorf("RANGECMP5B_1: %d rows, %d columns; want 2518 rows, 1 column", n, c)
	}
	if n, _ := read(storage.Selection{Scope: edid.Scope{Network: "other"}}); n != 0 {
		t.Errorf("a scope that matches nothing read %d rows", n)
	}
}

// nov770Streams are the two NOV770 antenna streams of the fixture.
var nov770Streams = []edid.LogSource{
	{FileType: fileparse.FileTypeNOV770, Name: "RANGECMP5B"},
	{FileType: fileparse.FileTypeNOV770, Name: "RANGECMP5B_1"},
}

// TestRINEXEpochsRoundTrip: the NOV770 fixture's epochs, flattened by
// BuildGNSSObs and rebuilt by rinex.ToEpochs, equal the parsed epochs under
// the table's rules (epochs and satellites without observations are not
// stored; zero code, phase, doppler and SNR are stored as null and read back
// as zero; only the loss-of-lock, half-cycle and BOC flags are stored).
func TestRINEXEpochsRoundTrip(t *testing.T) {
	for _, ls := range nov770Streams {
		t.Run(ls.Name, func(t *testing.T) {
			rows, err := nov770.ParseGNSSObs(fixturePath(fixtures[0]), ls)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				t.Fatalf("%s: the fixture parsed to no rows", ls)
			}
			tbl, err := datamodels.BuildGNSSObs(rows, scope)
			if err != nil {
				t.Fatal(err)
			}
			defer tbl.Release()
			stream, got, err := rinex.ToEpochs(tbl)
			if err != nil {
				t.Fatal(err)
			}
			if want := edid.Stream(scope, ls); stream != want {
				t.Errorf("stream %q, want %q", stream, want)
			}

			// The expected epochs: those with observations, in time order.
			var want []observation.Epoch
			for _, r := range rows {
				for _, sat := range r.Epoch.Satellites {
					if len(sat.Observations) > 0 {
						want = append(want, r.Epoch)
						break
					}
				}
			}
			slices.SortStableFunc(want, func(a, b observation.Epoch) int { return a.Time.Compare(b.Time) })
			if len(want) == 0 {
				t.Fatal("no epoch with observations to compare")
			}
			if len(got) != len(want) {
				t.Fatalf("%d epochs, want %d", len(got), len(want))
			}
			var nObs int
			for i := range want {
				w, g := want[i], got[i]
				if !g.Time.Equal(w.Time) || g.EpochPico != w.EpochPico {
					t.Fatalf("epoch %d: time %v+%dps, want %v+%dps", i, g.Time, g.EpochPico, w.Time, w.EpochPico)
				}
				if g.ReceiverClockOffset != w.ReceiverClockOffset || g.EpochFlag != w.EpochFlag {
					t.Errorf("epoch %d: clock offset %v flag %v, want %v %v", i, g.ReceiverClockOffset, g.EpochFlag, w.ReceiverClockOffset, w.EpochFlag)
				}
				var ws []*observation.Satellite
				for _, sat := range w.Satellites {
					if len(sat.Observations) > 0 {
						ws = append(ws, sat)
					}
				}
				if len(g.Satellites) != len(ws) {
					t.Fatalf("epoch %d: %d satellites, want %d", i, len(g.Satellites), len(ws))
				}
				for j, wsat := range ws {
					gsat := g.Satellites[j]
					if gsat.SatelliteKey != wsat.SatelliteKey {
						t.Fatalf("epoch %d sat %d: %v, want %v", i, j, gsat.SatelliteKey, wsat.SatelliteKey)
					}
					wantFcn := int8(0)
					if wsat.System == gnss.GLONASS {
						wantFcn = observation.FcnUnknown
						if observation.FcnValid(wsat.Fcn) {
							wantFcn = wsat.Fcn
						}
					}
					if gsat.Fcn != wantFcn {
						t.Errorf("epoch %d %v: fcn %d, want %d", i, gsat.SatelliteKey, gsat.Fcn, wantFcn)
					}
					if len(gsat.Observations) != len(wsat.Observations) {
						t.Fatalf("epoch %d %v: %d signals, want %d", i, gsat.SatelliteKey, len(gsat.Observations), len(wsat.Observations))
					}
					for k, wo := range wsat.Observations {
						nObs++
						gobs := gsat.Observations[k]
						if gobs.Code != wo.Code || gobs.Pseudorange != wo.Pseudorange || gobs.Phase != wo.Phase ||
							gobs.Doppler != wo.Doppler || gobs.SNR != wo.SNR {
							t.Fatalf("epoch %d %v %v: got %+v, want %+v", i, gsat.SatelliteKey, wo.Code, gobs, wo)
						}
						if gobs.Flags.LossOfLock() != (wo.Phase != 0 && wo.Flags.LossOfLock()) ||
							gobs.Flags.HalfCycleAmbiguity() != (wo.Phase != 0 && wo.Flags.HalfCycleAmbiguity()) ||
							gobs.Flags.BocTracking() != (wo.Phase != 0 && wo.Flags.BocTracking()) {
							t.Errorf("epoch %d %v %v: flags %v, want %v", i, gsat.SatelliteKey, wo.Code, gobs.Flags, wo.Flags)
						}
						if (gobs.LockTime == nil) != (wo.LockTime == nil) || gobs.LockTime != nil && *gobs.LockTime != *wo.LockTime {
							t.Errorf("epoch %d %v %v: lock time %v, want %v", i, gsat.SatelliteKey, wo.Code, gobs.LockTime, wo.LockTime)
						}
						if (gobs.LockTimeMax == nil) != (wo.LockTimeMax == nil) ||
							gobs.LockTimeMax != nil && *gobs.LockTimeMax != *wo.LockTimeMax {
							t.Fatalf("epoch %d %v %v: lock time bound %v, want %v", i, gsat.SatelliteKey, wo.Code, gobs.LockTimeMax, wo.LockTimeMax)
						}
					}
				}
			}
			if nObs == 0 {
				t.Fatal("no observation was compared")
			}
			if int64(nObs) != tbl.NumRows() {
				t.Errorf("compared %d observations of %d rows", nObs, tbl.NumRows())
			}
		})
	}
}

func rinexSettings(t *testing.T) *gnssrinex.Settings {
	t.Helper()
	b, err := os.ReadFile("rinex_settings.json")
	if err != nil {
		t.Fatal(err)
	}
	settings := gnssrinex.NewSettings()
	if err := json.Unmarshal(b, settings); err != nil {
		t.Fatal(err)
	}
	return settings
}

// rinexBodies is the RINEX files of dir by name, each without its
// "PGM / RUN BY / DATE" line (it holds the run date).
func rinexBodies(t *testing.T, dir string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.rnx"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var keep []string
		for _, l := range strings.Split(string(b), "\n") {
			if !strings.Contains(l, "PGM / RUN BY / DATE") {
				keep = append(keep, l)
			}
		}
		out[filepath.Base(path)] = strings.Join(keep, "\n")
	}
	return out
}

// ingestNOV770 ingests the NOV770 fixture into a fresh local warehouse.
func ingestNOV770(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	rdr, err := raw.NewReader([]string{fixturePath(fixtures[0])})
	if err != nil {
		t.Fatal(err)
	}
	w, err := iceberg.NewLocal(ctx, dir, datamodels.SFG, iceberg.Create())
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.Run(ctx, rdr, w, storage.Selection{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitOK {
		t.Fatalf("ingest: exit code %d, errors %+v", code, res.Errors)
	}
	return dir
}

// obsSelection selects the gnss_observations of scope, and of ls if it is
// not all log sources.
func obsSelection(ls edid.LogSource) storage.Selection {
	sel := storage.Selection{Scope: scope, Tables: append([]catalog.TableRef(nil), datamodels.L0GNSSObs)}
	if !ls.IsAll() {
		sel.LogSources = []edid.LogSource{ls}
	}
	return sel
}

// TestRINEXRawAndIcebergMatch: the RINEX files of one NOV770 stream written
// from the raw file and from the Iceberg table it was ingested into have the
// same names and bodies.
func TestRINEXRawAndIcebergMatch(t *testing.T) {
	ctx := context.Background()
	warehouse := ingestNOV770(t)
	for _, ls := range nov770Streams {
		t.Run(ls.Name, func(t *testing.T) {
			rawDir, icebergDir := t.TempDir(), t.TempDir()

			rdr, err := raw.NewReader([]string{fixturePath(fixtures[0])}, raw.Types(ls.FileType), raw.AsGiven())
			if err != nil {
				t.Fatal(err)
			}
			w := rinex.NewWriter(rawDir, rinexSettings(t), 0)
			res, err := convert.Run(ctx, rdr, w, obsSelection(ls))
			if err != nil {
				t.Fatal(err)
			}
			if code := convert.ExitCode(res, nil); code != convert.ExitOK {
				t.Fatalf("raw to RINEX: exit code %d, errors %+v", code, res.Errors)
			}
			if len(res.Outputs) == 0 {
				t.Fatal("raw to RINEX wrote no file")
			}

			ir, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
			if err != nil {
				t.Fatal(err)
			}
			defer ir.Close()
			iw := rinex.NewWriter(icebergDir, rinexSettings(t), 0)
			res, err = convert.Run(ctx, ir, iw, obsSelection(ls))
			if err != nil {
				t.Fatal(err)
			}
			if code := convert.ExitCode(res, nil); code != convert.ExitOK {
				t.Fatalf("iceberg to RINEX: exit code %d, errors %+v", code, res.Errors)
			}
			if len(res.Outputs) == 0 {
				t.Fatal("iceberg to RINEX wrote no file")
			}

			fromRaw, fromIceberg := rinexBodies(t, rawDir), rinexBodies(t, icebergDir)
			if len(fromRaw) == 0 {
				t.Fatal("no RINEX file from the raw file")
			}
			for name, body := range fromRaw {
				if len(body) < 1000 {
					t.Errorf("%s: only %d bytes", name, len(body))
				}
				if got, ok := fromIceberg[name]; !ok {
					t.Errorf("%s: not written from Iceberg (have %v)", name, slices.Sorted(maps.Keys(fromIceberg)))
				} else if got != body {
					t.Errorf("%s: the Iceberg body differs from the raw body", name)
				}
			}
			if len(fromIceberg) != len(fromRaw) {
				t.Errorf("%d files from Iceberg, %d from raw", len(fromIceberg), len(fromRaw))
			}
		})
	}
}

// TestRINEXTwoStreams: two streams (the fixture's two antennas) are a stream
// conflict naming both, and no file is written, from raw and from Iceberg.
func TestRINEXTwoStreams(t *testing.T) {
	ctx := context.Background()
	warehouse := ingestNOV770(t)
	open := map[string]func() storage.Reader{
		"raw": func() storage.Reader {
			rdr, err := raw.NewReader([]string{fixturePath(fixtures[0])}, raw.Types(fileparse.FileTypeNOV770), raw.AsGiven())
			if err != nil {
				t.Fatal(err)
			}
			return rdr
		},
		"iceberg": func() storage.Reader {
			ir, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { ir.Close() })
			return ir
		},
	}
	for name, mk := range open {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "rinex")
			w := rinex.NewWriter(out, rinexSettings(t), 0)
			res, err := convert.Run(ctx, mk(), w, obsSelection(edid.AllLogSources))
			if err != nil {
				t.Fatal(err)
			}
			conflict := w.Conflict()
			if !errors.Is(conflict, rinex.ErrMultipleStreams) {
				t.Fatalf("conflict %v, want rinex.ErrMultipleStreams", conflict)
			}
			for _, ls := range nov770Streams {
				if want := edid.Stream(scope, ls); !strings.Contains(conflict.Error(), want) {
					t.Errorf("conflict %q does not name %s", conflict, want)
				}
			}
			if len(res.Outputs) != 0 || len(w.Outputs()) != 0 {
				t.Errorf("outputs %+v, want none", res.Outputs)
			}
			if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
				t.Errorf("%d files written to %s", len(entries), out)
			}
			if _, err := os.Stat(out); err == nil {
				t.Errorf("%s was created", out)
			}
		})
	}
}

// fixtureSHAs is, by table name, the SHA-256 of each fixture that parses to
// rows of it.
func fixtureSHAs(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, p := range parseAll(t) {
		if p.rows.NumRows() == 0 {
			continue
		}
		src, err := catalog.HashFile(p.fixture)
		if err != nil {
			t.Fatal(err)
		}
		name := p.parser.Ref.Def.Name
		if !slices.Contains(out[name], src.SHA256) {
			out[name] = append(out[name], src.SHA256)
		}
	}
	return out
}

// checkFilesProvenance: after raw -> files -> Iceberg, each written table's
// current snapshot records the raw fixture's SHA-256 when the files were
// Parquet (which keeps it in its metadata), and no source when CSV.
func checkFilesProvenance(t *testing.T, warehouse string, format files.Format) {
	t.Helper()
	ctx := context.Background()
	ir, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer ir.Close()
	shas := fixtureSHAs(t)
	if len(shas) == 0 {
		t.Fatal("no fixture parsed to rows")
	}
	for _, name := range slices.Sorted(maps.Keys(shas)) {
		tbl, err := ir.Catalog().TableOf(mustRef(t, name))
		if err != nil {
			t.Fatal(err)
		}
		snap := tbl.Iceberg().CurrentSnapshot()
		if snap == nil || snap.Summary == nil {
			t.Errorf("%s: no current snapshot summary", name)
			continue
		}
		props := snap.Summary.Properties
		got, ok := props[catalog.PropSourceSHA256]
		switch format {
		case files.Parquet:
			if !slices.Contains(shas[name], got) {
				t.Errorf("%s: snapshot %s = %q, want the raw fixture's, one of %q", name, catalog.PropSourceSHA256, got, shas[name])
			}
		case files.CSV:
			if ok {
				t.Errorf("%s: snapshot from CSV records %s = %q, want none", name, catalog.PropSourceSHA256, got)
			}
			if f, ok := props[catalog.PropSourceFile]; ok {
				t.Errorf("%s: snapshot from CSV records %s = %q, want none", name, catalog.PropSourceFile, f)
			}
		}
	}
}

// checkCSVRINEX: the RINEX of the NOV770 RANGECMP5B stream written from a
// warehouse filled through CSV equals the RINEX written from the raw file, so
// the CSV round trip keeps every value RINEX shows, not only the counts.
func checkCSVRINEX(t *testing.T, warehouse string) {
	t.Helper()
	ctx := context.Background()
	ls := nov770Streams[0]
	rawDir, csvDir := t.TempDir(), t.TempDir()
	rdr, err := raw.NewReader([]string{fixturePath(fixtures[0])}, raw.Types(ls.FileType), raw.AsGiven())
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.Run(ctx, rdr, rinex.NewWriter(rawDir, rinexSettings(t), 0), obsSelection(ls))
	if err != nil {
		t.Fatal(err)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitOK {
		t.Fatalf("raw to RINEX: exit code %d, errors %+v", code, res.Errors)
	}
	ir, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer ir.Close()
	res, err = convert.Run(ctx, ir, rinex.NewWriter(csvDir, rinexSettings(t), 0), obsSelection(ls))
	if err != nil {
		t.Fatal(err)
	}
	if code := convert.ExitCode(res, nil); code != convert.ExitOK {
		t.Fatalf("CSV warehouse to RINEX: exit code %d, errors %+v", code, res.Errors)
	}
	fromRaw, fromCSV := rinexBodies(t, rawDir), rinexBodies(t, csvDir)
	if len(fromRaw) == 0 {
		t.Fatal("no RINEX file from the raw file")
	}
	for name, body := range fromRaw {
		if got, ok := fromCSV[name]; !ok {
			t.Errorf("%s: not written from the CSV warehouse (have %v)", name, slices.Sorted(maps.Keys(fromCSV)))
		} else if got != body {
			t.Errorf("%s: the RINEX body from the CSV warehouse differs from the raw body", name)
		}
	}
	if len(fromCSV) != len(fromRaw) {
		t.Errorf("%d files from the CSV warehouse, %d from raw", len(fromCSV), len(fromRaw))
	}
}

// streamRows counts the rows of tbl by stream_edid, scanning the whole table.
func streamRows(t *testing.T, tbl *catalog.Table) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	filter, err := storage.Selection{Scope: scope}.Filter(tbl.Def())
	if err != nil {
		t.Fatal(err)
	}
	schema, batches, err := tbl.Scan(ctx, filter, []string{catalog.StreamEDIDName})
	if err != nil {
		t.Fatal(err)
	}
	var recs []arrow.RecordBatch
	for rec, err := range batches {
		if err != nil {
			t.Fatal(err)
		}
		recs = append(recs, rec)
	}
	all := array.NewTableFromRecords(schema, recs)
	for _, r := range recs {
		r.Release()
	}
	defer all.Release()
	out, err := streamCounts(all)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// withTime is the first row of rows with its column col set to ts
// (microseconds). The caller releases it.
func withTime(t *testing.T, rows arrow.Table, col string, ts arrow.Timestamp) arrow.Table {
	t.Helper()
	rdr := array.NewTableReader(rows, 1)
	defer rdr.Release()
	if !rdr.Next() {
		t.Fatal("no rows")
	}
	rec := rdr.RecordBatch()
	idx := rec.Schema().FieldIndices(col)
	if len(idx) != 1 {
		t.Fatalf("no column %q", col)
	}
	cols := make([]arrow.Array, rec.NumCols())
	for i := range cols {
		cols[i] = rec.Column(i)
	}
	bld := array.NewTimestampBuilder(memory.DefaultAllocator, rec.Schema().Field(idx[0]).Type.(*arrow.TimestampType))
	bld.Append(ts)
	tsArr := bld.NewArray()
	bld.Release()
	defer tsArr.Release()
	cols[idx[0]] = tsArr
	out := array.NewRecordBatch(rec.Schema(), cols, 1)
	defer out.Release()
	return array.NewTableFromRecords(rec.Schema(), []arrow.RecordBatch{out})
}

// TestReplaceRejectsInsaneTime: a Replace with a row timed in 1970 (a packet
// with no time) fails with an error naming the stream, the column and the
// time, and the stream's rows already in the table are left intact, rather
// than the replace window widening back to 1970 and deleting them.
func TestReplaceRejectsInsaneTime(t *testing.T) {
	ctx := context.Background()
	warehouse := ingestNOV770(t)
	ice, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer ice.Close()
	tbl, err := ice.Catalog().TableOf(datamodels.L0GNSSObs)
	if err != nil {
		t.Fatal(err)
	}
	before := streamRows(t, tbl)
	if len(before) == 0 {
		t.Fatal("the warehouse holds no gnss_observations rows")
	}

	var parsedRows arrow.Table
	for _, p := range parseAll(t) {
		if p.parser.Ref.Def.Name == datamodels.L0GNSSObs.Def.Name && p.rows.NumRows() > 0 {
			parsedRows = p.rows
			break
		}
	}
	if parsedRows == nil {
		t.Fatal("no parsed gnss_observations rows")
	}
	col := tbl.Def().TimeColumn
	for _, bad := range []arrow.Timestamp{0, arrow.Timestamp(time.Now().Add(48 * time.Hour).UnixMicro())} {
		rows := withTime(t, parsedRows, col, bad)
		stream := ""
		if m, err := streamCounts(rows); err == nil {
			for s := range m {
				stream = s
			}
		}
		_, err = tbl.Replace(ctx, rows)
		rows.Release()
		if err == nil {
			t.Fatalf("Replace with %s = %v succeeded; want an error", col, time.UnixMicro(int64(bad)).UTC())
		}
		for _, want := range []string{stream, col, time.UnixMicro(int64(bad)).UTC().Format("2006-01-02")} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %q", err, want)
			}
		}
	}

	// Reload, so the counts are of the catalog's current snapshot.
	ice2, err := iceberg.NewLocal(ctx, warehouse, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer ice2.Close()
	tbl2, err := ice2.Catalog().TableOf(datamodels.L0GNSSObs)
	if err != nil {
		t.Fatal(err)
	}
	if after := streamRows(t, tbl2); !maps.Equal(after, before) {
		t.Errorf("stream rows after the refused Replace %v, want %v", after, before)
	}
}

// dirFiles is every file under dir with its SHA-256.
func dirFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		src, err := catalog.HashFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[rel] = src.SHA256
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestWarehouseGuardLocal reproduces "existing tables outside the configured
// warehouse" with local warehouses: a catalog built in warehouse A, opened
// through a copy of its catalog.db in warehouse B, so its tables are at A
// while B is claimed. Building (Create) through B is refused naming a table,
// its location and B, and nothing is written in either warehouse. Loading
// through B, and building through A, still work.
func TestWarehouseGuardLocal(t *testing.T) {
	ctx := context.Background()
	a := ingestNOV770(t)
	b := t.TempDir()
	db, err := os.ReadFile(filepath.Join(a, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "catalog.db"), db, 0o644); err != nil {
		t.Fatal(err)
	}
	beforeA, beforeB := dirFiles(t, a), dirFiles(t, b)

	_, err = iceberg.NewLocal(ctx, b, datamodels.SFG, iceberg.Create())
	if err == nil {
		t.Fatal("building through warehouse B, whose tables are in A, succeeded; want the guard's error")
	}
	for _, want := range []string{"outside the warehouse", b, "--database-prefix"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if !strings.Contains(err.Error(), filepath.Base(a)) {
		t.Errorf("error %q does not name the table's location in %s", err, a)
	}
	if afterA := dirFiles(t, a); !maps.Equal(afterA, beforeA) {
		t.Errorf("warehouse A changed: %d files before, %d after", len(beforeA), len(afterA))
	}
	if afterB := dirFiles(t, b); !maps.Equal(afterB, beforeB) {
		t.Errorf("warehouse B changed: %v, want only catalog.db unchanged", slices.Sorted(maps.Keys(afterB)))
	}

	// Loading reads wherever the tables are; the guard is for building.
	r, err := iceberg.NewLocal(ctx, b, datamodels.SFG)
	if err != nil {
		t.Fatalf("loading through B: %v", err)
	}
	r.Close()
	// The warehouse the tables live in builds as before, also given with a
	// trailing slash.
	for _, wh := range []string{a, a + "/"} {
		w, err := iceberg.NewLocal(ctx, wh, datamodels.SFG, iceberg.Create())
		if err != nil {
			t.Fatalf("building through %s: %v", wh, err)
		}
		w.Close()
	}
}
