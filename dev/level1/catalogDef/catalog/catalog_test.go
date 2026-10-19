package catalog

import (
	"context"
	"testing"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
)

func TestCatalogTable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, def := range schemas.Level1Tables {
		tbl, err := cat.Table(ctx, def)
		if err != nil {
			t.Fatalf("%s: %v", def.Name, err)
		}
		spec, _ := def.PartitionSpec()
		if !tbl.Spec().Equals(spec) {
			t.Errorf("%s: spec %v, want %v", def.Name, tbl.Spec(), spec)
		}
	}
	// Reopening loads the existing tables instead of failing on create.
	cat2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cat2.Close()
	if _, err := cat2.Table(ctx, schemas.AttitudeAHRS); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}
