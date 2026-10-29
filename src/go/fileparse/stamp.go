package fileparse

import (
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/EarthScope/es_sfgtools/src/golangtools/datamodels"
)

// stamp returns parsed (rows in datamodels.ParsedSchema form) as a table
// with p.Table's full schema: Network, Station and Campaign from scope,
// stream_edid from each row's Log_Source, every other column taken from
// parsed by name. Columns keep parsed's chunking.
func (p Parser) stamp(parsed arrow.Table, scope datamodels.Scope) (arrow.Table, error) {
	sc, err := p.Table.ArrowSchema()
	if err != nil {
		return nil, err
	}
	logSource, err := parsedColumn(parsed, datamodels.LogSourceColumn, arrow.BinaryTypes.String)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", p.FileType, p.Table.Name, err)
	}
	edids := map[string]string{}
	edid := func(ls string) string {
		e, ok := edids[ls]
		if !ok {
			e = p.EDID(scope, ls)
			edids[ls] = e
		}
		return e
	}

	cols := make([]arrow.Column, 0, sc.NumFields())
	defer func() {
		for i := range cols {
			cols[i].Release()
		}
	}()
	for _, f := range sc.Fields() {
		var data *arrow.Chunked
		switch f.Name {
		case "Network":
			data = constant(logSource, scope.Network)
		case "Station":
			data = constant(logSource, scope.Station)
		case "Campaign":
			data = constant(logSource, scope.Campaign)
		case datamodels.StreamEDIDColumn.Name:
			data = mapStrings(logSource, edid)
		default:
			c, err := parsedColumn(parsed, f.Name, f.Type)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", p.FileType, p.Table.Name, err)
			}
			data = c.Data()
			data.Retain()
		}
		cols = append(cols, *arrow.NewColumn(f, data))
		data.Release()
	}
	return array.NewTable(sc, cols, parsed.NumRows()), nil
}

// parsedColumn returns parsed's column name, which must have type typ.
func parsedColumn(parsed arrow.Table, name string, typ arrow.DataType) (*arrow.Column, error) {
	idx := parsed.Schema().FieldIndices(name)
	if len(idx) != 1 {
		return nil, fmt.Errorf("parser returned no %q column", name)
	}
	c := parsed.Column(idx[0])
	if !arrow.TypeEqual(c.DataType(), typ) {
		return nil, fmt.Errorf("parser returned %q as %s, want %s", name, c.DataType(), typ)
	}
	return c, nil
}

// constant is a string column of v, chunked like like.
func constant(like *arrow.Column, v string) *arrow.Chunked {
	return mapStrings(like, func(string) string { return v })
}

// mapStrings is f applied to each value of the string column src, chunked
// like src.
func mapStrings(src *arrow.Column, f func(string) string) *arrow.Chunked {
	b := array.NewStringBuilder(memory.DefaultAllocator)
	defer b.Release()
	var chunks []arrow.Array
	for _, chunk := range src.Data().Chunks() {
		s := chunk.(*array.String)
		b.Reserve(s.Len())
		for i := range s.Len() {
			b.Append(f(s.Value(i)))
		}
		chunks = append(chunks, b.NewArray())
	}
	data := arrow.NewChunked(arrow.BinaryTypes.String, chunks)
	for _, c := range chunks {
		c.Release()
	}
	return data
}
