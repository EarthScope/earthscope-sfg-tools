package datamodels

// The catalog's shape, declared as data: a CatalogDef holds DatabaseDefs,
// which hold TableDefs. These say what exists, not where. The environment
// (AWS Glue, a local SQLite warehouse, a REST catalog) is the caller's
// catalog configuration, so one definition builds the same databases and
// tables in any of them (see catalog.Build).

// DatabaseDef declares one database (an Iceberg namespace) and its tables.
// Table names are unique within a database, and every table has the
// database's level.
type DatabaseDef struct {
	Name   string
	Doc    string
	Tables []TableDef
}

// Table returns the database's table named name.
func (d DatabaseDef) Table(name string) (TableDef, bool) {
	for _, t := range d.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return TableDef{}, false
}

// Has reports whether the database declares a table named def.Name.
func (d DatabaseDef) Has(def TableDef) bool {
	_, ok := d.Table(def.Name)
	return ok
}

// CatalogDef declares a catalog: the databases it holds.
type CatalogDef struct {
	Name      string
	Doc       string
	Databases []DatabaseDef
}

// Database returns the catalog's database named name.
func (c CatalogDef) Database(name string) (DatabaseDef, bool) {
	for _, d := range c.Databases {
		if d.Name == name {
			return d, true
		}
	}
	return DatabaseDef{}, false
}

// DatabaseOf returns the database that declares def, matched by name and
// level: a name can be at more than one level (garpos is in gnssa_l1 and
// gnssa_l2), but not twice at one level.
func (c CatalogDef) DatabaseOf(def TableDef) (DatabaseDef, bool) {
	for _, d := range c.Databases {
		if t, ok := d.Table(def.Name); ok && t.Level == def.Level {
			return d, true
		}
	}
	return DatabaseDef{}, false
}

// Only narrows the catalog to defs: each database that declares one of
// them (by name and level, as DatabaseOf), holding just those tables, as
// given. Databases declaring none are left out. It lets a caller load or
// build part of a catalog, e.g. SFG.Only(GNSSObs).
func (c CatalogDef) Only(defs ...TableDef) CatalogDef {
	out := CatalogDef{Name: c.Name, Doc: c.Doc}
	for _, d := range c.Databases {
		db := DatabaseDef{Name: d.Name, Doc: d.Doc}
		for _, def := range defs {
			if t, ok := d.Table(def.Name); ok && t.Level == def.Level && !db.Has(def) {
				db.Tables = append(db.Tables, def)
			}
		}
		if len(db.Tables) > 0 {
			out.Databases = append(out.Databases, db)
		}
	}
	return out
}

// Tables lists every table of every database, in declaration order.
func (c CatalogDef) Tables() []TableDef {
	var out []TableDef
	for _, d := range c.Databases {
		out = append(out, d.Tables...)
	}
	return out
}

// The SFG databases.
var (
	GNSSA_L0 = DatabaseDef{
		Name:   "gnssa_l0",
		Doc:    "Level 0: records parsed from raw files, one table per record type.",
		Tables: Level0Tables,
	}
	GNSSA_L1 = DatabaseDef{
		Name:   "gnssa_l1",
		Doc:    "Level 1: PPP and smoothed antenna tracks, and GNSS-A built with Level 0 antenna positions.",
		Tables: Level1Tables,
	}
	GNSSA_L2 = DatabaseDef{
		Name:   "gnssa_l2",
		Doc:    "Level 2: GNSS-A built with the Level 1 smoothed antenna track.",
		Tables: Level2Tables,
	}
)

// SFG is the seafloor geodesy catalog: every database and table.
var SFG = CatalogDef{
	Name:      "sfg",
	Doc:       "Seafloor geodesy (GNSS-A) data, Level 0 to 2.",
	Databases: []DatabaseDef{GNSSA_L0, GNSSA_L1, GNSSA_L2},
}

// DatabaseOf returns the SFG database that declares def.
func DatabaseOf(def TableDef) (DatabaseDef, bool) { return SFG.DatabaseOf(def) }

// QualifiedName is def's name qualified by its SFG database, e.g.
// gnssa_l0.inspva; the bare name if no SFG database declares it.
func QualifiedName(def TableDef) string {
	if db, ok := DatabaseOf(def); ok {
		return db.Name + "." + def.Name
	}
	return def.Name
}
