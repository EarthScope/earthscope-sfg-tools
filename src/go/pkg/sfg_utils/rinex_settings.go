package sfg_utils

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"gitlab.com/earthscope/gnsstools/codecs/rinex"
)

// NormalizeAntennaModel formats the RINEX A20 type field: 16 model columns
// followed by four radome columns. Legacy JSON commonly uses "NOV850 NONE".
func NormalizeAntennaModel(value string) (string, error) {
	fields := strings.Fields(value)
	if len(value) == 20 && len(fields) == 1 {
		fields = []string{value[:16], value[16:]}
	}
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) > 2 {
		return "", fmt.Errorf("antenna_model must contain a model and optional radome")
	}
	model, radome := fields[0], "NONE"
	if len(fields) == 2 {
		radome = fields[1]
	}
	for _, c := range model {
		if c > unicode.MaxASCII {
			return "", fmt.Errorf("antenna model must be ASCII")
		}
	}
	if len(model) > 16 {
		return "", fmt.Errorf("antenna model exceeds 16 characters")
	}
	if len(radome) != 4 {
		return "", fmt.Errorf("antenna radome must be four ASCII alphanumeric characters")
	}
	for _, c := range radome {
		if c > unicode.MaxASCII || !(unicode.IsLetter(c) || unicode.IsDigit(c)) {
			return "", fmt.Errorf("antenna radome must be four ASCII alphanumeric characters")
		}
	}
	return fmt.Sprintf("%-16s%s", model, radome), nil
}

// ReadRinexSettings validates metadata for every RINEX conversion command,
// including CLI JSON inputs which bypass Python's RinexMetadata validation.
func ReadRinexSettings(path string) (*rinex.Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	settings := rinex.NewSettings()
	if err := json.Unmarshal(data, settings); err != nil {
		return nil, fmt.Errorf("parsing settings JSON: %w", err)
	}
	settings.AntennaModel, err = NormalizeAntennaModel(settings.AntennaModel)
	if err != nil {
		return nil, fmt.Errorf("invalid antenna metadata: %w", err)
	}
	return settings, nil
}
