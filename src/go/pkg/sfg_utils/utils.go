package sfg_utils

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	tiledb "github.com/TileDB-Inc/TileDB-Go"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	rinex "gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

// BuildV3ObsFilename constructs a RINEX v3/v4 long-format observation
// filename: SSSSMRCCC_S_YYYYDDDHHMM_PPP_FFF_DT.rnx
//
// Monument number, receiver number, and country code aren't tracked per
// station today, so they're fixed at "00"/"USA"; the data-source character
// is always "R" (receiver-generated) since this pipeline never produces
// RINEX from a real-time stream. epochs must be sorted by time and used to
// both anchor the file's start timestamp and estimate the sampling
// interval, so decimated output is labeled correctly.
func BuildV3ObsFilename(settings *rinex.Settings, epochs []observation.Epoch) string {
	t := epochs[0].Time
	doy := t.YearDay()
	station := fmt.Sprintf("%s00USA", settings.MarkerName)
	sysChar := "M"
	if settings.RinexSystem != "" {
		sysChar = string(settings.RinexSystem[0])
	}
	freq := estimateDataFrequency(epochs)
	return fmt.Sprintf("%s_R_%04d%03d%02d%02d_01D_%s_%sO.rnx",
		station, t.Year(), doy, t.Hour(), t.Minute(), freq, sysChar)
}

// estimateDataFrequency returns the RINEX 3-character sampling-interval
// code (e.g. "01S", "30S", "15M") derived from the median gap between
// consecutive epochs. Returns "00U" (unspecified) when fewer than two
// epochs are available.
func estimateDataFrequency(epochs []observation.Epoch) string {
	if len(epochs) < 2 {
		return "00U"
	}
	n := len(epochs) - 1
	if n > 200 {
		n = 200
	}
	deltas := make([]float64, n)
	for i := 0; i < n; i++ {
		deltas[i] = epochs[i+1].Time.Sub(epochs[i].Time).Seconds()
	}
	sort.Float64s(deltas)
	median := deltas[len(deltas)/2]
	if median <= 0 {
		return "00U"
	}
	switch {
	case median < 1:
		return fmt.Sprintf("%02dC", int(math.Round(median*100)))
	case median < 60:
		return fmt.Sprintf("%02dS", int(math.Round(median)))
	case median < 3600:
		return fmt.Sprintf("%02dM", int(math.Round(median/60)))
	case median < 86400:
		return fmt.Sprintf("%02dH", int(math.Round(median/3600)))
	default:
		return fmt.Sprintf("%02dD", int(math.Round(median/86400)))
	}
}

func ArrayExists(arrayPath string) bool {
	ctx, err := tiledb.NewContext(nil)
	if err != nil {
		log.Errorf("failed creating TileDB co	ntext: %v", err)
		return false
	}
	defer ctx.Free()

	schema, err := tiledb.LoadArraySchema(ctx, arrayPath)
	if err != nil {
		log.Errorf("failed to load TileDB array schema: %v", err)
	}
	if schema == nil {
		return false
	} else {
		return true
	}
}

func LoadEnv() {
	// Get the file path of the current source file
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatalf("Unable to get the current file path")
	}
	// src/golangtools/cmd/tdb2rnx/main.go

	// src/.env
	// Get the directory of the current file
	dir := filepath.Dir(currentFile)
	for i := 0; i < 3; i++ {
		// Move up three directories
		dir = filepath.Dir(dir)
	}
	// Construct the path to the .env file
	envFilePath := filepath.Join(dir, ".env")

	// Load the .env file
	log.Infof("Loading .env file from %s", envFilePath)
	err := godotenv.Load(envFilePath)
	if err != nil {
		log.Warn("Error loading .env file", err)
	}
}

// src/golangtools/cmd/tdb2rnx/main.go
// src/.env

func SortEpochsByTime(epochs []observation.Epoch) {
	sort.Slice(epochs, func(i, j int) bool {
		return epochs[i].Time.Before(epochs[j].Time)
	})
}

func BatchEpochsByDay(epochs []observation.Epoch) (map[string][]observation.Epoch, error) {

	if len(epochs) == 0 {
		return nil, fmt.Errorf("no epochs to batch")
	}
	batchedEpochs := make(map[string][]observation.Epoch)
	// first, sort epochs by time
	SortEpochsByTime(epochs)
	for _, epoch := range epochs {
		dayKey := GetYMDKey(epoch.Time)
		batchedEpochs[dayKey] = append(batchedEpochs[dayKey], epoch)
	}
	for dayKey := range batchedEpochs {
		log.Infof("Batched %d epochs for day %s", len(batchedEpochs[dayKey]), dayKey)
	}
	return batchedEpochs, nil
}

func WriteEpochs(epochs []observation.Epoch, settings *rinex.Settings) error {
	SortEpochsByTime(epochs)
	startYear, startMonth, startDay := epochs[0].Time.Date()
	filename := BuildV3ObsFilename(settings, epochs)
	log.Infof("Generating Daily RINEX File For Year %d, Month %d, Day %d To %s", startYear, startMonth, startDay, filename)

	// Check if the file already exists
	if _, err := os.Stat(filename); err == nil {
		log.Warnf("File Already Exists: %s", filename)
		// delete the file
		err := os.Remove(filename)
		if err != nil {
			log.Errorf("failed deleting existing file: %s", err)
		}
	}
	outFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Errorf("Error opening output file: %s", err)
		os.Exit(1)
	}

	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	obsWriter := rinex.NewObsWriter(writer, settings)
	defer obsWriter.Flush()


	for _, e := range epochs {
		_, err := obsWriter.Write(e)
		if err != nil {
			slog.Error("Error writing observation", "error", err)
			return err
		}
	}
	return nil
}

// Reader parses the NovAtel ASCII logs carried inside a GPSA binary
// (NOV000) stream. Internally it flattens the underlying DLE-framed
// packets into one continuous ASCII byte stream (see gpsaASCIIReader) and
// hands that to novatelascii.Scanner, since GPSA packet boundaries do not
// align with log boundaries — a large log can span several packets and
// several small logs can be interleaved between the fragments of a large
// one.
type Reader struct {
	scanner novatelascii.Scanner
}

func NewReader(r io.Reader) Reader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return Reader{scanner: novatelascii.NewScanner(newGPSAASCIIReader(br))}
}

// nextMessageNOV00bin returns the next NovAtel ASCII log from the GPSA
// binary stream.
func (reader *Reader) nextMessageNOV00bin() (message novatelascii.Message, err error) {
	return reader.scanner.NextMessage()
}



// DecimateEpochs decimates a slice of epochs to keep only those that fall on a modulo boundary.
// It also propagates Loss-of-Lock Indicators (LLI) from skipped epochs to the next written epoch.
//
// Parameters:
//   - epochs: The slice of epochs to decimate (must be sorted by time)
//   - moduloMillis: The modulo interval in milliseconds (e.g., 1000 for 1 Hz, 15000 for 15-second intervals)
//
// Returns:
//   - A decimated slice of epochs with LLI bits propagated from skipped epochs
//
// If moduloMillis is 0 or negative, the original epochs are returned unchanged.
func DecimateEpochs(epochs []observation.Epoch, moduloMillis int64) []observation.Epoch {
	if moduloMillis <= 0 {
		return epochs
	}
	if len(epochs) == 0 {
		return epochs
	}

	// Ensure epochs are sorted by time
	SortEpochsByTime(epochs)

	decimatedEpochs := []observation.Epoch{}

	// Track accumulated LLI bits for each satellite/observation pair
	// Key: SatelliteKey string + ObservationKey string
	accumulatedLLI := make(map[string]observation.ObservationFlags)

	for _, epoch := range epochs {
		epochMillis := epoch.Time.UnixMilli()

		// Check if this epoch falls on the modulo boundary
		if epochMillis%moduloMillis == 0 {
			// Apply accumulated LLI bits to this epoch's observations
			for _, sat := range epoch.Satellites {
				for i := range sat.Observations {
					lliKey := fmt.Sprintf("%s_%s", sat.SatelliteKey, sat.Observations[i].Code)
					if accFlags, exists := accumulatedLLI[lliKey]; exists {
						// OR the accumulated flags with this observation's flags
						sat.Observations[i].Flags |= accFlags
					}
				}
			}

			decimatedEpochs = append(decimatedEpochs, epoch)

			// Clear accumulated LLI after writing an epoch
			accumulatedLLI = make(map[string]observation.ObservationFlags)
		} else {
			// This epoch is being skipped - accumulate any LLI bits
			for _, sat := range epoch.Satellites {
				for _, obs := range sat.Observations {
					if obs.Flags&observation.FlagLossOfLock != 0 {
						lliKey := fmt.Sprintf("%s_%s", sat.SatelliteKey, obs.Code)
						accumulatedLLI[lliKey] |= obs.Flags
					}
				}
			}
		}
	}

	slog.Info("Decimated epochs", "original", len(epochs), "decimated", len(decimatedEpochs), "modulo_ms", moduloMillis)
	return decimatedEpochs
}

func GetYMDKey(t time.Time) string {
	year, month, day := t.Date()
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}