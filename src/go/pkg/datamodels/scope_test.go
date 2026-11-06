package datamodels

import "testing"

func TestStreamEDID(t *testing.T) {
	scope := Scope{Network: "cascadia-gorda", Station: "SLT1", Campaign: "2026_A_1126"}
	got := StreamEDID(scope, "NOV770", "RANGECMP5B_1")
	want := "cascadia-gorda.SLT1.2026_A_1126.NOV770.RANGECMP5B_1"
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

func TestSplitEDID(t *testing.T) {
	scope := Scope{Network: "net", Station: "STA", Campaign: "2026_A_1126"}
	f, ok := SplitEDID(DerivedEDID(scope, ProductPRIDE, "NOV000:RANGEA"))
	if !ok || f[3] != ProductPRIDE || f[4] != "NOV000.RANGEA" {
		t.Errorf("derived EDID fields %q, %v; want the input whole in the last", f, ok)
	}
	if _, ok := SplitEDID("net.STA.2026_A_1126.NOV000"); ok {
		t.Error("four fields: ok")
	}
}

func TestCheckScope(t *testing.T) {
	if err := CheckScope(Scope{Network: "cascadia-gorda", Station: "SLT1", Campaign: "2026_A_1126"}); err != nil {
		t.Error(err)
	}
	if err := CheckScope(Scope{Network: "net", Station: "ST.A", Campaign: "C"}); err == nil {
		t.Error("station with the separator: no error")
	}
}
