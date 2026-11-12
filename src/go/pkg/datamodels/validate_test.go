package datamodels

import (
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// soundSpeedRows is a full-row SoundSpeed table with the given speeds.
func soundSpeedRows(t *testing.T, speeds ...float64) arrow.Table {
	t.Helper()
	sc, err := SoundSpeed.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	rb := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer rb.Release()
	for i, v := range speeds {
		rb.Field(0).(*array.StringBuilder).Append("net.sta.camp.CTD.CTD")
		rb.Field(1).(*array.TimestampBuilder).Append(arrow.Timestamp(i))
		rb.Field(2).(*array.Float64Builder).Append(float64(i))
		rb.Field(3).(*array.Float64Builder).Append(v)
	}
	rec := rb.NewRecordBatch()
	defer rec.Release()
	return array.NewTableFromRecords(sc, []arrow.RecordBatch{rec})
}

func TestValidateChecks(t *testing.T) {
	ok := soundSpeedRows(t, 1480, 1500)
	defer ok.Release()
	if err := SoundSpeed.Validate(ok); err != nil {
		t.Errorf("in-range rows: %v", err)
	}

	bad := soundSpeedRows(t, 1500, 5000, 9000) // speed check: [0, 3800]
	defer bad.Release()
	err := SoundSpeed.Validate(bad)
	if err == nil || !strings.Contains(err.Error(), "sound_speed.speed: 2 values out of range, first 5000") {
		t.Errorf("out-of-range rows: %v", err)
	}
}
