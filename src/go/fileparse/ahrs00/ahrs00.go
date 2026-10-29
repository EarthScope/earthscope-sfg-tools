// Package ahrs00 parses Sonardyne AHRS00.bin logs (XSENS MTData2 wrapped in
// GPSA packets) into the Level 1 attitude_ahrs table.
package ahrs00

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/datamodels"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/gpsa"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/logtype"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/xsens"
)

const (
	// FileType names the file type in stream EDIDs.
	FileType = "AHRS00"
	// LogType is the one log type AHRS00.bin files carry, and the
	// Log_Source value of their rows.
	LogType = "AHRS00"
)

// Stats summarises a parse.
type Stats struct {
	Packets        int // valid GPSA packets
	CorruptPackets int // GPSA packets dropped (checksum/framing)
	Messages       int // MTData2 messages decoded
	BadMessages    int // XSENS candidates dropped by checksum
}

// wrap360 maps an angle in degrees onto [0, 360).
func wrap360(deg float64) float64 {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}
	return deg
}

// magneticHeading converts magnetometer x/y to a heading, using the formula
// the GPSA system uses for its AHRS observation (Sonardyne README:
// Excel DEGREES(ATAN2(magX, -magY)), i.e. atan2(-magY, magX)).
func magneticHeading(x, y float64) float64 {
	return wrap360(math.Atan2(-y, x) * 180 / math.Pi)
}

type packetStart struct {
	offset int64 // stream offset of the packet's first data byte
	pkt    gpsa.Packet
}

// Parse reads an AHRS00.bin stream. Each XSENS message takes the time of
// the GPSA packet holding its first byte.
func Parse(r io.Reader) ([]datamodels.AttitudeAHRSRow, Stats, error) {
	var (
		out     []datamodels.AttitudeAHRSRow
		stats   Stats
		dec     xsens.Decoder
		starts  []packetStart // packets whose bytes are still buffered
		written int64
	)
	g := gpsa.NewReader(r)
	emit := func() {
		for {
			m, off, ok := dec.Next()
			if !ok {
				return
			}
			for len(starts) > 1 && starts[1].offset <= off {
				starts = starts[1:]
			}
			out = append(out, toAttitude(starts[0].pkt, m))
		}
	}
	for {
		pkt, err := g.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, stats, err
		}
		stats.Packets++
		starts = append(starts, packetStart{written, pkt})
		dec.Write(pkt.Data)
		written += int64(len(pkt.Data))
		emit()
	}
	stats.CorruptPackets = g.Corrupt
	stats.Messages = len(out)
	stats.BadMessages = dec.Bad
	return out, stats, nil
}

// toAttitude is the attitude_ahrs row of XSENS message m, timed by its GPSA
// packet p. Measurements the message did not carry are nil.
func toAttitude(p gpsa.Packet, m xsens.MTData2) datamodels.AttitudeAHRSRow {
	a := datamodels.AttitudeAHRSRow{
		Time:           time.UnixMicro(int64(p.CommonTime)).UTC(),
		TimeInstrument: float64(p.InstrumentTime) / 1e6,
	}
	if m.Euler.Valid {
		a.R, a.P, a.H = ptr(m.Euler.X), ptr(m.Euler.Y), ptr(wrap360(m.Euler.Z))
	}
	if m.Acceleration.Valid {
		a.Acx, a.Acy, a.Acz = ptr(m.Acceleration.X), ptr(m.Acceleration.Y), ptr(m.Acceleration.Z)
	}
	if m.MagneticField.Valid {
		a.HMag = ptr(magneticHeading(m.MagneticField.X, m.MagneticField.Y))
	}
	return a
}

func ptr(v float64) *float64 { return &v }

// AttitudeAHRS parses the logType (LogType) logs of the AHRS00.bin file at
// path into attitude_ahrs rows (see datamodels.ParsedSchema). The caller
// releases the table.
func AttitudeAHRS(path, logType string) (arrow.Table, error) {
	if err := logtype.Check(FileType, logType, LogType); err != nil {
		return nil, err
	}
	rs, stats, err := readFile(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed AHRS00", "file", filepath.Base(path), "rows", len(rs),
		"packets", stats.Packets, "corrupt_packets", stats.CorruptPackets, "bad_messages", stats.BadMessages)
	return datamodels.BuildAttitudeAHRS(rs, LogType)
}

func readFile(path string) ([]datamodels.AttitudeAHRSRow, Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, Stats{}, err
	}
	defer f.Close()
	rs, stats, err := Parse(f)
	if err != nil {
		return nil, stats, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rs, stats, nil
}
