package sfg_utils

import (
	"gitlab.com/earthscope/gnsstools/codecs/rinex"
	"os"
	"path/filepath"
	"testing"
)

func TestRinexSettingsEmitAntennaColumns(t *testing.T) {
	for _, tc := range []struct{ input, model, radome string }{
		{"NOV850 NONE", "NOV850", "NONE"},
		{"NOV850          NONE", "NOV850", "NONE"},
		{"TRM59800.00 SCIT", "TRM59800.00", "SCIT"},
		{"NOV850", "NOV850", "NONE"},
		{"ABCDEFGHIJKLMNOP NONE", "ABCDEFGHIJKLMNOP", "NONE"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(`{"antenna_serial":"SERIAL", "antenna_model":"`+tc.input+`"}`), 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := ReadRinexSettings(path)
			if err != nil {
				t.Fatal(err)
			}
			again, err := NormalizeAntennaModel(settings.AntennaModel)
			if err != nil || again != settings.AntennaModel {
				t.Fatalf("normalization not idempotent: %q, %v", again, err)
			}
			header, err := rinex.NewObsHeader(settings)
			if err != nil {
				t.Fatal(err)
			}
			record, found := header.FindRecord("ANT # / TYPE")
			if !found {
				t.Fatal("missing antenna record")
			}
			content := record.Content
			if content[:6] != "SERIAL" || content[20:20+len(tc.model)] != tc.model || content[36:40] != tc.radome {
				t.Fatalf("incorrect antenna columns: %q", content)
			}
			if len(content) != 60 {
				t.Fatalf("header content length = %d", len(content))
			}
		})
	}
}

func TestNormalizeAntennaRejectsTruncation(t *testing.T) {
	for _, value := range []string{"ABCDEFGHIJKLMNOPQ", "NOV850 TOOLONG", "NOV850 NO!E", "NOV850 NÖNE", "NOV850 NONE EXTRA"} {
		if _, err := NormalizeAntennaModel(value); err == nil {
			t.Errorf("accepted invalid value %q", value)
		}
	}
}
