// Package tablebuild builds full-row Arrow tables for a schemas.TableDef.
//
// File parsers append to the columns they know, by name; Table fills every
// column they never touched with nulls, so a parser does not need to know
// about columns its file type cannot supply.
package tablebuild

import (
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
)

// Builder accumulates rows for one table.
type Builder struct {
	def schemas.TableDef
	sc  *arrow.Schema
	rb  *array.RecordBuilder
}

// New returns a Builder for def's full Arrow schema.
func New(def schemas.TableDef) (*Builder, error) {
	sc, err := def.ArrowSchema()
	if err != nil {
		return nil, err
	}
	return &Builder{def: def, sc: sc, rb: array.NewRecordBuilder(memory.DefaultAllocator, sc)}, nil
}

// Release frees the builder's buffers.
func (b *Builder) Release() { b.rb.Release() }

// Col returns the builder of the named column as T, e.g.
// Col[*array.Float64Builder](b, "lat"). It panics on an unknown column or a
// wrong builder type: both are programming errors in the parser.
func Col[T array.Builder](b *Builder, name string) T {
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

// Scope appends one row of the standard Level 1 scope columns.
type Scope struct {
	network, station, campaign, logSource *array.StringBuilder
	scope                                 schemas.Scope
	source                                string
}

// Scope returns an appender for the Network, Station, Campaign and
// Log_Source columns; call Append once per row.
func (b *Builder) Scope(scope schemas.Scope, logSource string) *Scope {
	return &Scope{
		network:   Col[*array.StringBuilder](b, "Network"),
		station:   Col[*array.StringBuilder](b, "Station"),
		campaign:  Col[*array.StringBuilder](b, "Campaign"),
		logSource: Col[*array.StringBuilder](b, "Log_Source"),
		scope:     scope,
		source:    logSource,
	}
}

// Append appends the scope values for one row.
func (s *Scope) Append() {
	s.network.Append(s.scope.Network)
	s.station.Append(s.scope.Station)
	s.campaign.Append(s.scope.Campaign)
	s.logSource.Append(s.source)
}

// Float64 appends v, or null when ok is false.
func Float64(col *array.Float64Builder, v float64, ok bool) {
	if ok {
		col.Append(v)
	} else {
		col.AppendNull()
	}
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

// Unsupported is the error a file parser returns for a table its file type
// cannot produce.
func Unsupported(fileType string, def schemas.TableDef, supported ...schemas.TableDef) error {
	names := make([]string, len(supported))
	for i, s := range supported {
		names[i] = s.Name
	}
	return fmt.Errorf("%s files do not produce table %q (supported: %v)", fileType, def.Name, names)
}
