package sfg_utils

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/labstack/gommon/log"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	novatelbinary "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_binary"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

type InspvaaRecord struct {
	time              time.Time
	GNSSWeek          int
	GNSSSecondsofWeek float64
	latitude          float64
	longitude         float64
	height            float64
	northVelocity     float64
	eastVelocity      float64
	upVelocity        float64
	roll              float64
	pitch             float64
	azimuth           float64
	// status string
}

type INSSTDEVARecord struct {
	time              time.Time
	latitude_std      float64
	longitude_std     float64
	height_std        float64
	northVelocity_std float64
	eastVelocity_std  float64
	upVelocity_std    float64
	roll_std          float64
	pitch_std         float64
	azimuth_std       float64
}

type INSCompleteRecord struct {
	time              time.Time
	GNSSWeek          int
	GNSSSecondsofWeek float64
	latitude          float64
	longitude         float64
	height            float64
	northVelocity     float64
	eastVelocity      float64
	upVelocity        float64
	roll              float64
	pitch             float64
	azimuth           float64
	latitude_std      float64
	longitude_std     float64
	height_std        float64
	northVelocity_std float64
	eastVelocity_std  float64
	upVelocity_std    float64
	roll_std          float64
	pitch_std         float64
	azimuth_std       float64
	// status string
}

func MergeINSRecordsFlat(insPvaa InspvaaRecord, insStdDev INSSTDEVARecord) INSCompleteRecord {
	return INSCompleteRecord{
		time:              insPvaa.time,
		GNSSWeek:          insPvaa.GNSSWeek,
		GNSSSecondsofWeek: insPvaa.GNSSSecondsofWeek,
		latitude:          insPvaa.latitude,
		longitude:         insPvaa.longitude,
		height:            insPvaa.height,
		northVelocity:     insPvaa.northVelocity,
		eastVelocity:      insPvaa.eastVelocity,
		upVelocity:        insPvaa.upVelocity,
		roll:              insPvaa.roll,
		pitch:             insPvaa.pitch,
		azimuth:           insPvaa.azimuth,
		latitude_std:      insStdDev.latitude_std,
		longitude_std:     insStdDev.longitude_std,
		height_std:        insStdDev.height_std,
		northVelocity_std: insStdDev.northVelocity_std,
		eastVelocity_std:  insStdDev.eastVelocity_std,
		upVelocity_std:    insStdDev.upVelocity_std,
		roll_std:          insStdDev.roll_std,
		pitch_std:         insStdDev.pitch_std,
		azimuth_std:       insStdDev.azimuth_std,
		// status:                  insPvaa.status,
	}
}

func DeserializeINSPVAARecord(data string, time time.Time) (InspvaaRecord, error) {
	// 2267,580261.050000000,45.30245563418,-124.96561111107,-28.6138,-0.2412,0.6377,0.2949,2.627875295,0.299460630,70.416827684,INS_SOLUTION_GOOD
	record := InspvaaRecord{}
	record.time = time
	parts := strings.Split(data, ",")
	if len(parts) < 12 {
		return InspvaaRecord{}, fmt.Errorf("invalid INSPVAA record: %s", data)
	}
	week, err := strconv.Atoi(parts[0])
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.GNSSWeek = week
	seconds, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.GNSSSecondsofWeek = seconds // seconds since the start of the week

	latitude, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.latitude = latitude

	longitude, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.longitude = longitude

	height, err := strconv.ParseFloat(parts[4], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.height = height

	northVelocity, err := strconv.ParseFloat(parts[5], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.northVelocity = northVelocity

	eastVelocity, err := strconv.ParseFloat(parts[6], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.eastVelocity = eastVelocity

	upVelocity, err := strconv.ParseFloat(parts[7], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.upVelocity = upVelocity

	roll, err := strconv.ParseFloat(parts[8], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.roll = roll

	pitch, err := strconv.ParseFloat(parts[9], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.pitch = pitch

	azimuth, err := strconv.ParseFloat(parts[10], 64)
	if err != nil {
		return InspvaaRecord{}, fmt.Errorf("error deserializing INSPVAA (%s)", err)
	}
	record.azimuth = azimuth

	// status := strings.Join(parts[11:], ",")
	// record.status = status

	return record, nil

}

func DeserializeINSSTDEVARecord(data string, time time.Time) (INSSTDEVARecord, error) {
	record := INSSTDEVARecord{}
	record.time = time
	parts := strings.Split(data, ",")
	if len(parts) < 9 {
		return INSSTDEVARecord{}, fmt.Errorf("invalid INSSTDEVA record: %s", data)
	}

	latitude_std, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.latitude_std = latitude_std

	longitude_std, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.longitude_std = longitude_std

	height_std, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.height_std = height_std

	northVelocity_std, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.northVelocity_std = northVelocity_std

	eastVelocity_std, err := strconv.ParseFloat(parts[4], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.eastVelocity_std = eastVelocity_std

	upVelocity_std, err := strconv.ParseFloat(parts[5], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.upVelocity_std = upVelocity_std

	roll_std, err := strconv.ParseFloat(parts[6], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.roll_std = roll_std

	pitch_std, err := strconv.ParseFloat(parts[7], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.pitch_std = pitch_std

	azimuth_std, err := strconv.ParseFloat(parts[8], 64)
	if err != nil {
		return INSSTDEVARecord{}, fmt.Errorf("error deserializing INSSTDEVA (%s)", err)
	}
	record.azimuth_std = azimuth_std

	return record, nil
}

func MergeINSPVAAAndINSSTDEVA(INSPVAARecords []InspvaaRecord, INSSTDEVRecords []INSSTDEVARecord) []INSCompleteRecord {
	// sort the slices by time
	sort.Slice(INSPVAARecords, func(i, j int) bool {
		return INSPVAARecords[i].time.Before(INSPVAARecords[j].time)
	})
	sort.Slice(INSSTDEVRecords, func(i, j int) bool {
		return INSSTDEVRecords[i].time.Before(INSSTDEVRecords[j].time)
	})
	var matchedRecords []INSCompleteRecord
	i := 0
	j := 0
	foundMatch := 0
	var elemB INSSTDEVARecord

	for i < len(INSPVAARecords) {
		inspvaarecord := INSPVAARecords[i]
		if j < len(INSSTDEVRecords) {
			elemB = INSSTDEVRecords[j]
		} else {
			elemB = INSSTDEVARecord{}
		}

		if inspvaarecord.time.Equal(elemB.time) {
			foundMatch++
			merged := MergeINSRecordsFlat(inspvaarecord, elemB)
			matchedRecords = append(matchedRecords, merged)
			i++
			j++
		} else {
			merged := MergeINSRecordsFlat(inspvaarecord, INSSTDEVARecord{})
			matchedRecords = append(matchedRecords, merged)
			i++

		}

	}

	log.Infof("Found %d matching elements between the two lists", foundMatch)
	// Print the matching elements
	return matchedRecords
}
func GetTimeDiffsINSPVA(list []INSCompleteRecord) []float64 {
	var diffs []float64
	minDiff := 100000.0 // 1000 seconds
	for i := 1; i < len(list); i++ {
		difference := list[i].time.Sub(list[i-1].time).Seconds()
		if difference < minDiff {
			minDiff = difference
		}
		if difference < 1 {
			diffs = append(diffs, difference)
		}
	}
	var diffs_average float64
	if len(diffs) > 0 {
		var sum float64
		for _, v := range diffs {
			sum += v
		}
		diffs_average = sum / float64(len(diffs))
	}
	log.Infof("INSPVA Average time difference: %f seconds Minimum time difference: %f seconds", diffs_average, minDiff)
	return diffs
}

func GetTimeDiffGNSS(list []observation.Epoch) []float64 {
	var diffs []float64
	minDiff := 100000.0 // 1000 seconds
	for i := 1; i < len(list); i++ {
		difference := list[i].Time.Sub(list[i-1].Time).Seconds()
		if difference < minDiff {
			minDiff = difference
		}
		if difference < 1 {
			diffs = append(diffs, difference)
		}
	}
	var diffs_average float64
	if len(diffs) > 0 {
		var sum float64
		for _, v := range diffs {
			sum += v
		}
		diffs_average = sum / float64(len(diffs))
	}
	log.Infof("GNSS Average time difference: %f seconds Minimum time difference: %f seconds", diffs_average, minDiff)
	return diffs
}

func removeBeforeASCIISyncChar(s string) (string, error) {
	longMessageIndex := strings.Index(s, "#")
	shortMessageIndex := strings.Index(s, "%")
	switch {
	case longMessageIndex != -1:
		return s[longMessageIndex:], nil
	case shortMessageIndex != -1:
		return s[shortMessageIndex:], nil
	default:
		return "", fmt.Errorf("novatel ASCII sync char not found")
	}
}

func processBuffer(buffer []byte) (message novatelascii.Message, err error) {
	stringArray := string(buffer)
	trimmedLine, err := removeBeforeASCIISyncChar(stringArray)
	if err != nil {
		return message, err
	}
	//fmt.Print("\n Trimmed Line: ", trimmedLine)
	endOfHeaderIndex := strings.Index(trimmedLine, ";")
	endOfDataIndex := strings.Index(trimmedLine, "*")

	if endOfDataIndex <= endOfHeaderIndex {
		return message, fmt.Errorf("message is missing checksum")
		// endOfDataIndex = len(trimmedLine) - 1
	}
	if endOfDataIndex == -1 {
		return message, fmt.Errorf("message is missing checksum")
		// endOfDataIndex = len(trimmedLine) - 1
	}
	if endOfHeaderIndex < 2 {
		return message, fmt.Errorf("message is too short")
	}
	splitHeaderText := strings.Split(trimmedLine[1:endOfHeaderIndex], ",")
	if len(splitHeaderText) < 10 {
		return message, fmt.Errorf("message header is too short")
	}
	switch trimmedLine[0] {
	case '#': // long
		sequence, err := strconv.Atoi(splitHeaderText[2])
		if err != nil {
			return message, err
		}
		idleTime, err := strconv.ParseFloat(splitHeaderText[3], 64)
		if err != nil {
			return message, err
		}
		week, err := strconv.ParseFloat(splitHeaderText[5], 64)
		if err != nil {
			return message, err
		}
		seconds, err := strconv.ParseFloat(splitHeaderText[6], 64)
		if err != nil {
			return message, err
		}
		recStatus, err := strconv.ParseFloat(splitHeaderText[7], 64)
		if err != nil {
			return message, err
		}
		recSWVersion, err := strconv.ParseFloat(splitHeaderText[9], 64)
		if err != nil {
			return message, err
		}
		longMessage := novatelascii.LongMessage{
			Sync:         string(trimmedLine[0]),
			Msg:          splitHeaderText[0],
			Port:         splitHeaderText[1],
			Sequence:     sequence,
			IdleTime:     idleTime,
			TimeStatus:   splitHeaderText[4],
			Week:         week,
			Seconds:      seconds,
			RecStatus:    recStatus,
			Reserved:     splitHeaderText[8],
			RecSWVersion: recSWVersion,
			Data:         trimmedLine[endOfHeaderIndex+1 : endOfDataIndex],
			Checksum:     trimmedLine[endOfDataIndex:],
		}
		return longMessage, nil
	default:
		return novatelascii.LongMessage{}, fmt.Errorf("unknown error")
	}

}

// gpsaHeaderLen is the size, in bytes, of the binary GPSA packet header
// that precedes every ASCII log fragment: 2-byte message id, 8-byte
// instrument time, 8-byte common time.
const gpsaHeaderLen = 18

// readRawGPSAPacket reads one DLE/STX/ETX-framed GPSA packet from r
// (Sonardyne binary framing with DLE byte-stuffing) and returns its
// unstuffed contents: the 18-byte GPSA header followed by the packet's
// data bytes, with the trailing 1-byte XOR checksum stripped.
//
// A single ASCII NovAtel log (e.g. a large multi-signal RANGEA) can be
// split across several consecutive GPSA packets when it exceeds the
// underlying link's packet size; only the first fragment carries the
// '#'/'%' sync char and full 18-byte header semantics, continuation
// fragments carry raw log bytes. Reassembly is the caller's job — this
// function only ever returns one low-level packet's payload.
//
// Corrupt packets (bad CRC, ETX with no matching STX, too short) are
// skipped with a debug log, mirroring the reference GPSABinaryExtractor.
func readRawGPSAPacket(r *bufio.Reader) ([]byte, error) {
	const dle byte = 0x10 // data link escape
	const stx byte = 0x02 // start of text
	const etx byte = 0x03 // end of text

	var gotDLE bool
	var gotSTX bool
	var buffer []byte
	var crc byte

	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		switch {
		case b == dle && !gotDLE:
			gotDLE = true

		case b == stx && gotDLE:
			// DLE + STX: start of packet
			gotDLE = false
			gotSTX = true
			buffer = buffer[:0]
			crc = 0

		case b == etx && gotDLE:
			// DLE + ETX: end of packet
			gotDLE = false
			hadSTX := gotSTX
			gotSTX = false

			if !hadSTX {
				slog.Debug("Found ETX with no STX")
				continue
			}
			if crc != 0 {
				slog.Debug("CRC error in packet", "crc", crc)
				continue
			}
			// header plus at least the trailing checksum byte
			if len(buffer) < gpsaHeaderLen+1 {
				slog.Debug("Packet too short", "len", len(buffer))
				continue
			}

			out := make([]byte, len(buffer)-1)
			copy(out, buffer[:len(buffer)-1]) // strip trailing checksum byte
			return out, nil

		default:
			if gotDLE {
				// A lone DLE should only precede STX, ETX, or a stuffed DLE.
				if b != dle {
					slog.Debug("Found DLE with no stuffing", "byte", b)
				}
				gotDLE = false
			}
			if gotSTX {
				buffer = append(buffer, b)
				crc ^= b
			}
		}
	}
}

// gpsaASCIIReader adapts a stream of DLE-framed GPSA binary packets into a
// continuous io.Reader of the NovAtel ASCII log bytes they carry.
//
// The GPSA transport chunks whatever ASCII log data is queued (RANGEA,
// INSPVAA, INSSTDEVA, ...) into fixed-size packets with no regard for log
// boundaries: one large log (e.g. a multi-signal RANGEA) can span several
// packets, and several short logs can be interleaved between the fragments
// of a large one. Packet boundaries therefore carry no message-framing
// meaning — the only reliable way to recover individual logs is to
// concatenate every packet's data back into one continuous ASCII stream
// and let novatelascii.Scanner split it on '\n', exactly as it already
// does for a plain ASCII NovAtel log file (see ProcessFileNOVASCII).
// The source recording can also have genuine gaps — a log's trailing
// checksum + CRLF simply missing from the stream (dropped at capture
// time, not a framing bug). Left alone that would let the next log's
// sync char get silently absorbed into the truncated log's data field,
// so gpsaASCIIReader forces every sync char onto its own line; the
// truncated log then fails to parse (as it must — its checksum really is
// gone) without dragging the following, otherwise-good log down with it.
type gpsaASCIIReader struct {
	r           *bufio.Reader
	buf         []byte
	lastByte    byte
	haveEmitted bool
}

func newGPSAASCIIReader(r *bufio.Reader) *gpsaASCIIReader {
	return &gpsaASCIIReader{r: r}
}

// appendChunk appends packet data to buf, inserting a '\n' before any
// sync char ('#' or '%') not already at the start of a line.
func (g *gpsaASCIIReader) appendChunk(chunk []byte) {
	for _, b := range chunk {
		if (b == '#' || b == '%') && g.haveEmitted && g.lastByte != '\n' {
			g.buf = append(g.buf, '\n')
		}
		g.buf = append(g.buf, b)
		g.lastByte = b
		g.haveEmitted = true
	}
}

func (g *gpsaASCIIReader) Read(p []byte) (int, error) {
	for len(g.buf) == 0 {
		packet, err := readRawGPSAPacket(g.r)
		if err != nil {
			return 0, err
		}
		if len(packet) > gpsaHeaderLen {
			g.appendChunk(packet[gpsaHeaderLen:])
		}
	}
	n := copy(p, g.buf)
	g.buf = g.buf[n:]
	return n, nil
}

// processFileNOVASCII reads a NOVATEL ASCII file and processes its contents to extract GNSS epochs.
// It takes a filename as input and returns a slice of observation.Epoch.
//
// The function performs the following steps:
// 1. Opens the specified file.
// 2. Creates a new scanner to read NOVATEL ASCII messages from the file.
// 3. Iterates over the messages in the file.
// 4. For each "RANGEA" message, deserializes the message data and converts it to a GNSS epoch.
// 5. Appends the GNSS epoch to the result slice.
//
// If an error occurs while opening the file or reading messages, the function logs the error and terminates the program.
func ProcessFileNOVASCII(filename string) ([]observation.Epoch, int, error) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	epochs := []observation.Epoch{}
	fail_counter := 0
	scanner := novatelascii.NewScanner(bufio.NewReader(file))
epochLoop:
	for {
		msg, err := scanner.NextMessage()
		if err != nil {
			fail_counter++
			if err == io.EOF {
				err = file.Close()
				if err != nil {
					slog.Error("Error closing file", "error", err)
					return epochs, fail_counter, err
				}
				break epochLoop
			}
			slog.Debug("Error reading message", "error", err)

		}
		// log.Debugf("%+v", msg)
		switch m := msg.(type) {
		case novatelascii.LongMessage:
			if m.Msg == "RANGEA" {
				rangea, err := novatelascii.DeserializeRANGEA(m.Data)
				if err != nil {
					slog.Error("Error deserializing RANGEA", "error", err)
					fail_counter++
				}
				// slog.Debug("Message time", "time", m.Time())
				epoch, err := rangea.SerializeGNSSEpoch(m.Time())
				if err != nil {
					slog.Error("Error serializing GNSS epoch", "error", err)
					fail_counter++
				}
				normalizeRangeAEpochTime(&epoch)
				epochs = append(epochs, epoch)
			}
		case novatelascii.ShortMessage:
			if m.Msg == "RANGEA" {
				rangea, err := novatelascii.DeserializeRANGEA(m.Data)
				if err != nil {
					slog.Error("Error deserializing RANGEA", "error", err)
					fail_counter++
				}
				epoch, err := rangea.SerializeGNSSEpoch(m.Time())
				if err != nil {
					slog.Error("Error serializing GNSS epoch", "error", err)
					fail_counter++
				}
				normalizeRangeAEpochTime(&epoch)
				epochs = append(epochs, epoch)
			}
		}
	}
	epochs = RemoveDuplicateEpochs(epochs)
	// RemoveDuplicateEpochs currently collects through a map, so sort after
	// deduplication. Lock-time reset detection requires monotonically ordered
	// epochs.
	sort.Slice(epochs, func(i, j int) bool {
		return epochs[i].Time.Before(epochs[j].Time)
	})
	var lockTracker observation.LockTimeTracker
	for i := range epochs {
		lockTracker.Apply(&epochs[i])
	}
	return epochs, fail_counter, nil
}

// normalizeRangeAEpochTime removes sub-millisecond floating-point artifacts
// introduced while converting the decimal GPS seconds in an ASCII header to a
// time.Time. RANGEA headers report milliseconds, so nearest-millisecond
// rounding preserves the receiver timestamp instead of turning .600 into .599.
func normalizeRangeAEpochTime(epoch *observation.Epoch) {
	epoch.Time = epoch.Time.Round(time.Millisecond)
}

// processFileNOVB processes a NOVB file and returns a slice of observation.Epoch.
// It reads the file, scans for messages, and extracts epochs from messages with ID 140.
// If an error occurs while opening the file, it logs a fatal error.
// If an error occurs while reading a message, it logs a warning and continues.
// If an error occurs while serializing an epoch, it logs an error and continues.
// It skips epochs with no satellites.
//
// Parameters:
//   - file: The path to the NOVB file to be processed.
//
// Returns:
//   - A slice of observation.Epoch containing the extracted epochs.
func ProcessFileNOVB(file string, antIndex uint8) ([]observation.Epoch, int, error) {
	epochs := []observation.Epoch{}
	failCounter, err := StreamFileNOVB(file, antIndex, func(epoch observation.Epoch) error {
		epochs = append(epochs, epoch)
		return nil
	})
	return epochs, failCounter, err
}

// StreamFileNOVB decodes primary- or secondary-antenna RANGE, RANGECMP and
// RANGECMP5 epochs from one NOV770 file and yields them in source order. Lock
// tracking is intentionally left to ContinuousEpochProcessor so it can span
// files and bounded TileDB writes.
func StreamFileNOVB(file string, antIndex uint8, yield func(observation.Epoch) error) (int, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, fmt.Errorf("opening NOV770 file: %w", err)
	}
	defer f.Close()
	if antIndex > 1 {
		return 0, fmt.Errorf("invalid antenna index %d", antIndex)
	}

	reader := bufio.NewReader(f)
	fail_counter := 0
MessageLoop:
	for {
		msg, err := novatelbinary.DeserializeMessage(reader)
		if err != nil {
			fail_counter++
			if err == io.EOF {
				break MessageLoop

			}
			if err == bufio.ErrBufferFull {
				log.Warnf("buffer full: %s", err)
				reader.Reset(f)
			}

			//log.Warnf("failed reading message: %s", err)
			continue MessageLoop
		}
		// Apply antenna filter at the message level for all message types.
		if msg.MeasurementSource() != antIndex {
			continue MessageLoop
		}

		var epoch observation.Epoch
		switch msg.MessageID {
		case 43:
			msg43 := msg.DeserializeMessage43()
			epoch, err = msg43.SerializeGNSSEpoch(msg.Time())
		case 140:
			{
				msg140 := msg.DeserializeMessage140()
				epoch, err = msg140.SerializeGNSSEpoch(msg.Time())
			}
		case 2537:
			{
				msg2537, err := msg.DeserializeMessage2537()
				if err != nil {
					log.Errorf("failed deserializing message 2537: %s", err)
					fail_counter++
					continue MessageLoop
				}
				epoch, err = msg2537.SerializeGNSSEpoch(msg.Time())
			}
		default:
			continue MessageLoop
		}
		if err != nil {
			log.Errorf("failed serializing epoch: %s", err)
			fail_counter++
			continue MessageLoop
		}
		if len(epoch.Satellites) == 0 {
			fail_counter++
			continue MessageLoop
		}
		epoch.AntennaIndex = msg.MeasurementSource()
		if err := yield(epoch); err != nil {
			return fail_counter, err
		}
	}
	return fail_counter, nil
}

// processFileNOV000 processes a NOV000 file containing GNSS and INS messages.
// It reads the file, parses messages such as RANGEA, INSPVAA, and INSSTDEVA,
// and deserializes them into corresponding records. The function merges INSPVAA
// and INSSTDEVA records into complete INS records, computes time differences for
// GNSS and INS epochs, and returns slices of GNSS epochs and merged INS records.
//
// Parameters:
//   - file: The path to the NOV000.bin file to be processed.
//
// Returns:
//   - []observation.Epoch: A slice of GNSS epoch records parsed from the file.
//   - []INSCompleteRecord: A slice of merged INS complete records.
//   - int: The number of failed parsing attempts.
//
// The function logs errors encountered during file reading and message deserialization,
// and logs the number of INSPVAA and INSSTDEVA records found.
func ProcessFileNOV000(file string) ([]observation.Epoch, []INSCompleteRecord, int) {
	epochs := []observation.Epoch{}
	insCompleteRecords, failCounter, err := StreamFileNOV000(file, func(epoch observation.Epoch) error {
		epochs = append(epochs, epoch)
		return nil
	})
	if err != nil {
		slog.Error("Error processing NOV000 file", "file", file, "error", err)
		failCounter++
	}
	GetTimeDiffGNSS(epochs)
	GetTimeDiffsINSPVA(insCompleteRecords)
	return epochs, insCompleteRecords, failCounter
}

// StreamFileNOV000 decodes the embedded ASCII RANGEA stream from one NOV000
// file, normalizes its millisecond timestamps, and yields GNSS epochs in source
// order. INS records are returned for their separate TileDB destination.
func StreamFileNOV000(file string, yield func(observation.Epoch) error) ([]INSCompleteRecord, int, error) {

	f, err := os.Open(file)
	if err != nil {
		return nil, 0, fmt.Errorf("opening NOV000 file: %w", err)
	}
	defer f.Close()
	reader := NewReader(bufio.NewReader(f))
	insEpochs := []InspvaaRecord{}
	insStdDevEpochs := []INSSTDEVARecord{}
	fail_counter := 0

epochLoop:
	for {
		message, err := reader.nextMessageNOV00bin()
		if err != nil {
			if err == io.EOF {
				err = f.Close()
				if err != nil {
					slog.Error("Error closing file", "error", err)
				}
				break epochLoop
			}
			fail_counter++
			slog.Debug("Error reading message", "error", err)
		}

		switch m := message.(type) {
		case novatelascii.LongMessage:

			// Deserialize the message based on its type

			// Check if the message is a GNSS RANGEA message
			if m.Msg == "RANGEA" {
				rangea, err := novatelascii.DeserializeRANGEA(m.Data)
				if err != nil {
					slog.Error("Error deserializing RANGEA", "error", err)
					fail_counter++
					continue epochLoop
				}
				epoch, err := rangea.SerializeGNSSEpoch(m.Time())
				if err != nil {
					slog.Error("Error serializing GNSS epoch", "error", err)
					fail_counter++
					continue epochLoop
				}
				normalizeRangeAEpochTime(&epoch)
				if err := yield(epoch); err != nil {
					return nil, fail_counter, err
				}
				// Check if the message is an INSPVAA message
			} else if m.Msg == "INSPVAA" {
				record, err := DeserializeINSPVAARecord(m.Data, m.Time())
				if err != nil {
					slog.Error("Error deserializing INSPVAA record", "error", err)
					fail_counter++
					continue epochLoop
				}
				insEpochs = append(insEpochs, record)

				// Check if the message is an INSSTDEVA message
			} else if m.Msg == "INSSTDEVA" {
				record, err := DeserializeINSSTDEVARecord(m.Data, m.Time())
				if err != nil {
					slog.Error("Error deserializing INSSTDEVA record", "error", err)
					fail_counter++
					continue epochLoop
				}
				insStdDevEpochs = append(insStdDevEpochs, record)
			}
		}
	}
	slog.Info("Found records", "INSPVAA", len(insEpochs), "INSSTDEVA", len(insStdDevEpochs), "num_fails", fail_counter)
	// Merge INSPVAA and INSSTDEVA records
	insCompleteRecords := MergeINSPVAAAndINSSTDEVA(insEpochs, insStdDevEpochs)
	return insCompleteRecords, fail_counter, nil
}

func GetFirstEpochTimeNOV000(file string) (time.Time, error) {

	f, err := os.Open(file)
	if err != nil {
		log.Fatalf("failed opening file %s, %s ", file, err)
	}
	defer f.Close()
	reader := NewReader(bufio.NewReader(f))

epochLoop:
	for {
		message, err := reader.nextMessageNOV00bin()
		if err != nil {
			if err == io.EOF {
				err = f.Close()
				if err != nil {
					log.Error(err)
				}
				break epochLoop
			}
			log.Print(err)
		}

		switch m := message.(type) {
		case novatelascii.LongMessage:
			if m.Msg == "RANGEA" {
				return m.Time().Round(time.Millisecond), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("no RANGEA message found in file")
}

func GetFirstEpochTimeNOVB(file string) (time.Time, error) {

	f, err := os.Open(file)
	if err != nil {
		log.Fatalf("failed opening file: %s", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
MessageLoop:
	for {
		msg, err := novatelbinary.DeserializeMessage(reader)
		if err != nil {
			if err == io.EOF {
				break MessageLoop

			}
			if err == bufio.ErrBufferFull {
				log.Warnf("buffer full: %s", err)
				reader.Reset(f)
			}
			//log.Warnf("failed reading message: %s", err)
			continue MessageLoop
		}
		if msg.MessageID == 43 || msg.MessageID == 140 || msg.MessageID == 2537 {
			return msg.Time(), nil
		} else {
			continue MessageLoop
		}
	}
	return time.Time{}, fmt.Errorf("no RANGE/RANGECMP/RANGECMP5 message found in file")
}

type FileTime struct {
	Filename string
	Time     time.Time
}

// SortFilesByFirstEpochNOVB sorts a list of NOVB files by their first epoch timestamp.
// It reads the first epoch time from each file and returns the files sorted in chronological order.
// Files that cannot be parsed or have errors reading the first epoch time are skipped with a warning.
// Parameters:
//   - files: A slice of file paths to NOVB files to be sorted
//
// Returns:
//   - A slice of file paths sorted by their first epoch time in ascending order
//   - An error if the operation fails (currently always returns nil)
func SortFilesByFirstEpochNOVB(files []string) ([]FileTime, error) {
	var fileTimes []FileTime
	for _, file := range files {
		t, err := GetFirstEpochTimeNOVB(file)
		if err != nil {
			log.Warnf("error getting first epoch time for file %s: %s", file, err)
			continue
		}
		fileTimes = append(fileTimes, FileTime{Filename: file, Time: t})
	}
	sort.Slice(fileTimes, func(i, j int) bool {
		return fileTimes[i].Time.Before(fileTimes[j].Time)
	})

	return fileTimes, nil
}

// SortFilesByFirstEpochNOV000 sorts a list of NOV000 files by their first epoch timestamp.
// It reads the first epoch time from each file and returns a new slice of filenames
// sorted in chronological order (earliest first).
//
// Files that cannot be read or parsed are skipped with a warning log message
// and will not be included in the returned slice.
//
// Parameters:
//   - files: A slice of file paths to NOV000 files to be sorted.
//
// Returns:
//   - A slice of file paths sorted by their first epoch time in ascending order.
//   - An error (currently always nil, errors are logged as warnings).
func SortFilesByFirstEpochNOV000(files []string) ([]FileTime, error) {

	var fileTimes []FileTime
	for _, file := range files {
		t, err := GetFirstEpochTimeNOV000(file)
		if err != nil {
			log.Warnf("error getting first epoch time for file %s: %s", file, err)
			continue
		}
		fileTimes = append(fileTimes, FileTime{Filename: file, Time: t})
	}
	sort.Slice(fileTimes, func(i, j int) bool {
		return fileTimes[i].Time.Before(fileTimes[j].Time)
	})
	return fileTimes, nil
}

func RemoveDuplicateEpochs(epochs []observation.Epoch) []observation.Epoch {
	seen := make(map[time.Time]observation.Epoch)
	for _, epoch := range epochs {
		if _, ok := seen[epoch.Time]; !ok {
			seen[epoch.Time] = epoch
		}
	}
	var uniqueEpochs []observation.Epoch
	for _, epoch := range seen {
		uniqueEpochs = append(uniqueEpochs, epoch)
	}
	log.Infof("Removed %d duplicate epochs, %d unique epochs remain", len(epochs)-len(uniqueEpochs), len(uniqueEpochs))
	return uniqueEpochs
}
