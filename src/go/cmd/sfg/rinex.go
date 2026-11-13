package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/rinexout"
)

var rinexCmd = &cobra.Command{
	Use:   "rinex [files...]",
	Short: "Convert raw NovAtel files to daily RINEX observation files",
	Long: "Decodes the GNSS observations of raw files of one --type (NOV000, NOV770, NVA or QCPIN) " +
		"and writes one RINEX observation file per day to --out, named by RINEX 3 convention. " +
		"Epochs from all files are merged, ordered and de-duplicated by time, cycle slips are flagged from the receiver lock times, then epochs are optionally decimated (slips on dropped epochs carry to the next kept one). " +
		"Logs with no time fix (status UNKNOWN or GPS week 0) are dropped. " +
		"A file type with several log sources (NOV770: RANGECMPB, RANGECMP5B, and _1 for the second antenna) needs --log-source.",
	Args: cobra.MinimumNArgs(1),
	RunE: runRINEX,
}

func init() {
	f := rinexCmd.Flags()
	f.String("type", "", "raw file type: NOV000, NOV770, NVA or QCPIN (required)")
	f.String("log-source", "", "log source to write, e.g. RANGEA, RANGECMP5B, RANGECMP5B_1 (default: the only one found)")
	f.String("settings", "", "rinex.Settings JSON file for the header (required)")
	f.Int64("modulo", 0, "decimation interval in milliseconds, e.g. 1000 for 1 Hz, 15000 for 15 s; 0 keeps every epoch")
	f.String("out", ".", "output directory")
	rootCmd.AddCommand(rinexCmd)
}

func runRINEX(cmd *cobra.Command, files []string) error {
	f := cmd.Flags()
	fileType, _ := f.GetString("type")
	logSource, _ := f.GetString("log-source")
	settingsPath, _ := f.GetString("settings")
	modulo, _ := f.GetInt64("modulo")
	out, _ := f.GetString("out")
	if fileType == "" || settingsPath == "" {
		return &convert.UsageError{Err: fmt.Errorf("--type and --settings are required")}
	}
	if modulo < 0 {
		return &convert.UsageError{Err: fmt.Errorf("--modulo %d: want 0 or more", modulo)}
	}
	settings, err := readRINEXSettings(settingsPath)
	if err != nil {
		return err
	}

	bySource := map[string][]observation.Epoch{}
	for _, path := range files {
		got, err := fileparse.ReadGNSSEpochs(path, fileType)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for src, eps := range got {
			if logSource != "" && src != logSource {
				// Count it so a wrong --log-source can list what was found,
				// but keep only the wanted source's epochs in memory.
				if _, ok := bySource[src]; !ok {
					bySource[src] = nil
				}
				continue
			}
			bySource[src] = append(bySource[src], eps...)
		}
		slog.Info("read file", "file", path, "epochs", counts(got))
	}
	src, err := pickLogSource(bySource, logSource)
	if err != nil {
		return err
	}

	epochs := rinexout.Prepare(bySource[src], modulo)
	if len(epochs) == 0 {
		return fmt.Errorf("no %s epochs to write", src)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, day := range rinexout.ByDay(epochs) {
		path, err := rinexout.WriteFile(out, settings, day)
		if err != nil {
			return err
		}
		slog.Info("wrote RINEX", "file", path, "log_source", src, "epochs", len(day))
	}
	return nil
}

// pickLogSource is want, which must be among bySource's log sources, or when
// want is "" the only log source with epochs.
func pickLogSource(bySource map[string][]observation.Epoch, want string) (string, error) {
	var found []string
	for _, src := range slices.Sorted(maps.Keys(bySource)) {
		if len(bySource[src]) > 0 {
			found = append(found, src)
		}
	}
	switch {
	case want != "":
		if !slices.Contains(found, want) {
			return "", fmt.Errorf("--log-source %s: no epochs; found %s", want, sourceList(found))
		}
		return want, nil
	case len(found) == 1:
		return found[0], nil
	case len(found) == 0:
		return "", fmt.Errorf("no GNSS epochs in the files")
	default:
		return "", &convert.UsageError{Err: fmt.Errorf("the files hold several log sources (%s): choose one with --log-source", sourceList(found))}
	}
}

func sourceList(srcs []string) string {
	if len(srcs) == 0 {
		return "none"
	}
	return strings.Join(srcs, ", ")
}

// counts is the number of epochs per log source, for logging.
func counts(bySource map[string][]observation.Epoch) map[string]int {
	out := make(map[string]int, len(bySource))
	for src, eps := range bySource {
		out[src] = len(eps)
	}
	return out
}

// readRINEXSettings reads a rinex.Settings JSON file over the defaults.
func readRINEXSettings(path string) (*rinex.Settings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	settings := rinex.NewSettings()
	if err := json.Unmarshal(b, settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return settings, nil
}
