package sfg_utils

import (
	"io"
	"strings"
	"testing"

	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
)

func TestDecimalReceiverStatus(t *testing.T) {
	cases := map[string]string{
		"#INSPVAA,COM1,0,73.5,FINESTEERING,2380,250000.000,024c0000,18bc,16809;2380,250000.0,1,2*abcd1234\r\n": "#INSPVAA,COM1,0,73.5,FINESTEERING,2380,250000.000,38535168,18bc,16809;2380,250000.0,1,2*abcd1234\r\n",
		"junk#RANGEA,COM1,0,73.5,FINESTEERING,2380,250000.000,00000020,18bc,16809;1*ff\n":                      "junk#RANGEA,COM1,0,73.5,FINESTEERING,2380,250000.000,32,18bc,16809;1*ff\n",
		"%RAWIMUSA,2380,250000.000;1,2*ff\n": "%RAWIMUSA,2380,250000.000;1,2*ff\n",
		"no message here\n":                  "no message here\n",
	}
	for in, want := range cases {
		if got := decimalReceiverStatus(in); got != want {
			t.Errorf("decimalReceiverStatus(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestReceiverStatusReaderRewritesEveryLine(t *testing.T) {
	in := "#A,P,0,1,FINE,1,2.0,0000000a,0,1;x*0\n#B,P,0,1,FINE,1,2.0,0000000b,0,1;y*0"
	out, err := io.ReadAll(newReceiverStatusReader(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	want := "#A,P,0,1,FINE,1,2.0,10,0,1;x*0\n#B,P,0,1,FINE,1,2.0,11,0,1;y*0"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestNextMessageSurvivesCorruptLine(t *testing.T) {
	in := "#BAD,COM1,0,1*00\n" +
		"#INSPVAA,COM1,0,73.5,FINESTEERING,2380,1.0,00000000,18bc,16809;1,2*ff\n"
	scanner := novatelascii.NewScanner(strings.NewReader(in))
	if _, err := nextMessage(scanner); err == nil {
		t.Fatal("want an error for the line with no ';'")
	}
	msg, err := nextMessage(scanner)
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := msg.(novatelascii.LongMessage); !ok || m.Msg != "INSPVAA" {
		t.Errorf("got %#v, want the INSPVAA message after the corrupt line", msg)
	}
}

func TestScanFileNOV000ReadsMessagesWithHexReceiverStatus(t *testing.T) {
	// The GPSA stream of this fixture de-frames to 49 RANGEA, 195 INSPVAA and
	// 193 INSSTDEVA logs, nearly all with receiver status 024c0000.
	epochs, pva, std, _ := ScanFileNOV000(nov000Fixture, false)
	if len(epochs) != 49 || len(pva) != 195 || len(std) != 193 {
		t.Errorf("got %d epochs, %d INSPVAA, %d INSSTDEVA; want 49, 195, 193",
			len(epochs), len(pva), len(std))
	}
}

func TestScanFileNOV000SkipsUnknownTime(t *testing.T) {
	// Every log in the fixture is from before the receiver's time fix.
	epochs, pva, std, _ := ScanFileNOV000(nov000Fixture, true)
	if len(epochs)+len(pva)+len(std) != 0 {
		t.Errorf("got %d epochs, %d INSPVAA, %d INSSTDEVA; want none",
			len(epochs), len(pva), len(std))
	}
}

const nov000Fixture = "../../../../tests/data/test_gcc1_20250821_205746_00062_NOV000.bin"
