package datamodels

import (
	"errors"
	"fmt"
	"slices"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// Validate checks rows against t: every schema column present, no nulls in
// required columns, and every value inside its Column's bounds and IsIn set
// (tables defined by External carry no bounds). It reports one error per
// offending column.
func (t TableDef) Validate(rows arrow.Table) error {
	schema, err := t.Schema()
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
	for _, c := range t.Columns {
		col := column(c.Name)
		if col == nil || (c.Ge == nil && c.Le == nil && c.Gt == nil && c.Lt == nil && c.IsIn == nil) {
			continue
		}
		bad, first, err := countViolations(c, col.Data().Chunks())
		if err != nil {
			errs = append(errs, fmt.Errorf("%s.%s: %w", t.Name, c.Name, err))
		} else if bad > 0 {
			errs = append(errs, fmt.Errorf("%s.%s: %d values out of range, first %v", t.Name, c.Name, bad, first))
		}
	}
	return errors.Join(errs...)
}

func (c Column) inBounds(v float64) bool {
	return (c.Ge == nil || v >= *c.Ge) && (c.Le == nil || v <= *c.Le) &&
		(c.Gt == nil || v > *c.Gt) && (c.Lt == nil || v < *c.Lt)
}

func countViolations(c Column, chunks []arrow.Array) (bad int, first any, err error) {
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
