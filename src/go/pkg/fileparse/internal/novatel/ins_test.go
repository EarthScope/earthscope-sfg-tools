package novatel

import (
	"testing"
	"time"
)

func TestParseINS(t *testing.T) {
	gps := time.Date(2023, 6, 21, 17, 11, 1, 50_000_000, time.UTC)
	pva, err := ParseINSPVA("2267,580261.050000000,45.30245563418,-124.96561111107,-28.6138,-0.2412,0.6377,0.2949,2.627875295,0.299460630,70.416827684,INS_SOLUTION_GOOD", gps)
	if err != nil {
		t.Fatal(err)
	}
	if pva.Lat != 45.30245563418 || pva.Azimuth != 70.416827684 ||
		pva.INSStatus == nil || *pva.INSStatus != "INS_SOLUTION_GOOD" {
		t.Errorf("INSPVA = %+v", pva)
	}
	std, err := ParseINSSTDEV("1.9247,1.7252,2.2174,0.0550,0.0479,0.0502,0.0325,0.0350,0.3994,2e000041,0,0,0,0", gps)
	if err != nil {
		t.Fatal(err)
	}
	if std.AzimuthSig != 0.3994 || std.ExtSolStat == nil || *std.ExtSolStat != "2e000041" ||
		std.TimeSinceUpdate == nil || *std.TimeSinceUpdate != "0" {
		t.Errorf("INSSTDEV = %+v", std)
	}
	if _, err := ParseINSPVA("2267,580261.05,bad", gps); err == nil {
		t.Error("want an error for a short INSPVAA")
	}
}

func TestDecimalReceiverStatus(t *testing.T) {
	in := "#RANGEA,USB2,0,51.5,FINESTEERING,2421,274978.000,024c0000,5103,18017;16,3*1df4624d\r\n"
	want := "#RANGEA,USB2,0,51.5,FINESTEERING,2421,274978.000,38535168,5103,18017;16,3*1df4624d\r\n"
	if got := decimalReceiverStatus(in); got != want {
		t.Errorf("got %q", got)
	}
}
