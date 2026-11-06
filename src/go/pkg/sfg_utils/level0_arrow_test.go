package sfg_utils

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"gitlab.com/earthscope/gnsstools/core/gnss/gnsstime"
)

func TestINSBatchesConvertGPSTimeToUTC(t *testing.T) {
	gps := gnsstime.GPSWeekSecToTime(2267, 580261.05)
	want := gps.Add(-18 * time.Second).UnixMicro()

	pva, err := DeserializeINSPVAARecord(
		"2267,580261.050000000,45.30245563418,-124.96561111107,-28.6138,-0.2412,0.6377,0.2949,2.627875295,0.299460630,70.416827684,INS_SOLUTION_GOOD",
		gps,
	)
	if err != nil {
		t.Fatal(err)
	}
	std, err := DeserializeINSSTDEVARecord(
		"0.0120,0.0110,0.0300,0.0040,0.0040,0.0050,0.0100,0.0110,0.0450,00000000,0,0,0,0",
		gps,
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, batch := range map[string]arrow.RecordBatch{
		"inspva":   INSPVABatch([]InspvaaRecord{pva}),
		"insstdev": INSSTDEVBatch([]INSSTDEVARecord{std}),
	} {
		got := int64(batch.Column(0).(*array.Timestamp).Value(0))
		if got != want {
			t.Errorf("%s time = %d µs, want %d (GPS - 18 s)", name, got, want)
		}
		batch.Release()
	}
}

func TestINSSTDEVBatchKeepsStatusFields(t *testing.T) {
	std, err := DeserializeINSSTDEVARecord(
		"0.0120,0.0110,0.0300,0.0040,0.0040,0.0050,0.0100,0.0110,0.0450,00000002,5,0,0,0",
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	batch := INSSTDEVBatch([]INSSTDEVARecord{std})
	defer batch.Release()
	if got := batch.Column(10).(*array.String).Value(0); got != "00000002" {
		t.Errorf("ext_sol_stat = %q", got)
	}
	if got := batch.Column(11).(*array.String).Value(0); got != "5" {
		t.Errorf("time_since_update = %q", got)
	}
	if got := batch.Column(9).(*array.Float64).Value(0); got != 0.0450 {
		t.Errorf("cov_hh = %v, want logged azimuth σ 0.045", got)
	}
}
