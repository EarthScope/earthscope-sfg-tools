package datamodels

import (
	"fmt"
	"maps"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// Go counterparts of the Python TableDefs in
// earthscope_sfg_tools.datamodels.iceberg, except that every Go table's
// only identity column is stream_edid (no Network, Station, Campaign or
// Log_Source columns), so the Python definitions no longer match.
// Iceberg tracks columns by field ID, which is assigned in declaration order
// starting at 1, so new columns must only ever be appended.

// field is shorthand for an Iceberg schema field in the table definitions.
// Definitions leave ID zero: newSchema numbers fields in declaration order.
type field = iceberg.NestedField

// Check is the value constraint of one column, enforced at ingest by
// Validate.
type Check struct {
	// Inclusive (Ge, Le) and exclusive (Gt, Lt) bounds; nil means unbounded.
	Ge, Le, Gt, Lt *float64
	// IsIn lists the allowed values of a string column; nil allows any.
	IsIn []string
}

// checks merges check sets, e.g. a shared column group's with a table's own.
func checks(sets ...map[string]Check) map[string]Check {
	out := map[string]Check{}
	for _, s := range sets {
		maps.Copy(out, s)
	}
	return out
}

// newSchema is the schema of fields, numbered 1, 2, ... in order.
func newSchema(fields ...field) *iceberg.Schema {
	numbered := make([]field, len(fields))
	for i, f := range fields {
		f.ID = i + 1
		numbered[i] = f
	}
	return iceberg.NewSchema(0, numbered...)
}

// PartitionKey is one partition field: a transform applied to a source column.
type PartitionKey struct {
	Source    string
	Transform iceberg.Transform
	Name      string // defaults to Source
}

// TableDef is an Iceberg table definition: name, level, schema and layout.
type TableDef struct {
	Name  string
	Level int
	Doc   string
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
	// library that owns it (see GNSSObs); Schema, Checks and SortBy are
	// then unused, and PartitionBy, if set, overrides its partition spec.
	External func() (External, error)
	// TimeColumn is the timestamp column that orders a stream's rows.
	// Table.Append replaces existing rows by (stream_edid, TimeColumn
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

// StreamEDIDName names the stream_edid column every table declares first:
// the rows' stream EDID (see StreamEDID, DerivedEDID), which parsers fill.
// It is the only identity column: network, station, campaign, file type
// and log source are its fields, not columns of their own.
const StreamEDIDName = "stream_edid"

// streamPartitions is the default partitioning: one partition per stream.
// A stream EDID starts network.station., so its partitions nest within
// each network and station.
var streamPartitions = []PartitionKey{
	{Source: StreamEDIDName, Transform: iceberg.IdentityTransform{}},
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

// AllTables lists every table of the SFG catalog, Level 0 to 2.
func AllTables() []TableDef { return SFG.Tables() }

// TableByName returns the SFG table named name, either qualified by its
// database (gnssa_l1.garpos) or bare (inspva). A bare name in more than one
// database is an error.
func TableByName(name string) (TableDef, error) {
	var found []TableDef
	for _, db := range SFG.Databases {
		for _, def := range db.Tables {
			if name == def.Name || name == db.Name+"."+def.Name {
				found = append(found, def)
			}
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
			names[i] = QualifiedName(def)
		}
		return TableDef{}, fmt.Errorf("table %q is ambiguous: one of %s", name, strings.Join(names, ", "))
	}
}
