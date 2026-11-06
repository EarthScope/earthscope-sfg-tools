package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

var update = flag.Bool("update", false, "rewrite golden files")

var (
	t0    = time.Date(2026, 6, 2, 19, 54, 31, 0, time.UTC)
	scope = datamodels.Scope{Network: "net", Station: "STA", Campaign: "2026_A_1126"}
)

const ahrsEDID, dfopEDID = "net.STA.2026_A_1126.AHRS00.AHRS00", "net.STA.2026_A_1126.DFOP00.DFOP00"

// attitudeRows is n attitude_ahrs rows of stream edid, one a second from
// start, with h null.
func attitudeRows(t *testing.T, edid string, start time.Time, n int) arrow.Table {
	t.Helper()
	sc, err := datamodels.AttitudeAHRS.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	set := func(name string, v any) {
		switch fb := b.Field(sc.FieldIndices(name)[0]).(type) {
		case *array.StringBuilder:
			fb.Append(v.(string))
		case *array.TimestampBuilder:
			fb.Append(v.(arrow.Timestamp))
		case *array.Float64Builder:
			fb.Append(v.(float64))
		}
	}
	for i := range n {
		set("stream_edid", edid)
		set("time", arrow.Timestamp(start.Add(time.Duration(i)*time.Second).UnixMicro()))
		set("time_instrument", float64(i))
	}
	for i, f := range sc.Fields() {
		if fb := b.Field(i); fb.Len() < n {
			if !f.Nullable {
				t.Fatalf("required column %s not set", f.Name)
			}
			fb.AppendNulls(n - fb.Len())
		}
	}
	rec := b.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

// warehouse is a SQL warehouse with 10 AHRS00 and 4 DFOP00 attitude rows.
func warehouse(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	cat, err := catalog.Open(ctx, catalog.Config{Name: catalog.Local, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, s := range []struct {
		edid string
		n    int
	}{{ahrsEDID, 10}, {dfopEDID, 4}} {
		rows := attitudeRows(t, s.edid, t0, s.n)
		if _, err := cat.Append(ctx, datamodels.AttitudeAHRS, rows); err != nil {
			t.Fatal(err)
		}
		rows.Release()
	}
	return dir
}

func spec(src, sink string) Spec {
	return Spec{
		Source: SourceIceberg, SourceURIs: []string{src}, SourceCatalog: catalog.Local,
		Sink: SinkFiles, SinkURI: sink, TableFormat: FormatCSV,
		Scope: scope, Tables: []string{"attitude_ahrs"},
	}
}

func run(t *testing.T, s Spec) *Result {
	t.Helper()
	c, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestCheckFlags(t *testing.T) {
	full := []string{"source", "source-uri", "source-catalog", "table", "sink", "sink-uri", "table-format", "scope"}
	if err := CheckFlags(SourceIceberg, SinkFiles, full); err != nil {
		t.Fatalf("all required flags: %v", err)
	}
	optional := append(full, "source-type", "log-source", "start", "end", "columns", "result")
	if err := CheckFlags(SourceIceberg, SinkFiles, optional); err != nil {
		t.Fatalf("all optional flags: %v", err)
	}
	for _, name := range full {
		t.Run("missing "+name, func(t *testing.T) {
			var given []string
			for _, g := range full {
				if g != name {
					given = append(given, g)
				}
			}
			err := CheckFlags(SourceIceberg, SinkFiles, given)
			var usage *UsageError
			if !errors.As(err, &usage) || !strings.Contains(err.Error(), "--"+name) {
				t.Errorf("got %v, want a usage error naming --%s", err, name)
			}
		})
	}
	for _, name := range []string{"sink-catalog", "rinex-version", "max-files"} {
		t.Run("inapplicable "+name, func(t *testing.T) {
			err := CheckFlags(SourceIceberg, SinkFiles, append(full, name))
			if err == nil || !strings.Contains(err.Error(), "--"+name+" doesn't apply") {
				t.Errorf("got %v, want --%s rejected", err, name)
			}
		})
	}
	for _, pair := range [][2]string{{"raw", SinkFiles}, {SourceIceberg, "rinex"}} {
		if err := CheckFlags(pair[0], pair[1], full); err == nil {
			t.Errorf("--source %s --sink %s: no error", pair[0], pair[1])
		}
	}
}

func TestParseScope(t *testing.T) {
	for in, want := range map[string]datamodels.Scope{
		"net":                 {Network: "net"},
		"net/STA":             {Network: "net", Station: "STA"},
		"net/STA/2026_A_1126": scope,
	} {
		if got, err := ParseScope(in); err != nil || got != want {
			t.Errorf("%q: %+v, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "net//c", "a/b/c/d", "/STA"} {
		if _, err := ParseScope(in); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
}

func TestNewErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Spec)
	}{
		{"unknown catalog", func(s *Spec) { s.SourceCatalog = "no-such-catalog" }},
		{"no table", func(s *Spec) { s.Tables = nil }},
		{"unknown table", func(s *Spec) { s.Tables = []string{"nope"} }},
		{"ambiguous table", func(s *Spec) { s.Tables = []string{"garpos"} }},
		{"same file name twice", func(s *Spec) { s.Tables = []string{"gnssa_l1.garpos", "gnssa_l2.garpos"} }},
		{"two source types", func(s *Spec) { s.SourceTypes = []string{"AHRS00", "DFOP00"} }},
		{"type without full scope", func(s *Spec) {
			s.Scope = datamodels.Scope{Network: "net"}
			s.SourceTypes = []string{"AHRS00"}
		}},
		{"unknown column", func(s *Spec) { s.Columns = []string{"time", "nope"} }},
		{"start after end", func(s *Spec) { s.Start, s.End = t0.Add(time.Hour), t0 }},
		{"unknown table format", func(s *Spec) { s.TableFormat = "xlsx" }},
		{"no scope", func(s *Spec) { s.Scope = datamodels.Scope{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := spec("w", "out")
			tc.edit(&s)
			_, err := New(s)
			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Errorf("got %v, want a usage error", err)
			}
		})
	}
}

func TestRunSelection(t *testing.T) {
	wh := warehouse(t)
	for _, tc := range []struct {
		name string
		edit func(*Spec)
		want int64
	}{
		{"campaign", func(*Spec) {}, 14},
		{"network", func(s *Spec) { s.Scope = datamodels.Scope{Network: "net"} }, 14},
		{"other network", func(s *Spec) { s.Scope = datamodels.Scope{Network: "x"} }, 0},
		{"file type", func(s *Spec) { s.SourceTypes = []string{"DFOP00"} }, 4},
		{"stream", func(s *Spec) { s.SourceTypes, s.LogSource = []string{"AHRS00"}, "AHRS00" }, 10},
		{"window", func(s *Spec) {
			s.SourceTypes, s.Start, s.End = []string{"AHRS00"}, t0.Add(2*time.Second), t0.Add(5*time.Second)
		}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := spec(wh, t.TempDir())
			tc.edit(&s)
			res := run(t, s)
			if !res.OK || len(res.Outputs) != 1 || res.Outputs[0].Rows != tc.want {
				t.Fatalf("ok %v, outputs %+v; want %d rows", res.OK, res.Outputs, tc.want)
			}
			var streamRows int64
			for _, st := range res.Streams {
				streamRows += st.Rows
			}
			if streamRows != tc.want {
				t.Errorf("streams count %d rows, want %d", streamRows, tc.want)
			}
		})
	}
}

func TestRunFormats(t *testing.T) {
	wh := warehouse(t)
	cols := []string{"time_instrument", "time", "h"}
	for _, format := range []string{FormatCSV, FormatArrow, FormatParquet} {
		t.Run(format, func(t *testing.T) {
			s := spec(wh, t.TempDir())
			s.TableFormat, s.SourceTypes, s.Columns = format, []string{"AHRS00"}, cols
			res := run(t, s)
			path := filepath.Join(s.SinkURI, "attitude_ahrs."+format)
			if len(res.Outputs) != 1 || res.Outputs[0].Path != path {
				t.Fatalf("outputs %+v, want %s", res.Outputs, path)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch format {
			case FormatCSV:
				lines := strings.Split(strings.TrimSpace(string(b)), "\n")
				if len(lines) != 11 || lines[0] != "time_instrument,time,h" {
					t.Fatalf("%d lines, header %q", len(lines), lines[0])
				}
				if want := "0,2026-06-02T19:54:31Z,"; lines[1] != want {
					t.Errorf("row %q, want %q (RFC 3339 time, empty null)", lines[1], want)
				}
			case FormatArrow:
				r, err := ipc.NewFileReader(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				if got := r.Schema().Field(0).Name; r.Schema().NumFields() != 3 || got != "time_instrument" {
					t.Errorf("%d fields, first %s", r.Schema().NumFields(), got)
				}
			case FormatParquet:
				pf, err := file.NewParquetReader(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				if pf.NumRows() != 10 {
					t.Errorf("%d rows, want 10", pf.NumRows())
				}
			}
		})
	}

	// An empty selection still writes a valid file.
	s := spec(wh, t.TempDir())
	s.Scope, s.TableFormat = datamodels.Scope{Network: "x"}, FormatParquet
	run(t, s)
	b, err := os.ReadFile(filepath.Join(s.SinkURI, "attitude_ahrs.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.NewParquetReader(bytes.NewReader(b)); err != nil {
		t.Errorf("empty selection: %v", err)
	}
}

func TestRunPartial(t *testing.T) {
	s := spec(warehouse(t), t.TempDir())
	s.Tables = []string{"attitude_ahrs", "inspva"} // inspva was never written
	res := run(t, s)
	if res.OK || len(res.Outputs) != 1 || len(res.Errors) != 1 || res.Errors[0].Subject != "gnssa_l0.inspva" {
		t.Fatalf("ok %v, outputs %+v, errors %+v", res.OK, res.Outputs, res.Errors)
	}
	if code := ExitCode(res, nil); code != ExitPartial {
		t.Errorf("exit code %d, want %d", code, ExitPartial)
	}
	if _, err := os.Stat(filepath.Join(s.SinkURI, "inspva.csv")); !os.IsNotExist(err) {
		t.Errorf("failed table left a file: %v", err)
	}
	if entries, _ := os.ReadDir(s.SinkURI); len(entries) != 1 {
		t.Errorf("sink holds %d entries, want 1 (no temp files)", len(entries))
	}
}

func TestExitCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  *Result
		err  error
		want int
	}{
		{"ok", &Result{OK: true}, nil, ExitOK},
		{"usage", nil, &UsageError{errors.New("x")}, ExitUsage},
		{"usage result", UsageResult(errors.New("x")), nil, ExitUsage},
		{"fatal", &Result{}, errors.New("x"), ExitFatal},
		{"nothing written", &Result{Errors: []Error{{Stage: "read"}}}, nil, ExitFatal},
		{"partial", &Result{Errors: []Error{{Stage: "read"}}, Outputs: []Output{{}}}, nil, ExitPartial},
	} {
		if got := ExitCode(tc.res, tc.err); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestResultGolden checks the result documents of a successful and a
// usage-error run against testdata. Paths are replaced by placeholders.
func TestResultGolden(t *testing.T) {
	wh, out := warehouse(t), t.TempDir()
	s := spec(wh, out)
	s.SourceTypes, s.Start, s.End = []string{"AHRS00"}, t0, t0.Add(time.Hour)
	ok := run(t, s)
	usage := UsageResult(CheckFlags(SourceIceberg, SinkFiles, []string{"source", "sink", "max-files"}))
	for name, res := range map[string]*Result{"ok": ok, "usage": usage} {
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		b = bytes.ReplaceAll(b, []byte(out), []byte("$SINK"))
		b = bytes.ReplaceAll(b, []byte(wh), []byte("$WAREHOUSE"))
		golden := filepath.Join("testdata", "result_"+name+".json")
		if *update {
			if err := os.WriteFile(golden, append(b, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(b) + "\n"; got != string(want) {
			t.Errorf("%s result:\n%s\nwant:\n%s", name, got, want)
		}
	}
}
