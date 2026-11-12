package datamodels

import "testing"

func TestSFGCatalog(t *testing.T) {
	dbs := map[string]bool{}
	for i, db := range SFG.Databases {
		if dbs[db.Name] {
			t.Errorf("duplicate database %q", db.Name)
		}
		dbs[db.Name] = true
		tables := map[string]bool{}
		for _, def := range db.Tables {
			if tables[def.Name] {
				t.Errorf("%s: duplicate table %q", db.Name, def.Name)
			}
			tables[def.Name] = true
			// DatabaseOf matches by name and level, so every table of a
			// database must have its level.
			if def.Level != i {
				t.Errorf("%s.%s: level %d, want %d", db.Name, def.Name, def.Level, i)
			}
			if got, ok := DatabaseOf(def); !ok || got.Name != db.Name {
				t.Errorf("DatabaseOf(%s.%s) = %q, %v", db.Name, def.Name, got.Name, ok)
			}
		}
	}
	if got := len(AllTables()); got != 19 {
		t.Errorf("AllTables has %d tables, want 19", got)
	}
	// garpos is in two databases; each definition finds its own.
	for def, want := range map[*TableDef]string{&GarposL1: "gnssa_l1.garpos", &GarposL2: "gnssa_l2.garpos"} {
		if got := QualifiedName(*def); got != want {
			t.Errorf("QualifiedName = %s, want %s", got, want)
		}
	}
	if !GNSSA_L0.Has(INSPVA) || GNSSA_L0.Has(GarposL1) {
		t.Error("Has: wrong membership")
	}
	only := SFG.Only(GNSSObs, GarposL2, GarposL1)
	if len(only.Databases) != 3 || only.Databases[0].Tables[0].Name != GNSSObs.Name ||
		len(only.Databases[1].Tables) != 1 || only.Databases[2].Tables[0].Level != 2 {
		t.Errorf("Only = %+v", only)
	}
	if _, ok := SFG.Database("gnssa_l9"); ok {
		t.Error("Database(gnssa_l9) found")
	}
}
