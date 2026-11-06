package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
)

// TestIcebergIngestExport ingests a DFOP00 file and a stray file by name
// alone, then exports the inspva rows it wrote.
func TestIcebergIngestExport(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sfg")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	dfop00, err := filepath.Abs("../../../../tests/data/test_gcc1_20250822_DFOP00.raw")
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notes, []byte("not raw data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wh := filepath.Join(dir, "warehouse")
	common := []string{"--catalog", "local", "--warehouse", wh, "--scope", "net/STA/2026_A_1126"}

	in := runResult(t, bin, append(append([]string{"iceberg", "ingest"}, common...), dfop00, notes), convert.ExitOK)
	types := map[string]string{}
	for _, i := range in.Inputs {
		types[filepath.Base(i.Path)] = i.Type + " " + i.Status
	}
	if types["test_gcc1_20250822_DFOP00.raw"] != "DFOP00 ok" || types["notes.txt"] != " skipped" {
		t.Errorf("inputs %v; want the DFOP00 file parsed and notes.txt skipped", types)
	}
	var ingested int64
	for _, o := range in.Outputs {
		if o.Table == "gnssa_l0.inspva" {
			ingested = o.Rows
		}
	}
	if ingested == 0 {
		t.Fatalf("no inspva rows ingested: %+v", in.Outputs)
	}

	out := filepath.Join(dir, "out")
	ex := runResult(t, bin, append(append([]string{"iceberg", "export"}, common...),
		"--table", "inspva", "--format", "csv", "--out", out), convert.ExitOK)
	if len(ex.Outputs) != 1 || ex.Outputs[0].Rows != ingested {
		t.Errorf("exported %+v, want %d inspva rows", ex.Outputs, ingested)
	}
	b, err := os.ReadFile(filepath.Join(out, "inspva.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "net.STA.2026_A_1126.DFOP00.INSPVAA") {
		t.Error("exported rows lack the DFOP00 INSPVAA stream_edid")
	}

	// A local catalog without --warehouse, or a missing required flag, is a
	// usage error with a result.
	runResult(t, bin, []string{"iceberg", "ingest", "--catalog", "local", "--scope", "net/STA/2026_A_1126", dfop00}, convert.ExitUsage)
	if res := runResult(t, bin, append([]string{"iceberg", "export"}, common...), convert.ExitUsage); len(res.Errors) != 1 ||
		!strings.Contains(res.Errors[0].Message, "--table, --out") {
		t.Errorf("errors %+v, want --table and --out named", res.Errors)
	}
}

// runResult runs bin with args and --result, checks its exit code and
// returns the result it wrote.
func runResult(t *testing.T, bin string, args []string, want int) convert.Result {
	t.Helper()
	path := filepath.Join(t.TempDir(), "result.json")
	cmd := exec.Command(bin, append(args, "--result", path)...)
	out, _ := cmd.CombinedOutput()
	if code := cmd.ProcessState.ExitCode(); code != want {
		t.Fatalf("sfg %s: exit code %d, want %d\n%s", strings.Join(args[:2], " "), code, want, out)
	}
	var res convert.Result
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	return res
}
