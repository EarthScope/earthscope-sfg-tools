package catalog

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/apache/iceberg-go"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// A partition value that needs escaping (e.g. a space) must land at the
// data file path the manifest records, so readers that take paths literally
// (pyiceberg, Java) find the file. Replacing rows reads and deletes through
// the same paths.
func TestEscapedPartitionPathsExist(t *testing.T) {
	ctx := context.Background()
	def := datamodels.AttitudeAHRS
	def.Name = "attitude_by_stream"
	def.PartitionBy = []datamodels.PartitionKey{{Source: "stream_edid", Transform: iceberg.IdentityTransform{}}}
	// Real EDIDs escape nothing; a space (escaped "+") keeps the case covered.
	const edid = "net work.STA.2026_A_1126.AHRS00.AHRS00"
	test := datamodels.CatalogDef{Name: "test", Databases: []datamodels.DatabaseDef{{Name: "test_l0", Tables: []datamodels.TableDef{def}}}}
	cat, err := Build(ctx, Config{Name: Local, Warehouse: t.TempDir()}, test)
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tbl, err := cat.TableOf(def)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range []int{10, 4} { // the second append replaces the first's first 4 rows
		rows := attitudeRows(t, edid, t0, n)
		if _, err := tbl.Append(ctx, rows); err != nil {
			t.Fatal(err)
		}
		rows.Release()
	}
	if all, _ := countTable(t, tbl, edid); all != 10 {
		t.Errorf("%d rows, want 10", all)
	}

	tasks, err := tbl.Iceberg().Scan().PlanFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("no data files")
	}
	for _, task := range tasks {
		p := task.File.FilePath()
		if !strings.Contains(p, "stream_edid=net+work.STA.") {
			t.Errorf("data file %s: partition value not escaped", p)
		}
		if _, err := os.Stat(strings.TrimPrefix(p, "file://")); err != nil {
			t.Errorf("data file not at its recorded path: %v", err)
		}
	}
}
