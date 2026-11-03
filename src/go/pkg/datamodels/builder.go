package datamodels

import (
	"fmt"
	"slices"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// Builder accumulates rows for one table.
type Builder struct {
	def TableDef
	sc  *arrow.Schema
	rb  *array.RecordBuilder
}

// LogSourceColumn is the Log_Source column every parsed table has.
const LogSourceColumn = "Log_Source"

// StampedColumns are the columns of a table that parsers do not produce:
// fileparse.Parser.Parse fills them from the scope and Log_Source.
var StampedColumns = []string{"Network", "Station", "Campaign", StreamEDIDColumn.Name}

// ParsedSchema is the schema of def's rows as a parser returns them: def's
// Arrow schema without the StampedColumns columns, with a required Log_Source
// column first if def has none (GNSSObs).
func ParsedSchema(def TableDef) (*arrow.Schema, error) {
	full, err := def.ArrowSchema()
	if err != nil {
		return nil, err
	}
	var fields []arrow.Field
	if !full.HasField(LogSourceColumn) {
		fields = append(fields, arrow.Field{Name: LogSourceColumn, Type: arrow.BinaryTypes.String})
	}
	for _, f := range full.Fields() {
		if !slices.Contains(StampedColumns, f.Name) {
			fields = append(fields, f)
		}
	}
	return arrow.NewSchema(fields, nil), nil
}

// newBuilder returns a Builder for def's parsed schema (see ParsedSchema).
func newBuilder(def TableDef) (*Builder, error) {
	sc, err := ParsedSchema(def)
	if err != nil {
		return nil, err
	}
	return &Builder{def: def, sc: sc, rb: array.NewRecordBuilder(memory.DefaultAllocator, sc)}, nil
}

// Release frees the builder's buffers.
func (b *Builder) Release() { b.rb.Release() }

// col returns the builder of the named column as T, e.g.
// col[*array.Float64Builder](b, "lat"). It panics on an unknown column or a
// wrong builder type: both are programming errors in the parser.
func col[T array.Builder](b *Builder, name string) T {
	idx := b.sc.FieldIndices(name)
	if len(idx) != 1 {
		panic(fmt.Sprintf("%s: no column %q", b.def.Name, name))
	}
	col, ok := b.rb.Field(idx[0]).(T)
	if !ok {
		panic(fmt.Sprintf("%s.%s: builder is %T", b.def.Name, name, b.rb.Field(idx[0])))
	}
	return col
}

// Table finishes the rows into an Arrow table and resets the builder.
// Untouched nullable columns become all-null; an untouched required column
// or columns of different lengths are an error.
func (b *Builder) Table() (arrow.Table, error) {
	n := 0
	for i := range b.sc.NumFields() {
		n = max(n, b.rb.Field(i).Len())
	}
	for i, f := range b.sc.Fields() {
		col := b.rb.Field(i)
		switch {
		case col.Len() == n:
		case col.Len() == 0 && f.Nullable:
			col.AppendNulls(n)
		case col.Len() == 0:
			return nil, fmt.Errorf("%s: required column %q was not filled", b.def.Name, f.Name)
		default:
			return nil, fmt.Errorf("%s: column %q has %d rows, want %d", b.def.Name, f.Name, col.Len(), n)
		}
	}
	rec := b.rb.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(b.sc, []arrow.RecordBatch{rec}), nil
}

// float64OrNull appends v, or null when ok is false.
func float64OrNull(col *array.Float64Builder, v float64, ok bool) {
	if ok {
		col.Append(v)
	} else {
		col.AppendNull()
	}
}
