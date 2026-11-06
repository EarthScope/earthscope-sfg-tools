package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

func TestCatalogTable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat, err := Open(ctx, Config{Name: Local, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, def := range datamodels.Level0Tables {
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
	cat2, err := Open(ctx, Config{Name: Local, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer cat2.Close()
	if _, err := cat2.Table(ctx, datamodels.AttitudeAHRS); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

// Opening a Glue catalog only loads AWS config (no network calls), so a
// misregistered catalog type fails here rather than at the first ingest.
func TestOpenGlue(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-2")
	cat, err := Open(context.Background(), Config{Warehouse: "s3://bucket/prefix", Props: iceberg.Properties{"type": "glue"}})
	if err != nil {
		t.Fatal(err)
	}
	cat.Close()
}

// The REST catalog is registered too; it is the cloud-neutral one.
func TestOpenUnreachableREST(t *testing.T) {
	_, err := Open(context.Background(), Config{Props: iceberg.Properties{"type": "rest", "uri": "http://127.0.0.1:1"}})
	if err == nil || errors.Is(err, icecat.ErrCatalogNotFound) {
		t.Fatalf("Open = %v, want a connection error from a registered rest catalog", err)
	}
}

func TestCheck(t *testing.T) {
	for _, name := range []string{Local, ""} {
		if err := Check(name); err != nil {
			t.Errorf("Check(%q) = %v", name, err)
		}
	}
	if err := Check("no-such-catalog"); err == nil {
		t.Error("Check(no-such-catalog): no error")
	}
	if _, err := Open(context.Background(), Config{Name: "no-such-catalog"}); err == nil {
		t.Error("Open(no-such-catalog): no error")
	}
	if _, err := Open(context.Background(), Config{Name: Local}); err == nil {
		t.Error("Open(local) without a warehouse: no error")
	}
}
