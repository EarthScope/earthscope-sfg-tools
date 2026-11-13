package rows

import (
	"time"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// SoundSpeedRow is one sound_speed row: one depth of a profile. Time is UTC.
type SoundSpeedRow struct {
	Time  time.Time // profile time, shared by every row of the profile
	Depth float64   // positive down [m]
	Speed float64   // [m/s]
}

// BuildSoundSpeed builds sound_speed rows of stream edid.
func BuildSoundSpeed(rs []SoundSpeedRow, edid string) (arrow.Table, error) {
	b, err := newBuilder(datamodels.SoundSpeed)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	var (
		edidCol = col[*array.StringBuilder](b, datamodels.StreamEDIDName)
		tm      = col[*array.TimestampBuilder](b, "time")
		depth   = col[*array.Float64Builder](b, "depth")
		speed   = col[*array.Float64Builder](b, "speed")
	)
	for _, r := range rs {
		edidCol.Append(edid)
		tm.Append(micros(r.Time))
		depth.Append(r.Depth)
		speed.Append(r.Speed)
	}
	return b.Table()
}
