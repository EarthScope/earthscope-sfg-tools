package novatel

import "strconv"

// TimeStatusUnknown is the NovAtel time status UNKNOWN: no time fix.
const TimeStatusUnknown = 20

// timeStatuses are the NovAtel OEM7 GPS reference time statuses
// (https://docs.novatel.com/OEM7/Content/Messages/GPS_Reference_Time_Statu.htm).
var timeStatuses = map[int]string{
	20: "UNKNOWN", 60: "APPROXIMATE", 80: "COARSEADJUSTING", 100: "COARSE",
	120: "COARSESTEERING", 130: "FREEWHEELING", 140: "FINEADJUSTING", 160: "FINE",
	170: "FINEBACKUPSTEERING", 180: "FINESTEERING", 200: "SATTIME",
}

// insStatuses are the NovAtel OEM7 inertial solution statuses
// (https://docs.novatel.com/OEM7/Content/SPAN_Logs/INSATT.htm#InertialSolutionStatus).
var insStatuses = map[int]string{
	0: "INS_INACTIVE", 1: "INS_ALIGNING", 2: "INS_HIGH_VARIANCE", 3: "INS_SOLUTION_GOOD",
	6: "INS_SOLUTION_FREE", 7: "INS_ALIGNMENT_COMPLETE", 8: "DETERMINING_ORIENTATION",
	9: "WAITING_INITIALPOS", 10: "WAITING_AZIMUTH", 11: "INITIALIZING_BIASES",
	12: "MOTION_DETECT", 14: "WAITING_ALIGNMENTORIENTATION",
}

// TimeStatusName is the name of NovAtel time status code, as the ASCII logs
// print it, e.g. FINESTEERING for 180; an unlisted code is its number.
func TimeStatusName(code int) string { return name(timeStatuses, code) }

// INSStatusName is the name of NovAtel inertial solution status code, as the
// ASCII logs print it, e.g. INS_SOLUTION_GOOD for 3; an unlisted code is its
// number.
func INSStatusName(code int) string { return name(insStatuses, code) }

func name(names map[int]string, code int) string {
	if s, ok := names[code]; ok {
		return s
	}
	return strconv.Itoa(code)
}
