package convert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/parquet/file"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
)

// dfop00Sample is two DFOP00 ping/reply cycles, as in the dfop00 tests.
const dfop00Sample = `{"event":"interrogation","event_id":1,"observations":{"AHRS":{"h":10.5,"p":1,"r":-2,"time":{"common":1780460559.981377,"instrument":8536.68,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3"},"sequence":1,"time":{"common":1780460560.0,"instrument":8536.69,"start_count":45,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":1,"observations":{"AHRS":{"h":10.5,"p":1,"r":-2,"time":{"common":1780460559.981377,"instrument":8536.68,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3"},"range":{"cn":"IR5209","diag":{"dbv":[-12.0],"snr":[18.5],"xc":[87.0]},"range":2.534127,"tat":200},"sequence":1,"time":{"common":1780460562.404127,"instrument":8539.1,"start_count":45,"status":"GPS_SYNC"},"uid":"0042D9"}
{"event":"interrogation","event_id":2,"observations":{"AHRS":"ERR1","GNSS":"ERR3"},"sequence":2,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"type":"BR"}
{"event":"range","event_id":2,"observations":{"AHRS":{"h_mag":68.6,"time":{"common":1780460575.5,"instrument":8552.2,"start_count":45,"status":"GPS_SYNC"}},"GNSS":"ERR3"},"range":{"cn":"IR5210","diag":{"dbv":0,"snr":0,"xc":21},"range":0.0,"tat":200},"sequence":2,"time":{"common":1780460575.0,"instrument":8551.7,"start_count":45,"status":"GPS_SYNC"},"uid":"0042DA"}
`

// rawDir is a campaign raw directory with two DFOP00 files and files a
// DFOP00 source must not read.
func rawDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"a_DFOP00.raw":   dfop00Sample,
		"b_DFOP00.raw":   dfop00Sample,
		"._a_DFOP00.raw": "AppleDouble",
		"c_AHRS00.bin":   "not DFOP00",
		"notes.txt":      "not DFOP00",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// sampleRows is the rows each DFOP00 parser makes of one sample file.
func sampleRows(t *testing.T) map[string]int64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x_DFOP00.raw")
	if err := os.WriteFile(path, []byte(dfop00Sample), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := map[string]int64{}
	for _, p := range fileparse.ForFileType("DFOP00") {
		tbl, err := p.Parse(path, scope)
		if err != nil {
			t.Fatal(err)
		}
		rows[p.Table.Namespace()+"."+p.Table.Name] = tbl.NumRows()
		tbl.Release()
	}
	return rows
}

func rawSpec(uri, sink string) Spec {
	return Spec{
		Source: SourceRaw, SourceURIs: []string{uri}, SourceTypes: []string{"DFOP00"},
		Sink: SinkFiles, SinkURI: sink, TableFormat: FormatArrow, Scope: scope,
	}
}

func outputRows(res *Result) map[string]int64 {
	rows := map[string]int64{}
	for _, o := range res.Outputs {
		rows[o.Table] = o.Rows
	}
	return rows
}

func TestRawToFiles(t *testing.T) {
	dir, want := rawDir(t), sampleRows(t)
	if want["gnssa_l1.travel_times"] == 0 {
		t.Fatal("sample has no travel times")
	}
	for _, format := range []string{FormatArrow, FormatCSV, FormatParquet} {
		t.Run(format, func(t *testing.T) {
			s := rawSpec(dir, t.TempDir())
			s.TableFormat = format
			res := run(t, s)
			if !res.OK {
				t.Fatalf("errors %+v", res.Errors)
			}
			// Every table DFOP00 feeds, with both files' rows.
			got := outputRows(res)
			if len(got) != len(want) {
				t.Errorf("tables %v, want %v", got, want)
			}
			for table, n := range want {
				if got[table] != 2*n {
					t.Errorf("%s: %d rows, want %d", table, got[table], 2*n)
				}
			}
			// The rows read back from the files.
			path := filepath.Join(s.SinkURI, "travel_times."+format)
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var n int64
			switch format {
			case FormatArrow:
				r, err := ipc.NewFileReader(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				for i := range r.NumRecords() {
					rec, err := r.Record(i)
					if err != nil {
						t.Fatal(err)
					}
					n += rec.NumRows()
				}
				r.Close()
			case FormatCSV:
				n = int64(strings.Count(string(b), "\n") - 1)
			case FormatParquet:
				pf, err := file.NewParquetReader(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				n = pf.NumRows()
			}
			if n != 2*want["gnssa_l1.travel_times"] {
				t.Errorf("%s holds %d rows, want %d", path, n, 2*want["gnssa_l1.travel_times"])
			}
		})
	}
}

func TestRawInputs(t *testing.T) {
	dir := rawDir(t)
	files := func(res *Result) []string {
		var out []string
		for _, in := range res.Inputs {
			if in.Type != "DFOP00" || in.Status != "ok" {
				t.Errorf("input %+v", in)
			}
			out = append(out, filepath.Base(in.Path))
		}
		return out
	}
	for _, tc := range []struct {
		name string
		edit func(*Spec)
		want string
	}{
		{"directory", func(*Spec) {}, "a_DFOP00.raw,b_DFOP00.raw"},
		{"max files", func(s *Spec) { s.MaxFiles = 1 }, "a_DFOP00.raw"},
		{"glob", func(s *Spec) { s.SourceURIs = []string{filepath.Join(dir, "b_*.raw")} }, "b_DFOP00.raw"},
		{"file", func(s *Spec) { s.SourceURIs = []string{filepath.Join(dir, "a_DFOP00.raw")} }, "a_DFOP00.raw"},
		{"files", func(s *Spec) {
			s.SourceURIs = []string{filepath.Join(dir, "b_DFOP00.raw"), filepath.Join(dir, "a_DFOP00.raw")}
		}, "a_DFOP00.raw,b_DFOP00.raw"},
		{"overlapping", func(s *Spec) {
			s.SourceURIs = []string{dir, filepath.Join(dir, "a_*.raw")} // a_DFOP00.raw once
		}, "a_DFOP00.raw,b_DFOP00.raw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := rawSpec(dir, t.TempDir())
			tc.edit(&s)
			if got := strings.Join(files(run(t, s)), ","); got != tc.want {
				t.Errorf("inputs %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRawFilters(t *testing.T) {
	dir, want := rawDir(t), sampleRows(t)

	s := rawSpec(dir, t.TempDir())
	s.Tables = []string{"travel_times"}
	got := outputRows(run(t, s))
	if len(got) != 1 || got["gnssa_l1.travel_times"] != 2*want["gnssa_l1.travel_times"] {
		t.Errorf("--table travel_times: %v", got)
	}

	// --log-source keeps the rows of that log source's streams.
	all := run(t, rawSpec(dir, t.TempDir()))
	wantLS := map[string]int64{}
	for _, st := range all.Streams {
		if strings.HasSuffix(st.EDID, ":DFOP00") {
			wantLS[st.Table] += st.Rows
		}
	}
	s = rawSpec(dir, t.TempDir())
	s.LogSource = "DFOP00"
	got = outputRows(run(t, s))
	for table, n := range got {
		if n != wantLS[table] {
			t.Errorf("--log-source DFOP00: %s has %d rows, want %d", table, n, wantLS[table])
		}
	}
	s = rawSpec(dir, t.TempDir())
	s.LogSource = "NOPE"
	for table, n := range outputRows(run(t, s)) {
		if n != 0 {
			t.Errorf("--log-source NOPE: %s has %d rows", table, n)
		}
	}
}

func TestRawBadFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	dir := rawDir(t)
	bad := filepath.Join(dir, "b_DFOP00.raw")
	if err := os.Chmod(bad, 0); err != nil {
		t.Fatal(err)
	}
	s := rawSpec(dir, t.TempDir())
	res := run(t, s)
	if res.OK || ExitCode(res, nil) != ExitPartial {
		t.Fatalf("ok %v, exit %d; want a partial run", res.OK, ExitCode(res, nil))
	}
	want := sampleRows(t)
	if got := outputRows(res); got["gnssa_l1.travel_times"] != want["gnssa_l1.travel_times"] {
		t.Errorf("good file's rows: %v", got)
	}
	var failed []Input
	for _, in := range res.Inputs {
		if in.Status == "failed" {
			failed = append(failed, in)
		}
	}
	if len(failed) != 1 || failed[0].Path != bad || failed[0].Error == "" {
		t.Errorf("failed inputs %+v, want %s", failed, bad)
	}
	if len(res.Errors) == 0 || res.Errors[0].Stage != "parse" || res.Errors[0].Subject != bad {
		t.Errorf("errors %+v", res.Errors)
	}
}

func TestRawErrors(t *testing.T) {
	dir := rawDir(t)
	for _, tc := range []struct {
		name string
		edit func(*Spec)
	}{
		{"partial scope", func(s *Spec) { s.Scope = datamodels.Scope{Network: "net", Station: "STA"} }},
		{"no type", func(s *Spec) { s.SourceTypes = nil }},
		{"unknown type", func(s *Spec) { s.SourceTypes = []string{"NOPE"} }},
		{"type twice", func(s *Spec) { s.SourceTypes = []string{"DFOP00", "dfop00"} }},
		{"table not produced", func(s *Spec) { s.Tables = []string{"inspva"} }},
		{"derived table", func(s *Spec) { s.Tables = []string{"gnssa_l2.garpos"} }},
		{"glob with two types", func(s *Spec) {
			s.SourceURIs, s.SourceTypes = []string{filepath.Join(dir, "*.raw")}, []string{"DFOP00", "AHRS00"}
		}},
		{"negative max files", func(s *Spec) { s.MaxFiles = -1 }},
		{"file with two types", func(s *Spec) {
			s.SourceURIs, s.SourceTypes = []string{dir, filepath.Join(dir, "a_DFOP00.raw")}, []string{"DFOP00", "AHRS00"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := rawSpec(dir, t.TempDir())
			tc.edit(&s)
			_, err := New(s)
			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Errorf("got %v, want a usage error", err)
			}
		})
	}

	// Flags that belong to an Iceberg source don't apply to a raw one.
	given := []string{"source", "source-uri", "source-type", "sink", "sink-uri", "table-format", "scope"}
	if err := CheckFlags(SourceRaw, SinkFiles, given); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"source-catalog", "start", "end", "columns"} {
		if err := CheckFlags(SourceRaw, SinkFiles, append(given, name)); err == nil {
			t.Errorf("--%s with --source raw: no error", name)
		}
	}

	// No files of the type is fatal, not a usage error.
	s := rawSpec(t.TempDir(), t.TempDir())
	c, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Run(t.Context())
	if err == nil || ExitCode(res, err) != ExitFatal {
		t.Errorf("empty directory: %v, exit %d", err, ExitCode(res, err))
	}
}
