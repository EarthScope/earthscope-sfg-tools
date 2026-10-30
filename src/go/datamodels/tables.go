package datamodels

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// Go counterparts of the Python TableDefs in
// earthscope_sfg_tools.datamodels.iceberg. Keep the two in step:
// Iceberg tracks columns by field ID, which is assigned in declaration order
// starting at 1, so new columns must only ever be appended.

// Column is one table column plus the value constraints enforced at ingest.
type Column struct {
	Name     string
	Type     iceberg.Type
	Doc      string
	Required bool
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

// TableDef is an Iceberg table definition: name, level, columns and layout.
type TableDef struct {
	Name    string
	Level   int
	Doc     string
	Columns []Column
	// PartitionBy defaults to identity on Network, Station, Campaign, or to
	// External's spec. Set on an External table, it overrides that spec;
	// keys name columns of External's schema.
	PartitionBy []PartitionKey
	// SortBy names the columns of the table's ascending sort order.
	SortBy []string
	// External, when set, supplies the whole table definition from the
	// library that owns it (see GNSSObs); Columns and SortBy are then
	// unused, and PartitionBy, if set, overrides its partition spec.
	External func() (External, error)
	// TimeColumn is the timestamp column that orders a stream's rows.
	// Catalog.Append replaces existing rows by (stream_edid, TimeColumn
	// window), so re-ingesting a file does not duplicate its rows.
	TimeColumn string
}

// External is a complete table definition owned by another library.
type External struct {
	Schema     *iceberg.Schema
	Spec       iceberg.PartitionSpec
	SortOrder  table.SortOrder
	Properties iceberg.Properties
}

// partitionFieldIDStart is the first partition field ID, by Iceberg convention.
const partitionFieldIDStart = 1000

func bound(v float64) *float64 { return &v }

var (
	str         = iceberg.PrimitiveTypes.String
	boolType    = iceberg.PrimitiveTypes.Bool
	int32Type   = iceberg.PrimitiveTypes.Int32
	float32Type = iceberg.PrimitiveTypes.Float32
	float64Type = iceberg.PrimitiveTypes.Float64
	timestamptz = iceberg.PrimitiveTypes.TimestampTz
)

// Level1Scope holds the columns every Level 1 table starts with.
var Level1Scope = []Column{
	{Name: "Network", Type: str, Doc: "Network identifier", Required: true},
	{Name: "Station", Type: str, Doc: "Station identifier", Required: true},
	{Name: "Campaign", Type: str, Doc: "Campaign identifier", Required: true},
	{Name: "Log_Source", Type: str, Doc: "Source log file or log type", Required: true},
}

// StreamEDIDColumn is the last column of every Level 1 table built with
// level1: the rows' stream EDID (see StreamEDID). It is last, and nullable,
// because it was added after the tables were defined: Iceberg assigns field
// IDs in declaration order and only allows new columns to be optional. The
// Go parsers always set it; the Python ingest leaves it null.
var StreamEDIDColumn = Column{Name: "stream_edid", Type: str,
	Doc: "Stream EDID, network:station:campaign:file_type:log_source"}

// level1 wraps cols in the Level 1 scope columns and StreamEDIDColumn.
func level1(cols ...Column) []Column {
	return append(append(append([]Column{}, Level1Scope...), cols...), StreamEDIDColumn)
}

var scopePartitions = []PartitionKey{
	{Source: "Network", Transform: iceberg.IdentityTransform{}},
	{Source: "Station", Transform: iceberg.IdentityTransform{}},
	{Source: "Campaign", Transform: iceberg.IdentityTransform{}},
}

// Namespace is the catalog namespace of the table's level, e.g. gnssa_l1.
func (t TableDef) Namespace() string { return fmt.Sprintf("gnssa_l%d", t.Level) }

// Identifier is the catalog identifier (namespace, name).
func (t TableDef) Identifier() table.Identifier { return table.Identifier{t.Namespace(), t.Name} }

// Column returns the named column.
func (t TableDef) Column(name string) (Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}

// Schema is the Iceberg schema: External's, or Columns with field IDs in
// declaration order.
func (t TableDef) Schema() (*iceberg.Schema, error) {
	if t.External != nil {
		ext, err := t.External()
		return ext.Schema, err
	}
	return t.columnSchema(), nil
}

func (t TableDef) columnSchema() *iceberg.Schema {
	fields := make([]iceberg.NestedField, len(t.Columns))
	for i, c := range t.Columns {
		fields[i] = iceberg.NestedField{
			ID: i + 1, Name: c.Name, Type: c.Type, Required: c.Required, Doc: c.Doc,
		}
	}
	return iceberg.NewSchema(0, fields...)
}

// PartitionSpec is built from PartitionBy when set, against External's
// schema for an External table; otherwise it is External's spec, or scope
// identity.
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
		keys = scopePartitions
	}
	return t.buildSpec(t.columnSchema(), keys)
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
	schema := t.columnSchema()
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
	sc, err := t.Schema()
	if err != nil {
		return nil, err
	}
	return table.SchemaToArrowSchema(sc, nil, true, false)
}

// AllTables lists every table definition, Level 1 to 3.
func AllTables() []TableDef {
	return slices.Concat(Level1Tables, Level2Tables, Level3Tables)
}

// TableByName returns the table named name, either qualified
// (gnssa_l2.garpos) or bare (inspva). A bare name shared by more than one
// level is an error.
func TableByName(name string) (TableDef, error) {
	var found []TableDef
	for _, def := range AllTables() {
		if name == def.Name || name == def.Namespace()+"."+def.Name {
			found = append(found, def)
		}
	}
	switch len(found) {
	case 0:
		return TableDef{}, fmt.Errorf("no table %q", name)
	case 1:
		return found[0], nil
	default:
		names := make([]string, len(found))
		for i, def := range found {
			names[i] = def.Namespace() + "." + def.Name
		}
		return TableDef{}, fmt.Errorf("table %q is ambiguous: one of %s", name, strings.Join(names, ", "))
	}
}
