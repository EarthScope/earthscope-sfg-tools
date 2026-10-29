package catalog

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/apache/iceberg-go"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// A partition value that needs escaping (stream_edid's ':') must land at the
// data file path the manifest records, so readers that take paths literally
// (pyiceberg, Java) find the file. Replacing rows reads and deletes through
// the same paths.
func TestEscapedPartitionPathsExist(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(ctx, Config{Type: TypeSQL, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	def := datamodels.AttitudeAHRS
	def.Name = "attitude_by_stream"
	def.PartitionBy = []datamodels.PartitionKey{{Source: "stream_edid", Transform: iceberg.IdentityTransform{}}}
	const edid = "net:STA:2026_A_1126:AHRS00:AHRS00"

	for _, n := range []int{10, 4} { // the second append replaces the first's first 4 rows
		rows := attitudeRows(t, edid, t0, n)
		if _, err := cat.Append(ctx, def, rows); err != nil {
			t.Fatal(err)
		}
		rows.Release()
	}
	if all, _ := count(t, cat, def, edid); all != 10 {
		t.Errorf("%d rows, want 10", all)
	}

	tbl, err := cat.Table(ctx, def)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := tbl.Scan().PlanFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("no data files")
	}
	for _, task := range tasks {
		p := task.File.FilePath()
		if !strings.Contains(p, "stream_edid=net%3ASTA%3A") {
			t.Errorf("data file %s: partition value not escaped", p)
		}
		if _, err := os.Stat(strings.TrimPrefix(p, "file://")); err != nil {
			t.Errorf("data file not at its recorded path: %v", err)
		}
	}
}
