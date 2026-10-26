package novatel770

import "testing"

func TestUnknownLogType(t *testing.T) {
	if _, err := GNSSObs("unused", "RANGEA"); err == nil {
		t.Fatal("want an error: NOV770 files have no RANGEA logs")
	}
}
