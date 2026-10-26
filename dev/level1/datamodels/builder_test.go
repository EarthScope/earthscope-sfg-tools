package datamodels

import (
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

func TestTableFillsUntouchedNullable(t *testing.T) {
	b, err := newBuilder(SoundSpeed)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	logSource := col[*array.StringBuilder](b, LogSourceColumn)
	for i := range 3 {
		logSource.Append("CTD")
		col[*array.TimestampBuilder](b, "time").Append(arrow.Timestamp(i))
		col[*array.Float64Builder](b, "depth").Append(float64(i))
		col[*array.Float64Builder](b, "speed").Append(1500)
	}
	tbl, err := b.Table()
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if tbl.NumRows() != 3 {
		t.Fatalf("%d rows", tbl.NumRows())
	}
	if want, _ := ParsedSchema(SoundSpeed); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want %v", tbl.Schema(), want)
	}
}

func TestTableErrors(t *testing.T) {
	b, _ := newBuilder(SoundSpeed)
	defer b.Release()
	col[*array.StringBuilder](b, LogSourceColumn).Append("CTD")
	if _, err := b.Table(); err == nil {
		t.Error("want an error: required columns time/depth/speed were not filled")
	}

	b2, _ := newBuilder(SoundSpeed)
	defer b2.Release()
	col[*array.StringBuilder](b2, LogSourceColumn).Append("CTD")
	col[*array.TimestampBuilder](b2, "time").Append(0)
	col[*array.Float64Builder](b2, "depth").Append(1)
	col[*array.Float64Builder](b2, "depth").Append(2)
	col[*array.Float64Builder](b2, "speed").Append(1500)
	if _, err := b2.Table(); err == nil {
		t.Error("want an error: depth has 2 rows, the rest 1")
	}
}

func TestParsedSchema(t *testing.T) {
	for _, def := range []TableDef{SoundSpeed, GNSSObs} {
		sc, err := ParsedSchema(def)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range StampedColumns {
			if sc.HasField(name) {
				t.Errorf("%s: parsed schema has stamped column %s", def.Name, name)
			}
		}
		idx := sc.FieldIndices(LogSourceColumn)
		if len(idx) != 1 || sc.Field(idx[0]).Nullable {
			t.Errorf("%s: parsed schema needs one required %s, got %v", def.Name, LogSourceColumn, idx)
		}
		full, _ := def.ArrowSchema()
		want := full.NumFields() - len(StampedColumns) + 1 // + Log_Source
		if full.HasField(LogSourceColumn) {
			want--
		}
		if def.Name == GNSSObs.Name {
			want += 3 // GNSSObs has no Network/Station/Campaign to drop
		}
		if sc.NumFields() != want {
			t.Errorf("%s: parsed schema has %d fields, want %d", def.Name, sc.NumFields(), want)
		}
	}
}
