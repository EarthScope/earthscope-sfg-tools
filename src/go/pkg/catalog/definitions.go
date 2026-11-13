package catalog

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// The catalog's shape, declared as data: a CatalogDef holds DatabaseDefs,
// which hold TableDefs. These say what exists, not where. The environment
// (AWS Glue, a local SQLite warehouse, a REST catalog) is the caller's
// Config, so one definition builds the same databases and tables in any of
// them (see Build). Package datamodels declares the SFG definitions.
//
// A database is a level: gnssa_l0 holds parsed records, gnssa_l1 and
// gnssa_l2 products built from them. A TableDef carries no level, so one
// definition can be listed in several databases (community_standard is in
// all three); a TableRef names one of those tables.

// StreamEDIDName names the stream_edid column every table declares first:
// the rows' stream EDID, which parsers fill. It is the only identity
// column: network, station, campaign, file type and log source are its
// fields, not columns of their own.
const StreamEDIDName = "stream_edid"

// partitionFieldIDStart is the first partition field ID, by Iceberg convention.
const partitionFieldIDStart = 1000

// streamPartitions is the default partitioning: one partition per stream.
// A stream EDID starts network.station., so its partitions nest within
// each network and station.
var streamPartitions = []PartitionKey{
	{Source: StreamEDIDName, Transform: iceberg.IdentityTransform{}},
}

// Check is the value constraint of one column, enforced at ingest by
// Validate.
type Check struct {
	// Inclusive (Ge, Le) and exclusive (Gt, Lt) bounds; nil means unbounded.
	Ge, Le, Gt, Lt *float64
	// IsIn lists the allowed values of a string column; nil allows any.
	IsIn []string
}

// PartitionKey is one partition field: a transform applied to a source column.
type PartitionKey struct {
	Source    string
	Transform iceberg.Transform
	Name      string // defaults to Source
}

// External is a complete table definition owned by another library.
type External struct {
	Schema     *iceberg.Schema
	Spec       iceberg.PartitionSpec
	SortOrder  table.SortOrder
	Properties iceberg.Properties
}

// TableDef is an Iceberg table definition: name, schema and layout. It says
// nothing of where the table lives: a DatabaseDef lists it, and the same
// definition can be in several databases (levels).
type TableDef struct {
	Name string
	Doc  string
	// Schema is the table's Iceberg schema; nil when External is set (use
	// IcebergSchema). Iceberg tracks columns by field ID, so a new column
	// must only be appended, taking the next ID.
	Schema *iceberg.Schema
	// Checks are value constraints by column name, enforced by Validate.
	Checks map[string]Check
	// PartitionBy defaults to identity on stream_edid, or to
	// External's spec. Set on an External table, it overrides that spec;
	// keys name columns of External's schema.
	PartitionBy []PartitionKey
	// SortBy names the columns of the table's ascending sort order.
	SortBy []string
	// External, when set, supplies the whole table definition from the
	// library that owns it (see datamodels.GNSSObs); Schema, Checks and
	// SortBy are then unused, and PartitionBy, if set, overrides its
	// partition spec.
	External func() (External, error)
	// TimeColumn is the timestamp column that orders a stream's rows.
	// Table.Append replaces existing rows by (stream_edid, TimeColumn
	// window), so re-ingesting a file does not duplicate its rows.
	TimeColumn string
}

// IcebergSchema is the table's Iceberg schema: Schema, or External's.
func (t TableDef) IcebergSchema() (*iceberg.Schema, error) {
	if t.External != nil {
		ext, err := t.External()
		return ext.Schema, err
	}
	if t.Schema == nil {
		return nil, fmt.Errorf("%s: no schema", t.Name)
	}
	return t.Schema, nil
}

// PartitionSpec is built from PartitionBy when set, against External's
// schema for an External table; otherwise it is External's spec, or
// stream_edid identity.
func (t TableDef) PartitionSpec() (iceberg.PartitionSpec, error) {
	if t.External != nil {
		ext, err := t.External()
		if err != nil || t.PartitionBy == nil {
			return ext.Spec, err
		}
		return t.buildSpec(ext.Schema, t.PartitionBy)
	}
	keys := t.PartitionBy
	if keys == nil {
		keys = streamPartitions
	}
	schema, err := t.IcebergSchema()
	if err != nil {
		return iceberg.PartitionSpec{}, err
	}
	return t.buildSpec(schema, keys)
}

// buildSpec is the partition spec of keys over schema's columns.
func (t TableDef) buildSpec(schema *iceberg.Schema, keys []PartitionKey) (iceberg.PartitionSpec, error) {
	opts := make([]iceberg.PartitionOption, len(keys))
	for i, k := range keys {
		name := k.Name
		if name == "" {
			name = k.Source
		}
		// Iceberg reserves IDs >= 1000 for partition fields (as in the Python TableDef).
		id := partitionFieldIDStart + i
		opts[i] = iceberg.AddPartitionFieldByName(k.Source, name, k.Transform, schema, &id)
	}
	spec, err := iceberg.NewPartitionSpecOpts(opts...)
	if err != nil {
		return iceberg.PartitionSpec{}, fmt.Errorf("%s: partition spec: %w", t.Name, err)
	}
	return spec, nil
}

// SortOrder is External's sort order, or an ascending, nulls-first sort on
// SortBy; unsorted when empty.
func (t TableDef) SortOrder() (table.SortOrder, error) {
	if t.External != nil {
		ext, err := t.External()
		return ext.SortOrder, err
	}
	if len(t.SortBy) == 0 {
		return table.UnsortedSortOrder, nil
	}
	schema, err := t.IcebergSchema()
	if err != nil {
		return table.SortOrder{}, err
	}
	fields := make([]table.SortField, len(t.SortBy))
	for i, name := range t.SortBy {
		f, ok := schema.FindFieldByName(name)
		if !ok {
			return table.SortOrder{}, fmt.Errorf("%s: unknown sort column %q", t.Name, name)
		}
		fields[i] = table.SortField{
			SourceIDs: []int{f.ID}, Transform: iceberg.IdentityTransform{},
			Direction: table.SortASC, NullOrder: table.NullsFirst,
		}
	}
	return table.NewSortOrder(table.InitialSortOrderID, fields)
}

// Properties are the table properties to create the table with: External's
// plus the table doc as "comment".
func (t TableDef) Properties() (iceberg.Properties, error) {
	props := iceberg.Properties{}
	if t.External != nil {
		ext, err := t.External()
		if err != nil {
			return nil, err
		}
		maps.Copy(props, ext.Properties)
	}
	props["comment"] = t.Doc
	return props, nil
}

// ArrowSchema is the Arrow schema of a full row, for building write batches.
func (t TableDef) ArrowSchema() (*arrow.Schema, error) {
	sc, err := t.IcebergSchema()
	if err != nil {
		return nil, err
	}
	return table.SchemaToArrowSchema(sc, nil, true, false)
}

// Validate checks rows against t: every schema column present, no nulls in
// required columns, and every value inside its column's Check bounds and
// IsIn set (tables defined by External carry no checks). It reports one
// error per offending column.
func (t TableDef) Validate(rows arrow.Table) error {
	schema, err := t.IcebergSchema()
	if err != nil {
		return err
	}
	sc := rows.Schema()
	column := func(name string) *arrow.Column {
		if idx := sc.FieldIndices(name); len(idx) > 0 {
			return rows.Column(idx[0])
		}
		return nil
	}
	var errs []error
	for _, f := range schema.Fields() {
		col := column(f.Name)
		switch {
		case col == nil:
			errs = append(errs, fmt.Errorf("%s.%s: missing column", t.Name, f.Name))
		case f.Required && col.NullN() > 0:
			errs = append(errs, fmt.Errorf("%s.%s: %d nulls in a required column", t.Name, f.Name, col.NullN()))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(t.Checks)) {
		c := t.Checks[name]
		col := column(name)
		if col == nil || (c.Ge == nil && c.Le == nil && c.Gt == nil && c.Lt == nil && c.IsIn == nil) {
			continue
		}
		bad, first, err := countViolations(c, col.Data().Chunks())
		if err != nil {
			errs = append(errs, fmt.Errorf("%s.%s: %w", t.Name, name, err))
		} else if bad > 0 {
			errs = append(errs, fmt.Errorf("%s.%s: %d values out of range, first %v", t.Name, name, bad, first))
		}
	}
	return errors.Join(errs...)
}

func (c Check) inBounds(v float64) bool {
	return (c.Ge == nil || v >= *c.Ge) && (c.Le == nil || v <= *c.Le) &&
		(c.Gt == nil || v > *c.Gt) && (c.Lt == nil || v < *c.Lt)
}

func countViolations(c Check, chunks []arrow.Array) (bad int, first any, err error) {
	check := func(ok bool, v any) {
		if !ok {
			if bad == 0 {
				first = v
			}
			bad++
		}
	}
	for _, chunk := range chunks {
		for i := 0; i < chunk.Len(); i++ {
			if chunk.IsNull(i) {
				continue
			}
			switch a := chunk.(type) {
			case *array.Float64:
				check(c.inBounds(a.Value(i)), a.Value(i))
			case *array.Float32:
				check(c.inBounds(float64(a.Value(i))), a.Value(i))
			case *array.Int32:
				check(c.inBounds(float64(a.Value(i))), a.Value(i))
			case *array.Int64:
				check(c.inBounds(float64(a.Value(i))), a.Value(i))
			case *array.String:
				check(c.IsIn == nil || slices.Contains(c.IsIn, a.Value(i)), a.Value(i))
			default:
				return 0, nil, fmt.Errorf("no constraint check for type %s", chunk.DataType())
			}
		}
	}
	return bad, first, nil
}

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
