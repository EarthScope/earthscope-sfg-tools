package catalog

import (
	"fmt"
	"strings"
)

// The catalog's shape, declared as data: a CatalogDef holds DatabaseDefs,
// which hold TableDefs (tables.go). These say what exists, not where. The environment
// (AWS Glue, a local SQLite warehouse, a REST catalog) is the caller's
// Config, so one definition builds the same databases and tables in any of
// them (see Build). Package datamodels declares the SFG definitions.
//
// A database is a level: gnssa_l0 holds parsed records, gnssa_l1 and
// gnssa_l2 products built from them. A TableDef carries no level, so one
// definition can be listed in several databases (community_standard is in
// all three); a TableRef names one of those tables.

// DatabaseDef declares one database (an Iceberg namespace) and its tables.
// Table names are unique within a database.
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

// Ref is def's table in this database. It does not check that the
// database declares def; the catalog does, when the ref is looked up.
func (d DatabaseDef) Ref(def TableDef) TableRef { return TableRef{Database: d.Name, Def: def} }

// Refs are the database's tables.
func (d DatabaseDef) Refs() []TableRef {
	out := make([]TableRef, len(d.Tables))
	for i, def := range d.Tables {
		out[i] = d.Ref(def)
	}
	return out
}

// TableRef names one table of a catalog: a definition in a database.
type TableRef struct {
	Database string
	Def      TableDef
}

// String is the table's qualified name, e.g. gnssa_l0.inspva.
func (r TableRef) String() string { return r.Database + "." + r.Def.Name }

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

// Refs lists every table of every database, in declaration order.
func (c CatalogDef) Refs() []TableRef {
	var out []TableRef
	for _, d := range c.Databases {
		out = append(out, d.Refs()...)
	}
	return out
}

// Find returns the table named name: qualified by its database
// (gnssa_l1.garpos) or bare (inspva). A bare name in more than one database
// is an error.
func (c CatalogDef) Find(name string) (TableRef, error) {
	var found []TableRef
	for _, ref := range c.Refs() {
		if name == ref.Def.Name || name == ref.String() {
			found = append(found, ref)
		}
	}
	switch len(found) {
	case 0:
		return TableRef{}, fmt.Errorf("no table %q", name)
	case 1:
		return found[0], nil
	default:
		names := make([]string, len(found))
		for i, ref := range found {
			names[i] = ref.String()
		}
		return TableRef{}, fmt.Errorf("table %q is ambiguous: one of %s", name, strings.Join(names, ", "))
	}
}

// Only narrows the catalog to refs: each database holding one of them, with
// just those tables, as given (the definitions in refs, not the catalog's).
// Databases holding none are left out, as are refs to databases the
// catalog lacks. It lets a caller build or load part of a catalog, e.g.
// datamodels.SFG.Only(datamodels.GNSSA_L0.Ref(datamodels.GNSSObs)).
func (c CatalogDef) Only(refs ...TableRef) CatalogDef {
	out := CatalogDef{Name: c.Name, Doc: c.Doc}
	for _, d := range c.Databases {
		db := DatabaseDef{Name: d.Name, Doc: d.Doc}
		for _, ref := range refs {
			if ref.Database == d.Name && d.Has(ref.Def) && !db.Has(ref.Def) {
				db.Tables = append(db.Tables, ref.Def)
			}
		}
		if len(db.Tables) > 0 {
			out.Databases = append(out.Databases, db)
		}
	}
	return out
}
