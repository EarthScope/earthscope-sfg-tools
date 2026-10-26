package sink

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/catalog"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
)

// rows returns n rows of def with every column set to its type's zero
// value, which Validate accepts.
func rows(t *testing.T, def datamodels.TableDef, n int) arrow.Table {
	t.Helper()
	sc, err := def.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	for i, f := range sc.Fields() {
		for range n {
			switch fb := b.Field(i).(type) {
			case *array.StringBuilder:
				fb.Append("x")
			case *array.Int32Builder:
				fb.Append(0)
			case *array.Int64Builder:
				fb.Append(0)
			case *array.Float32Builder:
				fb.Append(0)
			case *array.Float64Builder:
				fb.Append(0)
			case *array.BooleanBuilder:
				fb.Append(false)
			case *array.TimestampBuilder:
				fb.Append(arrow.Timestamp(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMicro()))
			default:
				t.Fatalf("%s.%s: no test value for %s", def.Name, f.Name, f.Type)
			}
		}
	}
	rec := b.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

// wrongSchema is a table no Level 1 def accepts.
func wrongSchema() arrow.Table {
	sc := arrow.NewSchema([]arrow.Field{{Name: "x", Type: arrow.PrimitiveTypes.Int64}}, nil)
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	b.Field(0).(*array.Int64Builder).Append(1)
	rec := b.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

// sinks returns one of each Sink, writing under t.TempDir().
func sinks(t *testing.T) map[string]Sink {
	t.Helper()
	ice, err := OpenIceberg(context.Background(), catalog.Config{Type: catalog.TypeSQL, Warehouse: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ice.Close() })
	return map[string]Sink{"iceberg": ice, "parquet": NewParquet(t.TempDir())}
}

func TestSinksValidate(t *testing.T) {
	bad := wrongSchema()
	defer bad.Release()
	for name, s := range sinks(t) {
		if _, err := s.Write(context.Background(), datamodels.AttitudeAHRS, bad); err == nil {
			t.Errorf("%s: wrote a table that does not match %s", name, datamodels.AttitudeAHRS.Name)
		}
	}
}

func TestParquetRoundTrip(t *testing.T) {
	def := datamodels.AttitudeAHRS
	in := rows(t, def, 3)
	defer in.Release()

	dir := t.TempDir()
	s := NewParquet(dir)
	n, err := s.Write(context.Background(), def, in)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("wrote %d rows, want 3", n)
	}
	paths, _ := filepath.Glob(filepath.Join(dir, def.Namespace(), def.Name, "*.parquet"))
	if len(paths) != 1 {
		t.Fatalf("%d parquet files, want 1", len(paths))
	}
	f, err := os.Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pf, err := file.NewParquetReader(f)
	if err != nil {
		t.Fatal(err)
	}
	fr, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fr.ReadTable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()
	if got.NumRows() != 3 {
		t.Errorf("read %d rows, want 3", got.NumRows())
	}
	if err := def.Validate(got); err != nil {
		t.Errorf("read-back table fails %s: %v", def.Name, err)
	}
}
