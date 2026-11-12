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
	edid := col[*array.StringBuilder](b, StreamEDIDName)
	for i := range 3 {
		edid.Append("n.s.c.CTD.CTD")
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
	if want, _ := SoundSpeed.ArrowSchema(); !tbl.Schema().Equal(want) {
		t.Fatalf("schema = %v, want %v", tbl.Schema(), want)
	}
}

func TestTableErrors(t *testing.T) {
	b, _ := newBuilder(SoundSpeed)
	defer b.Release()
	col[*array.StringBuilder](b, StreamEDIDName).Append("n.s.c.CTD.CTD")
	if _, err := b.Table(); err == nil {
		t.Error("want an error: required columns time/depth/speed were not filled")
	}

	b2, _ := newBuilder(SoundSpeed)
	defer b2.Release()
	col[*array.StringBuilder](b2, StreamEDIDName).Append("n.s.c.CTD.CTD")
	col[*array.TimestampBuilder](b2, "time").Append(0)
	col[*array.Float64Builder](b2, "depth").Append(1)
	col[*array.Float64Builder](b2, "depth").Append(2)
	col[*array.Float64Builder](b2, "speed").Append(1500)
	if _, err := b2.Table(); err == nil {
		t.Error("want an error: depth has 2 rows, the rest 1")
	}
}
