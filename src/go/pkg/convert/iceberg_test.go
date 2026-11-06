package convert

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/apache/iceberg-go/config"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

func rawToIcebergSpec(raw, wh string) Spec {
	return Spec{
		Source: SourceRaw, SourceURIs: []string{raw}, SourceTypes: []string{"DFOP00"},
		Sink: SinkIceberg, SinkURI: wh, SinkCatalog: catalog.Local, Scope: scope,
	}
}

// tableRows is the rows of each table in the warehouse's catalog.
func tableRows(t *testing.T, wh string, defs ...datamodels.TableDef) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: wh})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	out := map[string]int64{}
	for _, def := range defs {
		_, batches, err := cat.Scan(ctx, def, catalog.Selection{Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		for rec, err := range batches {
			if err != nil {
				t.Fatal(err)
			}
			out[def.Namespace()+"."+def.Name] += rec.NumRows()
			rec.Release()
		}
	}
	return out
}

// oneFileDir is a raw directory holding one DFOP00 sample file.
func oneFileDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a_DFOP00.raw"), []byte(dfop00Sample), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

var dfop00Tables = []datamodels.TableDef{datamodels.TWTT, datamodels.INSPVA}

func TestRawToIceberg(t *testing.T) {
	raw, wh, want := oneFileDir(t), t.TempDir(), sampleRows(t)
	for range 2 { // the second run replaces the first run's rows
		res := run(t, rawToIcebergSpec(raw, wh))
		if !res.OK {
			t.Fatalf("errors %+v", res.Errors)
		}
		if got := outputRows(res); got["gnssa_l0.travel_times"] != want["gnssa_l0.travel_times"] {
			t.Errorf("outputs %v, want %v", got, want)
		}
		if got := tableRows(t, wh, dfop00Tables...); got["gnssa_l0.travel_times"] != want["gnssa_l0.travel_times"] ||
			got["gnssa_l0.inspva"] != want["gnssa_l0.inspva"] {
			t.Errorf("catalog holds %v, want %v", got, want)
		}
	}

	// Each snapshot records the file its rows came from.
	ctx := context.Background()
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: wh})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tbl, err := cat.Table(ctx, datamodels.TWTT)
	if err != nil {
		t.Fatal(err)
	}
	if p := tbl.CurrentSnapshot().Summary.Properties; p[catalog.PropSourceFile] != filepath.Join(raw, "a_DFOP00.raw") {
		t.Errorf("snapshot source %q", p[catalog.PropSourceFile])
	}
}

// Groups are committed --files-per-commit at a time, all of a table's at
// once by default, with the same rows and one snapshot per file either way.
func TestFilesPerCommit(t *testing.T) {
	raw := t.TempDir()
	for _, name := range []string{"a_DFOP00.raw", "b_DFOP00.raw", "c_DFOP00.raw"} {
		if err := os.WriteFile(filepath.Join(raw, name), []byte(dfop00Sample), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := sampleRows(t)["gnssa_l0.travel_times"] // each file replaces the one before
	commits := map[int]int{}
	for _, perCommit := range []int{0, 1, 2} {
		wh := t.TempDir()
		s := rawToIcebergSpec(raw, wh)
		s.Tables, s.FilesPerCommit = []string{"travel_times"}, perCommit
		if res := run(t, s); !res.OK {
			t.Fatalf("--files-per-commit %d: errors %+v", perCommit, res.Errors)
		}
		if got := tableRows(t, wh, datamodels.TWTT)["gnssa_l0.travel_times"]; got != want {
			t.Errorf("--files-per-commit %d: %d rows, want %d", perCommit, got, want)
		}
		ctx := context.Background()
		cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: wh})
		if err != nil {
			t.Fatal(err)
		}
		tbl, err := cat.Table(ctx, datamodels.TWTT)
		if err != nil {
			t.Fatal(err)
		}
		var files []string
		for snap := tbl.CurrentSnapshot(); snap != nil; {
			files = append(files, filepath.Base(snap.Summary.Properties[catalog.PropSourceFile]))
			if snap.ParentSnapshotID == nil {
				break
			}
			snap = tbl.Metadata().SnapshotByID(*snap.ParentSnapshotID)
		}
		if got := strings.Join(files, ","); got != "c_DFOP00.raw,b_DFOP00.raw,a_DFOP00.raw" {
			t.Errorf("--files-per-commit %d: snapshots of %s", perCommit, got)
		}
		for range tbl.Metadata().PreviousFiles() {
			commits[perCommit]++
		}
		cat.Close()
	}
	// Three files: one commit, three, or two, after the table's creation.
	if commits[1]-commits[0] != 2 || commits[2]-commits[0] != 1 {
		t.Errorf("metadata versions by --files-per-commit: %v", commits)
	}
}

func TestRawToIcebergPartial(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	raw, wh, want := oneFileDir(t), t.TempDir(), sampleRows(t)
	bad := filepath.Join(raw, "b_DFOP00.raw")
	if err := os.WriteFile(bad, []byte(dfop00Sample), 0); err != nil {
		t.Fatal(err)
	}
	res := run(t, rawToIcebergSpec(raw, wh))
	if code := ExitCode(res, nil); code != ExitPartial {
		t.Fatalf("exit code %d, want %d; errors %+v", code, ExitPartial, res.Errors)
	}
	if got := tableRows(t, wh, datamodels.TWTT); got["gnssa_l0.travel_times"] != want["gnssa_l0.travel_times"] {
		t.Errorf("good file's rows not committed: %v", got)
	}
}

// TestCatalogWarehouseDefault: a Spec's Iceberg endpoint without a URI
// takes its catalog's configured warehouse (as sfg iceberg passes it); a
// local catalog, or one configuring none, still needs one. The convert
// flags still require every URI.
func TestCatalogWarehouseDefault(t *testing.T) {
	saved := config.EnvConfig.Catalogs
	t.Cleanup(func() { config.EnvConfig.Catalogs = saved })
	config.EnvConfig.Catalogs = map[string]config.CatalogConfig{
		"cloud": {CatalogType: "glue", Warehouse: "s3://bucket/wh"},
		"bare":  {CatalogType: "glue"},
	}
	if err := CheckFlags(SourceRaw, SinkIceberg, []string{"source", "source-uri", "source-type", "sink", "sink-catalog", "scope"}); err == nil {
		t.Error("convert without --sink-uri: no error")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x_DFOP00.raw"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := rawSpec(filepath.Join(dir, "x_DFOP00.raw"), "")
	spec.Sink, spec.TableFormat, spec.SinkCatalog = SinkIceberg, "", "cloud"
	c, err := New(spec)
	if err != nil {
		t.Fatal(err)
	}
	if c.spec.SinkURI != "s3://bucket/wh" {
		t.Errorf("sink URI %q, want the configured warehouse", c.spec.SinkURI)
	}
	for _, name := range []string{"bare", catalog.Local} {
		spec.SinkCatalog = name
		var usage *UsageError
		if _, err := New(spec); !errors.As(err, &usage) || !strings.Contains(err.Error(), "--sink-uri is required") {
			t.Errorf("--sink-catalog %s without --sink-uri: got %v", name, err)
		}
	}
}

func TestIcebergSinkErrors(t *testing.T) {
	given := []string{"source", "source-uri", "source-type", "sink", "sink-uri", "sink-catalog", "scope"}
	if err := CheckFlags(SourceRaw, SinkIceberg, given); err != nil {
		t.Fatal(err)
	}
	if err := CheckFlags(SourceRaw, SinkIceberg, given[:len(given)-2]); err == nil {
		t.Error("no --sink-catalog: no error")
	}
	if err := CheckFlags(SourceRaw, SinkIceberg, append(given, "table-format")); err == nil {
		t.Error("--table-format with an Iceberg sink: no error")
	}
	if err := CheckFlags(SourceRaw, SinkFiles, []string{"source", "source-uri", "source-type", "sink", "sink-uri", "table-format", "scope", "files-per-commit"}); err == nil {
		t.Error("--files-per-commit with a files sink: no error")
	}
	copyFlags := []string{"source", "source-uri", "source-catalog", "table", "sink", "sink-uri", "sink-catalog", "scope"}
	if err := CheckFlags(SourceIceberg, SinkIceberg, copyFlags); err != nil {
		t.Errorf("catalog copy: %v", err)
	}
	if err := CheckFlags(SourceIceberg, SinkIceberg, append(copyFlags, "columns")); err == nil {
		t.Error("--columns into a catalog: no error")
	}
	for _, tc := range []struct {
		name string
		edit func(*Spec)
	}{
		{"unknown catalog", func(s *Spec) { s.SinkCatalog = "no-such-catalog" }},
		{"negative files per commit", func(s *Spec) { s.FilesPerCommit = -1 }},
		{"two warehouses", func(s *Spec) {
			s.Source, s.SourceCatalog, s.SourceTypes, s.Tables = SourceIceberg, catalog.Local, nil, []string{"inspva"}
			s.SourceURIs = []string{t.TempDir(), t.TempDir()}
		}},
		{"copy onto itself", func(s *Spec) {
			s.Source, s.SourceCatalog, s.SourceTypes, s.Tables = SourceIceberg, catalog.Local, nil, []string{"inspva"}
			s.SourceURIs = []string{s.SinkURI + "/"}
		}},
	} {
		s := rawToIcebergSpec(oneFileDir(t), t.TempDir())
		tc.edit(&s)
		var usage *UsageError
		if _, err := New(s); !errors.As(err, &usage) {
			t.Errorf("%s: got %v, want a usage error", tc.name, err)
		}
	}
}

func copySpec(src, dst string) Spec {
	return Spec{
		Source: SourceIceberg, SourceURIs: []string{src}, SourceCatalog: catalog.Local, Tables: []string{"attitude_ahrs"},
		Sink: SinkIceberg, SinkURI: dst, SinkCatalog: catalog.Local, Scope: scope,
	}
}

// csvRows is the sorted CSV rows of a warehouse's attitude_ahrs table, with
// its columns in definition order.
func csvRows(t *testing.T, wh string) []string {
	t.Helper()
	out := t.TempDir()
	s := spec(wh, out)
	for _, c := range datamodels.AttitudeAHRS.Columns {
		s.Columns = append(s.Columns, c.Name)
	}
	run(t, s)
	b, err := os.ReadFile(filepath.Join(out, "attitude_ahrs.csv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	slices.Sort(lines[1:])
	return lines
}

func TestCopy(t *testing.T) {
	src, dst := warehouse(t), t.TempDir() // src: 10 AHRS00 and 4 DFOP00 rows
	for range 2 {                         // the second run replaces the first run's rows
		res := run(t, copySpec(src, dst))
		if !res.OK || len(res.Streams) != 2 {
			t.Fatalf("ok %v, streams %+v, errors %+v", res.OK, res.Streams, res.Errors)
		}
		if got, want := csvRows(t, dst), csvRows(t, src); !slices.Equal(got, want) {
			t.Fatalf("copy holds\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}

	// Each snapshot records where its rows came from.
	ctx := context.Background()
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: dst})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tbl, err := cat.Table(ctx, datamodels.AttitudeAHRS)
	if err != nil {
		t.Fatal(err)
	}
	p := tbl.CurrentSnapshot().Summary.Properties
	if p[PropCopySource] != catalog.Local+":"+src || p[PropCopySelection] != "--scope net/STA/2026_A_1126" {
		t.Errorf("snapshot summary %v", p)
	}
}

// A windowed copy replaces the sink's rows of the stream only over the
// copied rows' span.
func TestCopyWindow(t *testing.T) {
	ctx := context.Background()
	src, dst := t.TempDir(), t.TempDir()
	write := func(wh string, start time.Time, n int) {
		cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: wh})
		if err != nil {
			t.Fatal(err)
		}
		defer cat.Close()
		rows := attitudeRows(t, ahrsEDID, start, n)
		defer rows.Release()
		if _, err := cat.Append(ctx, datamodels.AttitudeAHRS, rows); err != nil {
			t.Fatal(err)
		}
	}
	write(dst, t0, 10)                           // sink: :31 to :40, whole seconds
	write(src, t0.Add(500*time.Millisecond), 10) // source: :31.5 to :40.5
	s := copySpec(src, dst)
	s.Start, s.End = t0.Add(2*time.Second), t0.Add(5*time.Second) // copies :33.5, :34.5 and :35.5
	if res := run(t, s); !res.OK {
		t.Fatalf("errors %+v", res.Errors)
	}
	// Sink rows :34 and :35 fall in the copied span [:33.5, :35.5] and are
	// replaced; the rest, :33 included, are outside it and stay.
	out := t.TempDir()
	read := spec(dst, out)
	read.Columns = []string{"time"}
	run(t, read)
	b, err := os.ReadFile(filepath.Join(out, "attitude_ahrs.csv"))
	if err != nil {
		t.Fatal(err)
	}
	var secs []string
	for _, ts := range strings.Split(strings.TrimSpace(string(b)), "\n")[1:] {
		secs = append(secs, strings.TrimSuffix(strings.TrimPrefix(ts, "2026-06-02T19:54:"), "Z"))
	}
	slices.Sort(secs)
	if got, want := strings.Join(secs, " "), "31 32 33 33.5 34.5 35.5 36 37 38 39 40"; got != want {
		t.Errorf("sink seconds %s, want %s", got, want)
	}
}

// A sink table created by an older definition gains the columns it lacks.
func TestCopyEvolvesSink(t *testing.T) {
	ctx := context.Background()
	src, dst := warehouse(t), t.TempDir()
	old := datamodels.AttitudeAHRS
	old.Columns = slices.DeleteFunc(slices.Clone(old.Columns), func(c datamodels.Column) bool { return c.Name == "h_mag" })
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: dst})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Table(ctx, old); err != nil {
		t.Fatal(err)
	}
	cat.Close()

	if res := run(t, copySpec(src, dst)); !res.OK {
		t.Fatalf("errors %+v", res.Errors)
	}
	if got, want := csvRows(t, dst), csvRows(t, src); !slices.Equal(got, want) {
		t.Errorf("copy into an older table holds\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// shifted is dfop00Sample 10000 s later per step.
func shifted(steps int) string {
	s := dfop00Sample
	for range steps {
		s = strings.NewReplacer("178046", "178047", "178047", "178048", "146449", "146450", "146450", "146451").Replace(s)
	}
	return s
}

// TestFilesPerSnapshot: files merge into snapshots of up to
// --files-per-snapshot, and a file overlapping one already merged starts
// the next, so it replaces that file as if written alone.
func TestFilesPerSnapshot(t *testing.T) {
	raw := t.TempDir()
	for name, content := range map[string]string{
		"a_DFOP00.raw": dfop00Sample, "b_DFOP00.raw": shifted(1), "c_DFOP00.raw": shifted(2),
		"d_DFOP00.raw": dfop00Sample, // a again
	} {
		if err := os.WriteFile(filepath.Join(raw, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wh := t.TempDir()
	s := rawToIcebergSpec(raw, wh)
	s.Tables, s.FilesPerSnapshot, s.Workers = []string{"travel_times"}, 3, 2
	if res := run(t, s); !res.OK {
		t.Fatalf("errors %+v", res.Errors)
	}
	if got, want := tableRows(t, wh, datamodels.TWTT)["gnssa_l0.travel_times"], 3*sampleRows(t)["gnssa_l0.travel_times"]; got != want {
		t.Errorf("%d rows, want %d (a, b, c; d replacing a)", got, want)
	}
	ctx := context.Background()
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: wh})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tbl, err := cat.Table(ctx, datamodels.TWTT)
	if err != nil {
		t.Fatal(err)
	}
	last := tbl.CurrentSnapshot()
	if filepath.Base(last.Summary.Properties[catalog.PropSourceFile]) != "d_DFOP00.raw" || last.ParentSnapshotID == nil {
		t.Fatalf("last snapshot %v, want d alone after a merged one", last.Summary.Properties)
	}
	var merged []catalog.SourceFile
	first := tbl.Metadata().SnapshotByID(*last.ParentSnapshotID)
	if err := json.Unmarshal([]byte(first.Summary.Properties[catalog.PropSourceFiles]), &merged); err != nil || len(merged) != 3 {
		t.Errorf("first snapshot files %v, %v; want a, b, c", merged, err)
	}
}
