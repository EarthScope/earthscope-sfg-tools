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
	if got := def.Identifier(); got[0] != "gnssa_l1" || got[1] != "attitude_ahrs" {
		t.Errorf("Identifier = %v", got)
	}

	sc, err := def.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(sc.Fields()); n != 16 {
		t.Fatalf("schema has %d fields, want 16", n)
	}
	// stream_edid was added last, so it is the last field ID and nullable.
	if f, ok := sc.FindFieldByName("stream_edid"); !ok || f.ID != 16 || f.Required {
		t.Errorf("stream_edid: id %d required %v ok %v, want id 16, nullable", f.ID, f.Required, ok)
	}
	// Field IDs must follow declaration order (and match the Python TableDef).
	for i, name := range []string{"Network", "Station", "Campaign", "Log_Source", "time", "time_instrument", "time_start_count"} {
		f, ok := sc.FindFieldByName(name)
		if !ok || f.ID != i+1 {
			t.Errorf("%s: id %d ok %v, want id %d", name, f.ID, ok, i+1)
		}
	}
	if f, _ := sc.FindFieldByName("time_start_count"); f.Required {
		t.Error("time_start_count should be nullable")
	}

	spec, err := def.PartitionSpec()
	if err != nil {
		t.Fatal(err)
	}
	if spec.NumFields() != 3 {
		t.Fatalf("spec has %d fields, want 3", spec.NumFields())
	}
	for i, name := range []string{"Network", "Station", "Campaign"} {
		f := spec.Field(i)
		if f.Name != name || f.SourceIDs[0] != i+1 || !f.Transform.Equals(iceberg.IdentityTransform{}) {
			t.Errorf("partition field %d = %+v", i, f)
		}
	}

	if so, err := def.SortOrder(); err != nil || !so.IsUnsorted() {
		t.Errorf("SortOrder = %v, %v; want unsorted", so, err)
	}

	as, err := def.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	if as.NumFields() != 16 || as.Field(4).Nullable {
		t.Errorf("arrow schema: %d fields, time nullable %v", as.NumFields(), as.Field(4).Nullable)
	}
}

func TestLevel1Tables(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range Level1Tables {
		t.Run(def.Name, func(t *testing.T) {
			if seen[def.Name] {
				t.Fatalf("duplicate table name %q", def.Name)
			}
			seen[def.Name] = true
			if def.Namespace() != "gnssa_l1" {
				t.Errorf("namespace %q", def.Namespace())
			}
			names := map[string]bool{}
			for _, c := range def.Columns {
				if names[c.Name] {
					t.Errorf("duplicate column %q", c.Name)
				}
				names[c.Name] = true
			}
			if def.External == nil {
				for _, s := range Level1Scope {
					if c, ok := def.Column(s.Name); !ok || !c.Required {
						t.Errorf("scope column %s missing or nullable", s.Name)
					}
				}
			}
			as, err := def.ArrowSchema()
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{def.TimeColumn, StreamEDIDColumn.Name} {
				idx := as.FieldIndices(name)
				if len(idx) != 1 {
					t.Errorf("no %q column (TimeColumn / stream_edid)", name)
				}
			}
			if idx := as.FieldIndices(def.TimeColumn); len(idx) == 1 && as.Field(idx[0]).Type.ID() != arrow.TIMESTAMP {
				t.Errorf("TimeColumn %q is %s, want a timestamp", def.TimeColumn, as.Field(idx[0]).Type)
			}
			if _, err := def.PartitionSpec(); err != nil {
				t.Error(err)
			}
			if _, err := def.SortOrder(); err != nil {
				t.Error(err)
			}
			sc, err := def.Schema()
			if err != nil {
				t.Fatal(err)
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
	if GNSSObs.Columns != nil || GNSSObs.SortBy != nil {
		t.Fatal("GNSSObs must not define its own columns or sort order")
	}
	sc, err := GNSSObs.Schema()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stream_edid", "time_utc", "time_gpst", "time_sub_us_ps", "constellation", "satellite_id", "signal"} {
		if f, ok := sc.FindFieldByName(name); !ok || !f.Required {
			t.Errorf("%s: missing or nullable", name)
		}
	}
	for _, name := range []string{"Network", "Station", "Campaign", "Log_Source"} {
		if _, ok := sc.FindFieldByName(name); ok {
			t.Errorf("%s: gnsstools table has no scope columns", name)
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
	again, _ := GNSSObs.Schema()
	if again != sc {
		t.Error("Schema rebuilt the gnsstools definition")
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
