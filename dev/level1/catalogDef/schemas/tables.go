package schemas

import (
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// Go counterparts of the Python TableDefs in
// earthscope_sfg_tools.datamodels.iceberg.schemas. Keep the two in step:
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
	// PartitionBy defaults to identity on Network, Station, Campaign.
	PartitionBy []PartitionKey
	// SortBy names the columns of the table's ascending sort order.
	SortBy []string
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

// level1 prefixes cols with the Level 1 scope columns.
func level1(cols ...Column) []Column {
	return append(append([]Column{}, Level1Scope...), cols...)
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

// Schema is the Iceberg schema, with field IDs in declaration order.
func (t TableDef) Schema() *iceberg.Schema {
	fields := make([]iceberg.NestedField, len(t.Columns))
	for i, c := range t.Columns {
		fields[i] = iceberg.NestedField{
			ID: i + 1, Name: c.Name, Type: c.Type, Required: c.Required, Doc: c.Doc,
		}
	}
	return iceberg.NewSchema(0, fields...)
}

// PartitionSpec builds the spec from PartitionBy (scope identity by default).
func (t TableDef) PartitionSpec() (iceberg.PartitionSpec, error) {
	keys := t.PartitionBy
	if keys == nil {
		keys = scopePartitions
	}
	schema := t.Schema()
	opts := make([]iceberg.PartitionOption, len(keys))
	for i, k := range keys {
		name := k.Name
		if name == "" {
			name = k.Source
		}
		// Iceberg reserves IDs >= 1000 for partition fields (as in the Python TableDef).
		id := 1000 + i
		opts[i] = iceberg.AddPartitionFieldByName(k.Source, name, k.Transform, schema, &id)
	}
	spec, err := iceberg.NewPartitionSpecOpts(opts...)
	if err != nil {
		return iceberg.PartitionSpec{}, fmt.Errorf("%s: partition spec: %w", t.Name, err)
	}
	return spec, nil
}

// SortOrder is an ascending, nulls-first sort on SortBy; unsorted when empty.
func (t TableDef) SortOrder() (table.SortOrder, error) {
	if len(t.SortBy) == 0 {
		return table.UnsortedSortOrder, nil
	}
	schema := t.Schema()
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

// ArrowSchema is the Arrow schema of a full row, for building write batches.
func (t TableDef) ArrowSchema() (*arrow.Schema, error) {
	return table.SchemaToArrowSchema(t.Schema(), nil, true, false)
}
