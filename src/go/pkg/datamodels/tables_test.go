package datamodels

import (
	"fmt"
	"github.com/apache/arrow-go/v18/arrow"
	"strings"
	"testing"

	"github.com/apache/iceberg-go"
)

func TestAttitudeAHRS(t *testing.T) {
	def := AttitudeAHRS
	if got := QualifiedName(def); got != "gnssa_l0.attitude_ahrs" {
		t.Errorf("QualifiedName = %v", got)
	}

	sc, err := def.IcebergSchema()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(sc.Fields()); n != 12 {
		t.Fatalf("schema has %d fields, want 12", n)
	}
	// Field IDs follow declaration order; stream_edid is first and required.
	for i, name := range []string{"stream_edid", "time", "time_instrument", "time_start_count"} {
		f, ok := sc.FindFieldByName(name)
		if !ok || f.ID != i+1 {
			t.Errorf("%s: id %d ok %v, want id %d", name, f.ID, ok, i+1)
		}
	}
	if f, _ := sc.FindFieldByName("stream_edid"); !f.Required {
		t.Error("stream_edid should be required")
	}
	if f, _ := sc.FindFieldByName("time_start_count"); f.Required {
		t.Error("time_start_count should be nullable")
	}

	spec, err := def.PartitionSpec()
	if err != nil {
		t.Fatal(err)
	}
	if got := partitionFields(t, spec, sc); got != "stream_edid=identity(stream_edid)" {
		t.Errorf("partition spec = %s", got)
	}

	if so, err := def.SortOrder(); err != nil || !so.IsUnsorted() {
		t.Errorf("SortOrder = %v, %v; want unsorted", so, err)
	}

	as, err := def.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	if as.NumFields() != 12 || as.Field(1).Nullable {
		t.Errorf("arrow schema: %d fields, time nullable %v", as.NumFields(), as.Field(1).Nullable)
	}
}

func TestLevel0Tables(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range Level0Tables {
		t.Run(def.Name, func(t *testing.T) {
			if seen[def.Name] {
				t.Fatalf("duplicate table name %q", def.Name)
			}
			seen[def.Name] = true
			if db, ok := DatabaseOf(def); !ok || db.Name != "gnssa_l0" {
				t.Errorf("database %q, %v", db.Name, ok)
			}
			if def.External == nil {
				ownSchema(t, def)
			}
			noScopeColumns(t, def)
			as, err := def.ArrowSchema()
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{def.TimeColumn, StreamEDIDName} {
				idx := as.FieldIndices(name)
				if len(idx) != 1 {
					t.Errorf("no %q column (TimeColumn / stream_edid)", name)
				}
			}
			if idx := as.FieldIndices(def.TimeColumn); len(idx) == 1 && as.Field(idx[0]).Type.ID() != arrow.TIMESTAMP {
				t.Errorf("TimeColumn %q is %s, want a timestamp", def.TimeColumn, as.Field(idx[0]).Type)
			}
			spec, err := def.PartitionSpec()
			if err != nil {
				t.Fatal(err)
			}
			sc, err := def.IcebergSchema()
			if err != nil {
				t.Fatal(err)
			}
			if got := partitionFields(t, spec, sc); def.External == nil && got != "stream_edid=identity(stream_edid)" {
				t.Errorf("partition spec = %s", got)
			}
			if _, err := def.SortOrder(); err != nil {
				t.Error(err)
			}
			if as.NumFields() != len(sc.Fields()) {
				t.Errorf("arrow schema has %d fields, want %d", as.NumFields(), len(sc.Fields()))
			}
		})
	}
}

// GNSSObs is defined by gnsstools, with its own partitioning; these are the
// parts the parsers and queries rely on.
func TestGNSSObsFromGnsstools(t *testing.T) {
	if GNSSObs.Schema != nil || GNSSObs.Checks != nil || GNSSObs.SortBy != nil {
		t.Fatal("GNSSObs must not define its own schema, checks or sort order")
	}
	sc, err := GNSSObs.IcebergSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stream_edid", "time_utc", "time_gpst", "time_sub_us_ps", "constellation", "satellite_id", "signal"} {
		if f, ok := sc.FindFieldByName(name); !ok || !f.Required {
			t.Errorf("%s: missing or nullable", name)
		}
	}

	// PartitionBy overrides gnsstools' spec, resolving columns against the
	// gnsstools schema's field IDs.
	spec, err := GNSSObs.PartitionSpec()
	if err != nil {
		t.Fatal(err)
	}
	if got := partitionFields(t, spec, sc); got != "time_gpst_day=day(time_gpst),stream_edid=identity(stream_edid)" {
		t.Errorf("partition spec = %s", got)
	}
	// Without the override it is gnsstools' own spec.
	gnsstools := GNSSObs
	gnsstools.PartitionBy = nil
	spec, err = gnsstools.PartitionSpec()
	if err != nil {
		t.Fatal(err)
	}
	if got := partitionFields(t, spec, sc); got != "time_gpst_day=day(time_gpst),stream_edid_bucket=bucket[64](stream_edid)" {
		t.Errorf("gnsstools partition spec = %s", got)
	}
	if so, err := GNSSObs.SortOrder(); err != nil || so.Len() == 0 {
		t.Errorf("SortOrder = %v, %v; want gnsstools' sort", so, err)
	}
	props, err := GNSSObs.Properties()
	if err != nil {
		t.Fatal(err)
	}
	if props["gnss.schema-version"] == "" || props["comment"] != GNSSObs.Doc {
		t.Errorf("properties = %v", props)
	}

	// The definition is built once per process.
	again, _ := GNSSObs.IcebergSchema()
	if again != sc {
		t.Error("Schema rebuilt the gnsstools definition")
	}
}

// ownSchema checks a table defined here (not External): field IDs 1..n in
// declaration order, unique names, a required stream_edid first, and checks
// that name schema columns of a type they can check.
func ownSchema(t *testing.T, def TableDef) {
	t.Helper()
	if def.Schema == nil {
		t.Fatal("no Schema")
	}
	fields := def.Schema.Fields()
	names := map[string]bool{}
	for i, f := range fields {
		if f.ID != i+1 {
			t.Errorf("%s: field ID %d, want %d (declaration order)", f.Name, f.ID, i+1)
		}
		if names[f.Name] {
			t.Errorf("duplicate column %q", f.Name)
		}
		names[f.Name] = true
	}
	if c := fields[0]; c.Name != StreamEDIDName || !c.Required {
		t.Errorf("first column %q (required %v), want a required stream_edid", c.Name, c.Required)
	}
	for name, c := range def.Checks {
		f, ok := def.Schema.FindFieldByName(name)
		if !ok {
			t.Errorf("check on %q, which is not a column", name)
			continue
		}
		bounded := c.Ge != nil || c.Le != nil || c.Gt != nil || c.Lt != nil
		switch {
		case bounded && f.Type != float64Type && f.Type != float32Type && f.Type != int32Type:
			t.Errorf("%s: bounds on a %s column", name, f.Type)
		case c.IsIn != nil && f.Type != str:
			t.Errorf("%s: IsIn on a %s column", name, f.Type)
		}
	}
}

// noScopeColumns checks that def's only identity column is stream_edid.
func noScopeColumns(t *testing.T, def TableDef) {
	t.Helper()
	sc, err := def.IcebergSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Network", "Station", "Campaign", "Log_Source"} {
		if _, ok := sc.FindFieldByName(name); ok {
			t.Errorf("%s has a %s column", def.Name, name)
		}
	}
}

// partitionFields describes spec's fields as name=transform(source column).
func partitionFields(t *testing.T, spec iceberg.PartitionSpec, sc *iceberg.Schema) string {
	t.Helper()
	var parts []string
	for _, f := range spec.Fields() {
		src, ok := sc.FindColumnName(f.SourceID())
		if !ok {
			t.Fatalf("partition field %s: no source column %d", f.Name, f.SourceID())
		}
		parts = append(parts, fmt.Sprintf("%s=%s(%s)", f.Name, f.Transform, src))
	}
	return strings.Join(parts, ",")
}

func TestDerivedTables(t *testing.T) {
	for _, set := range []struct {
		ns   string
		defs []TableDef
	}{{"gnssa_l1", Level1Tables}, {"gnssa_l2", Level2Tables}} {
		for _, def := range set.defs {
			t.Run(set.ns+"."+def.Name, func(t *testing.T) {
				if db, ok := DatabaseOf(def); !ok || db.Name != set.ns {
					t.Errorf("database %q, %v", db.Name, ok)
				}
				noScopeColumns(t, def)
				ownSchema(t, def)
				if c, ok := def.Schema.FindFieldByName(def.TimeColumn); !ok || c.Type != timestamptz || !c.Required {
					t.Errorf("TimeColumn %q missing, nullable or not a timestamp", def.TimeColumn)
				}
				spec, err := def.PartitionSpec()
				if err != nil {
					t.Fatal(err)
				}
				sc, err := def.IcebergSchema()
				if err != nil {
					t.Fatal(err)
				}
				if got := partitionFields(t, spec, sc); got != "stream_edid=identity(stream_edid)" {
					t.Errorf("partition spec = %s", got)
				}
			})
		}
	}
}

func TestDerivedEDID(t *testing.T) {
	scope := Scope{Network: "net", Station: "STA", Campaign: "2025_A_1126"}
	got := DerivedEDID(scope, ProductPRIDE, "NOV000:RANGEA")
	if want := "net.STA.2025_A_1126.PRIDE.NOV000.RANGEA"; got != want {
		t.Errorf("DerivedEDID = %q, want %q", got, want)
	}
}

func TestTableByName(t *testing.T) {
	for name, want := range map[string]string{"inspva": "gnssa_l0.inspva", "gnssa_l2.garpos": "gnssa_l2.garpos",
		"gnss_antenna_positions_ppp": "gnssa_l1.gnss_antenna_positions_ppp"} {
		def, err := TableByName(name)
		if err != nil || QualifiedName(def) != want {
			t.Errorf("TableByName(%q) = %s, %v; want %s", name, QualifiedName(def), err, want)
		}
	}
	for _, name := range []string{"garpos", "nope", "gnssa_l0.garpos"} {
		if _, err := TableByName(name); err == nil {
			t.Errorf("TableByName(%q): no error", name)
		}
	}
}
