package schemas

import (
	"testing"

	"github.com/apache/iceberg-go"
)

func TestAttitudeAHRS(t *testing.T) {
	def := AttitudeAHRS
	if got := def.Identifier(); got[0] != "gnssa_l1" || got[1] != "attitude_ahrs" {
		t.Errorf("Identifier = %v", got)
	}

	sc := def.Schema()
	if n := len(sc.Fields()); n != 15 {
		t.Fatalf("schema has %d fields, want 15", n)
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
	if as.NumFields() != 15 || as.Field(4).Nullable {
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
			for _, s := range Level1Scope {
				if c, ok := def.Column(s.Name); !ok || !c.Required {
					t.Errorf("scope column %s missing or nullable", s.Name)
				}
			}
			if _, err := def.PartitionSpec(); err != nil {
				t.Error(err)
			}
			if _, err := def.SortOrder(); err != nil {
				t.Error(err)
			}
			as, err := def.ArrowSchema()
			if err != nil {
				t.Fatal(err)
			}
			if as.NumFields() != len(def.Columns) {
				t.Errorf("arrow schema has %d fields, want %d", as.NumFields(), len(def.Columns))
			}
		})
	}
}

// The leading columns must match gnsstools' observations table so its readers
// work on gnss_observations.
func TestGNSSObsMatchesGnsstools(t *testing.T) {
	want := []struct {
		name     string
		typ      iceberg.Type
		required bool
	}{
		{"stream_edid", iceberg.PrimitiveTypes.String, true},
		{"time_utc", iceberg.PrimitiveTypes.TimestampTz, true},
		{"time_gpst", iceberg.PrimitiveTypes.TimestampTz, true},
		{"time_sub_us_ps", iceberg.PrimitiveTypes.Int32, true},
		{"clock_offset", iceberg.PrimitiveTypes.Float64, false},
		{"constellation", iceberg.PrimitiveTypes.String, true},
		{"satellite_id", iceberg.PrimitiveTypes.Int32, true},
		{"signal", iceberg.PrimitiveTypes.String, true},
		{"code", iceberg.PrimitiveTypes.Float64, false},
		{"phase", iceberg.PrimitiveTypes.Float64, false},
		{"doppler", iceberg.PrimitiveTypes.Float64, false},
		{"snr", iceberg.PrimitiveTypes.Float64, false},
		{"loss_of_lock", iceberg.PrimitiveTypes.Bool, false},
		{"half_cycle_ambiguity", iceberg.PrimitiveTypes.Bool, false},
		{"boc_tracking", iceberg.PrimitiveTypes.Bool, false},
		{"lock_time", iceberg.PrimitiveTypes.Float64, false},
		{"epoch_flag", iceberg.PrimitiveTypes.Int32, false},
		{"fcn", iceberg.PrimitiveTypes.Int32, false},
		{"write_time", iceberg.PrimitiveTypes.TimestampTz, false},
		{"arrival_time", iceberg.PrimitiveTypes.TimestampTz, false},
	}
	cols := GNSSObs.Columns
	for i, w := range want {
		c := cols[i]
		if c.Name != w.name || !c.Type.Equals(w.typ) || c.Required != w.required {
			t.Errorf("column %d = %s %v required=%v, want %s %v required=%v",
				i, c.Name, c.Type, c.Required, w.name, w.typ, w.required)
		}
	}

	spec, err := GNSSObs.PartitionSpec()
	if err != nil {
		t.Fatal(err)
	}
	if spec.NumFields() != 4 {
		t.Fatalf("spec has %d fields, want 4", spec.NumFields())
	}
	if f := spec.Field(3); f.Name != "time_gpst_day" || !f.Transform.Equals(iceberg.DayTransform{}) {
		t.Errorf("partition field 3 = %+v", f)
	}
	if so, err := GNSSObs.SortOrder(); err != nil || so.Len() != 5 {
		t.Errorf("SortOrder = %v, %v; want 5 fields", so, err)
	}
}
