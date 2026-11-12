package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// recorder wraps a catalog and records the calls Build makes. Build must
// use only the generic iceberg-go Catalog interface, so the same calls
// reach Glue or a REST catalog as reach the local one.
type recorder struct {
	icecat.Catalog
	calls []string
}

func (r *recorder) CreateNamespace(ctx context.Context, ns table.Identifier, props iceberg.Properties) error {
	r.calls = append(r.calls, "CreateNamespace "+strings.Join(ns, ".")+" comment="+props["comment"])
	return r.Catalog.CreateNamespace(ctx, ns, props)
}

func (r *recorder) LoadTable(ctx context.Context, id table.Identifier) (*table.Table, error) {
	r.calls = append(r.calls, "LoadTable "+strings.Join(id, "."))
	return r.Catalog.LoadTable(ctx, id)
}

func (r *recorder) CreateTable(ctx context.Context, id table.Identifier, sc *iceberg.Schema, opts ...icecat.CreateTableOpt) (*table.Table, error) {
	r.calls = append(r.calls, "CreateTable "+strings.Join(id, "."))
	return r.Catalog.CreateTable(ctx, id, sc, opts...)
}

// tableNames lists the tables of namespace ns.
func tableNames(t *testing.T, cat *Catalog, ns string) []string {
	t.Helper()
	var names []string
	for id, err := range cat.cat.ListTables(context.Background(), table.Identifier{ns}) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, id[len(id)-1])
	}
	slices.Sort(names)
	return names
}

// buildRecorded builds def in the local warehouse dir through a recorder.
func buildRecorded(t *testing.T, dir string, def datamodels.CatalogDef) (*Catalog, *recorder) {
	t.Helper()
	ctx := context.Background()
	ice, err := openIceberg(ctx, Config{Name: Local, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{Catalog: ice}
	cat, err := connect(ctx, rec, def, buildDatabase)
	if err != nil {
		t.Fatal(err)
	}
	return cat, rec
}

func TestBuild(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat, rec := buildRecorded(t, dir, datamodels.SFG)

	// Each database: one CreateNamespace with its doc, then each table
	// looked up and created.
	var want []string
	for _, db := range datamodels.SFG.Databases {
		want = append(want, "CreateNamespace "+db.Name+" comment="+db.Doc)
		for _, def := range db.Tables {
			want = append(want, "LoadTable "+db.Name+"."+def.Name, "CreateTable "+db.Name+"."+def.Name)
		}
	}
	if !slices.Equal(rec.calls, want) {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(rec.calls, "\n"), strings.Join(want, "\n"))
	}
	for _, db := range datamodels.SFG.Databases {
		var wantTables []string
		for _, def := range db.Tables {
			wantTables = append(wantTables, def.Name)
			// Every declared table is open, found by name without I/O.
			if _, err := mustDatabase(t, cat, db.Name).Table(def.Name); err != nil {
				t.Error(err)
			}
		}
		slices.Sort(wantTables)
		if got := tableNames(t, cat, db.Name); !slices.Equal(got, wantTables) {
			t.Errorf("%s tables = %v, want %v", db.Name, got, wantTables)
		}
		props, err := cat.cat.LoadNamespaceProperties(ctx, table.Identifier{db.Name})
		if err != nil || props["comment"] != db.Doc {
			t.Errorf("%s properties = %v, %v", db.Name, props, err)
		}
	}

	// Rows survive a second Build on a fresh connection, which creates nothing.
	rows := attitudeRows(t, "net.STA.C.AHRS00.AHRS00", t0, 5)
	if _, err := appendRows(ctx, cat, datamodels.AttitudeAHRS, rows); err != nil {
		t.Fatal(err)
	}
	rows.Release()
	cat.Close()
	cat, rec = buildRecorded(t, dir, datamodels.SFG)
	defer cat.Close()
	for _, c := range rec.calls {
		if !strings.HasPrefix(c, "LoadTable") && !strings.HasPrefix(c, "CreateNamespace") {
			t.Errorf("second Build: %s", c)
		}
	}
	if all, _ := count(t, cat, datamodels.AttitudeAHRS, ""); all != 5 {
		t.Errorf("%d rows after rebuilding, want 5", all)
	}
}

// mustDatabase is cat's database named name.
func mustDatabase(t *testing.T, cat *Catalog, name string) *Database {
	t.Helper()
	db, err := cat.Database(name)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLookups(t *testing.T) {
	ctx := context.Background()
	cat, err := Build(ctx, Config{Name: Local, Warehouse: t.TempDir()}, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	l0 := mustDatabase(t, cat, "gnssa_l0")
	if mustDatabase(t, cat, "gnssa_l0") != l0 || l0.Name() != "gnssa_l0" {
		t.Error("Database: not one handle per name")
	}
	if _, err := cat.Database("gnssa_l9"); err == nil {
		t.Error("Database(gnssa_l9): no error")
	}
	if _, err := l0.Table("garpos"); err == nil {
		t.Error("Table: found gnssa_l1's garpos in gnssa_l0")
	}
	a, _ := l0.Table("inspva")
	if b, _ := cat.TableOf(datamodels.INSPVA); a == nil || a != b {
		t.Error("Table and TableOf: not one handle per table")
	}
	l1, _ := cat.TableOf(datamodels.GarposL1)
	l2, _ := cat.TableOf(datamodels.GarposL2)
	if l1 == nil || l1 == l2 || l1.Def().Level != 1 {
		t.Error("TableOf: garpos not found by level")
	}
}

// Load opens what is there and creates nothing, not even a namespace.
func TestLoad(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := Config{Name: Local, Warehouse: dir}
	// An initialised warehouse holding nothing.
	empty, err := Build(ctx, cfg, datamodels.CatalogDef{Name: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	empty.Close()
	cat, err := Load(ctx, cfg, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.TableOf(datamodels.INSPVA); !errors.Is(err, ErrNotInCatalog) {
		t.Errorf("TableOf on an empty warehouse = %v, want ErrNotInCatalog", err)
	}
	if exists, err := cat.cat.CheckNamespaceExists(ctx, table.Identifier{"gnssa_l0"}); err != nil || exists {
		t.Errorf("Load created the namespace (%v, %v)", exists, err)
	}
	cat.Close()

	built, err := Build(ctx, cfg, datamodels.SFG.Only(datamodels.INSPVA))
	if err != nil {
		t.Fatal(err)
	}
	built.Close()
	cat, err = Load(ctx, cfg, datamodels.SFG)
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if _, err := cat.TableOf(datamodels.INSPVA); err != nil {
		t.Errorf("TableOf(inspva) after building it: %v", err)
	}
	if _, err := cat.TableOf(datamodels.AttitudeAHRS); !errors.Is(err, ErrNotInCatalog) {
		t.Errorf("TableOf(attitude) = %v, want ErrNotInCatalog", err)
	}
}

// racer hides existing tables from the first LoadTable of each, as if
// another worker created them between Build's lookup and its create.
type racer struct {
	icecat.Catalog
	seen map[string]bool
}

func (r *racer) LoadTable(ctx context.Context, id table.Identifier) (*table.Table, error) {
	key := strings.Join(id, ".")
	if !r.seen[key] {
		r.seen[key] = true
		return nil, icecat.ErrNoSuchTable
	}
	return r.Catalog.LoadTable(ctx, id)
}

// A Build racing another that already created its tables uses theirs.
func TestBuildConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	def := datamodels.SFG.Only(datamodels.INSPVA, datamodels.AttitudeAHRS)
	first, err := Build(ctx, Config{Name: Local, Warehouse: dir}, def)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	ice, err := openIceberg(ctx, Config{Name: Local, Warehouse: dir})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := connect(ctx, &racer{Catalog: ice, seen: map[string]bool{}}, def, buildDatabase)
	if err != nil {
		t.Fatalf("Build losing a create race: %v", err)
	}
	defer cat.Close()
	if _, err := cat.TableOf(datamodels.INSPVA); err != nil {
		t.Error(err)
	}
}

func TestBuildRejectsDuplicates(t *testing.T) {
	ctx := context.Background()
	cfg := Config{Name: Local, Warehouse: t.TempDir()}
	twice := datamodels.CatalogDef{Name: "t", Databases: []datamodels.DatabaseDef{
		{Name: "d", Tables: []datamodels.TableDef{datamodels.INSPVA, datamodels.INSPVA}}}}
	if cat, err := Build(ctx, cfg, twice); err == nil {
		cat.Close()
		t.Error("Build: no error for a table declared twice")
	}
	dbTwice := datamodels.CatalogDef{Name: "t", Databases: []datamodels.DatabaseDef{datamodels.GNSSA_L0, datamodels.GNSSA_L0}}
	if cat, err := Build(ctx, cfg, dbTwice); err == nil {
		cat.Close()
		t.Error("Build: no error for a database declared twice")
	}
}

// Load reads a missing local warehouse as an error, without creating it.
func TestLoadMissingWarehouse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "typo")
	if cat, err := Load(context.Background(), Config{Name: Local, Warehouse: dir}, datamodels.SFG); err == nil {
		cat.Close()
		t.Fatal("Load: no error for a missing warehouse")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Load created %s (%v)", dir, err)
	}
}
