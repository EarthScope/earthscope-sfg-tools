package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// TestConvertExitCodes runs the built binary: exit codes are part of the
// interface pipelines rely on.
func TestConvertExitCodes(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sfg")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// A warehouse holding an empty attitude_ahrs table, inspva rows from a
	// DFOP00 fixture, and no gnss_observations table.
	wh := filepath.Join(dir, "warehouse")
	ctx := context.Background()
	cat, err := catalog.Build(ctx, catalog.LocalConfig(wh), datamodels.SFG.Only(datamodels.L0AttitudeAHRS))
	if err != nil {
		t.Fatal(err)
	}
	cat.Close()
	dfop00, err := filepath.Abs("../../../../tests/data/test_gcc1_20250822_DFOP00.raw")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "iceberg", "ingest", "--catalog", "local", "--warehouse", wh,
		"--scope", "net/STA/2026_A_1126", "--table", "inspva", dfop00).CombinedOutput(); err != nil {
		t.Fatalf("seeding inspva: %v\n%s", err, out)
	}

	base := []string{"convert", "--source", "iceberg", "--source-catalog", "local", "--source-uri", wh,
		"--scope", "net/STA/2026_A_1126", "--sink", "files", "--table-format", "csv"}
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"ok", []string{"--table", "inspva"}, convert.ExitOK},
		{"partial", []string{"--table", "inspva,gnss_observations"}, convert.ExitPartial},
		{"nothing selected", []string{"--table", "attitude_ahrs"}, convert.ExitFatal},
		{"nothing selected, one table missing", []string{"--table", "attitude_ahrs,gnss_observations"}, convert.ExitFatal},
		{"fatal", []string{"--table", "gnss_observations"}, convert.ExitFatal},
		{"missing flag", nil, convert.ExitUsage},
		{"unknown flag", []string{"--table", "attitude_ahrs", "--max-files", "1"}, convert.ExitUsage},
		{"bad value", []string{"--table", "garpos"}, convert.ExitUsage},
		{"second warehouse", []string{"--table", "attitude_ahrs", wh}, convert.ExitUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := filepath.Join(t.TempDir(), "result.json")
			args := append(append(base, "--sink-uri", t.TempDir(), "--result", result), tc.args...)
			err := exec.Command(bin, args...).Run()
			code := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if code != tc.want {
				t.Errorf("exit code %d, want %d", code, tc.want)
			}
			// Every run that got past flag parsing writes its result.
			if tc.name == "unknown flag" {
				return // --max-files is unknown to cobra: rejected while parsing
			}
			b, err := os.ReadFile(result)
			if err != nil {
				t.Fatal(err)
			}
			var res convert.Result
			if err := json.Unmarshal(b, &res); err != nil {
				t.Fatal(err)
			}
			if res.OK != (tc.want == convert.ExitOK) {
				t.Errorf("result ok %v with exit code %d", res.OK, code)
			}
		})
	}
}

// Arguments after the flags are more source URIs, as a shell glob after
// --source-uri expands to.
func TestConvertSourceArgs(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sfg")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	var files []string
	for _, name := range []string{"a_DFOP00.raw", "b_DFOP00.raw", "c_DFOP00.raw"} {
		files = append(files, filepath.Join(dir, name))
		if err := os.WriteFile(files[len(files)-1], []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result := filepath.Join(dir, "result.json")
	args := append([]string{"convert", "--source", "raw", "--source-type", "DFOP00", "--scope", "net/STA/2026_A_1126",
		"--sink", "files", "--sink-uri", t.TempDir(), "--table-format", "csv", "--result", result,
		"--source-uri"}, files[:2]...)
	exec.Command(bin, append(args, "--source-uri", files[2])...).Run() // the exit code depends on parsing "{}"
	b, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var res convert.Result
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Inputs) != 3 || len(res.Source.URIs) != 3 {
		t.Errorf("inputs %+v, source %+v; want the three files", res.Inputs, res.Source)
	}
}
