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

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/gpsa"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/internal/tablebuild"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse/xsens"
)

// LogSource is the Log_Source value for rows parsed from AHRS00.bin.
const LogSource = "AHRS00"

// Tables are the tables AHRS00.bin files produce.
var Tables = []schemas.TableDef{schemas.AttitudeAHRS}

// Row is one attitude_ahrs row. Measurement fields are NaN when the XSENS
// message did not carry them.
type Row struct {
	Time           int64   // UTC, microseconds since the UNIX epoch
	TimeInstrument float64 // seconds since instrument power-on
	Acx, Acy, Acz  float64 // [m/s^2]
	H              float64 // heading [deg], 0..360
	HMag           float64 // magnetic heading from the magnetometer [deg], 0..360
	P, R           float64 // pitch, roll [deg]
}

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
func Parse(r io.Reader) ([]Row, Stats, error) {
	var (
		rows    []Row
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
			rows = append(rows, toRow(starts[0].pkt, m))
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
	stats.Messages = len(rows)
	stats.BadMessages = dec.Bad
	return rows, stats, nil
}

func toRow(p gpsa.Packet, m xsens.MTData2) Row {
	nan := math.NaN()
	row := Row{
		Time:           int64(p.CommonTime),
		TimeInstrument: float64(p.InstrumentTime) / 1e6,
		Acx:            nan, Acy: nan, Acz: nan,
		H: nan, HMag: nan, P: nan, R: nan,
	}
	if m.Euler.Valid {
		row.R, row.P, row.H = m.Euler.X, m.Euler.Y, wrap360(m.Euler.Z)
	}
	if m.Acceleration.Valid {
		row.Acx, row.Acy, row.Acz = m.Acceleration.X, m.Acceleration.Y, m.Acceleration.Z
	}
	if m.MagneticField.Valid {
		row.HMag = magneticHeading(m.MagneticField.X, m.MagneticField.Y)
	}
	return row
}

// ParseFile parses the AHRS00.bin file at path into def for scope. AHRS00
// files produce schemas.AttitudeAHRS only. The caller releases the table.
func ParseFile(path string, def schemas.TableDef, scope schemas.Scope) (arrow.Table, error) {
	if def.Name != schemas.AttitudeAHRS.Name {
		return nil, tablebuild.Unsupported(LogSource, def, Tables...)
	}
	rows, stats, err := readFile(path)
	if err != nil {
		return nil, err
	}
	slog.Debug("parsed AHRS00", "file", filepath.Base(path), "rows", len(rows),
		"packets", stats.Packets, "corrupt_packets", stats.CorruptPackets, "bad_messages", stats.BadMessages)
	return attitudeTable(rows, scope)
}

func readFile(path string) ([]Row, Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, Stats{}, err
	}
	defer f.Close()
	rows, stats, err := Parse(f)
	if err != nil {
		return nil, stats, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rows, stats, nil
}

// attitudeTable converts rows to an attitude_ahrs table: NaN measurements
// become null; the SV3-only time_start_count/time_status are left null.
func attitudeTable(rows []Row, scope schemas.Scope) (arrow.Table, error) {
	b, err := tablebuild.New(schemas.AttitudeAHRS)
	if err != nil {
		return nil, err
	}
	defer b.Release()
	sc := b.Scope(scope, LogSource)
	tm := tablebuild.Col[*array.TimestampBuilder](b, "time")
	tmInstrument := tablebuild.Col[*array.Float64Builder](b, "time_instrument")
	var measurements []*array.Float64Builder
	for _, name := range []string{"acx", "acy", "acz", "h", "h_mag", "p", "r"} {
		measurements = append(measurements, tablebuild.Col[*array.Float64Builder](b, name))
	}
	for _, r := range rows {
		sc.Append()
		tm.Append(arrow.Timestamp(r.Time))
		tmInstrument.Append(r.TimeInstrument)
		for i, v := range []float64{r.Acx, r.Acy, r.Acz, r.H, r.HMag, r.P, r.R} {
			tablebuild.Float64(measurements[i], v, !math.IsNaN(v))
		}
	}
	return b.Table()
}
