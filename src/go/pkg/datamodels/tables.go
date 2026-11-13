package datamodels

import (
	"fmt"
	"maps"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

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
