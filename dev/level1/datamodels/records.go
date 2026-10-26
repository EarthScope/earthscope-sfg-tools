package datamodels

import (
	"time"

	"github.com/apache/arrow-go/v18/arrow"
)

// micros is t as an Arrow microsecond timestamp.
func micros(t time.Time) arrow.Timestamp { return arrow.Timestamp(t.UnixMicro()) }

// appendOpt appends *v to col, or null when v is nil.
func appendOpt[T any, B interface {
	Append(T)
	AppendNull()
}](col B, v *T) {
	if v == nil {
		col.AppendNull()
	} else {
		col.Append(*v)
	}
}
