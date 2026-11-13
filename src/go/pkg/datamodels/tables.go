package datamodels

import (
	"maps"

	"github.com/apache/iceberg-go"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
)

// field is shorthand for an Iceberg schema field in the table definitions.
// Definitions leave ID zero: newSchema numbers fields in declaration order.
type field = iceberg.NestedField

// checks merges check sets, e.g. a shared column group's with a table's own.
func checks(sets ...map[string]catalog.Check) map[string]catalog.Check {
	out := map[string]catalog.Check{}
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

func bound(v float64) *float64 { return &v }

var (
	str         = iceberg.PrimitiveTypes.String
	boolType    = iceberg.PrimitiveTypes.Bool
	int32Type   = iceberg.PrimitiveTypes.Int32
	float32Type = iceberg.PrimitiveTypes.Float32
	float64Type = iceberg.PrimitiveTypes.Float64
	timestamptz = iceberg.PrimitiveTypes.TimestampTz
)

// StreamEDIDName names the stream_edid column every table declares first
// (see catalog.StreamEDIDName); parsers fill it with the rows' stream EDID.
const StreamEDIDName = catalog.StreamEDIDName
