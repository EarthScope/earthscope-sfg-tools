package datamodels

import "testing"

func TestStreamEDID(t *testing.T) {
	scope := Scope{Network: "cascadia-gorda", Station: "SLT1", Campaign: "2026_A_1126"}
	got := StreamEDID(scope, "NOV770", "RANGECMP5B_1")
	want := "cascadia-gorda:SLT1:2026_A_1126:NOV770:RANGECMP5B_1"
	if got != want {
		t.Errorf("StreamEDID = %q, want %q", got, want)
	}
}

func TestLogSource(t *testing.T) {
	for _, c := range []struct {
		logType string
		ant     uint8
		want    string
	}{
		{"RANGECMP5B", 0, "RANGECMP5B"},
		{"RANGECMP5B", 1, "RANGECMP5B_1"},
		{"RANGECMPB", 1, "RANGECMPB_1"},
		{"INSPVAA", 0, "INSPVAA"},
	} {
		if got := LogSource(c.logType, c.ant); got != c.want {
			t.Errorf("LogSource(%q, %d) = %q, want %q", c.logType, c.ant, got, c.want)
		}
	}
}
