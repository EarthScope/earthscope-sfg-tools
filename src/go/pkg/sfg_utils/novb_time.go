package sfg_utils

import novatelbinary "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_binary"

// usableNOVBObservationTime excludes receiver startup timestamps from both
// source-file ordering and observation ingestion. UNKNOWN/APPROXIMATE and
// COARSEADJUSTING are not established reference times. SATTIME is for satellite
// logs, not receiver observations. Keep coarse through fine/freewheeling states.
// https://docs.novatel.com/OEM7/Content/Messages/GPS_Reference_Time_Statu.htm
func usableNOVBObservationTime(msg novatelbinary.Message) bool {
	if msg.Week == 0 || msg.GPSMilliSecondsOfWeek >= 604800000 {
		return false
	}
	switch msg.TimeStatus {
	case 100, 120, 130, 140, 160, 170, 180:
		return true
	default:
		return false
	}
}
