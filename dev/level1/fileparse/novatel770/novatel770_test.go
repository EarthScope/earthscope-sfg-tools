package novatel770

import "testing"

func TestLogSource(t *testing.T) {
	for _, c := range []struct {
		id   uint16
		ant  uint8
		want string
	}{
		{2537, 0, "RANGECMP5B"},
		{2537, 1, "RANGECMP5B_1"},
		{140, 0, "RANGECMPB"},
		{140, 1, "RANGECMPB_1"},
	} {
		if got := LogSource(c.id, c.ant); got != c.want {
			t.Errorf("LogSource(%d, %d) = %q, want %q", c.id, c.ant, got, c.want)
		}
	}
}
