package tablebuild

import (
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
)

func TestTableFillsUntouchedNullable(t *testing.T) {
	b, err := New(schemas.SoundSpeed)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	sc := b.Scope(schemas.Scope{Network: "n", Station: "s", Campaign: "c"}, "CTD")
	for i := range 3 {
		sc.Append()
		Col[*array.TimestampBuilder](b, "time").Append(arrow.Timestamp(i))
		Col[*array.Float64Builder](b, "depth").Append(float64(i))
		Col[*array.Float64Builder](b, "speed").Append(1500)
	}
	tbl, err := b.Table()
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if tbl.NumRows() != 3 {
		t.Fatalf("%d rows", tbl.NumRows())
	}
	if err := schemas.SoundSpeed.Validate(tbl); err != nil {
		t.Fatal(err)
	}
}

func TestTableErrors(t *testing.T) {
	b, _ := New(schemas.SoundSpeed)
	defer b.Release()
	b.Scope(schemas.Scope{}, "CTD").Append()
	if _, err := b.Table(); err == nil {
		t.Error("want an error: required columns time/depth/speed were not filled")
	}

	b2, _ := New(schemas.SoundSpeed)
	defer b2.Release()
	b2.Scope(schemas.Scope{}, "CTD").Append()
	Col[*array.TimestampBuilder](b2, "time").Append(0)
	Col[*array.Float64Builder](b2, "depth").Append(1)
	Col[*array.Float64Builder](b2, "depth").Append(2)
	Col[*array.Float64Builder](b2, "speed").Append(1500)
	if _, err := b2.Table(); err == nil {
		t.Error("want an error: depth has 2 rows, the rest 1")
	}
}
