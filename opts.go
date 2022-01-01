package canusb

import "log"

// Canusb runs
// clock freq 16
// saab i-bus BTR0=CB and BTR1=9A
// saab p-bus 500kbit
func OptRate(kbit float64) string {
	switch kbit {
	case 10:
		return "S0"
	case 20:
		return "S1"
	case 47.619:
		// BTR0 0xCB, BTR1 0x9A
		return "scb9a"
	case 50:
		return "S2"
	case 100:
		return "S3"
	case 125:
		return "S4"
	case 250:
		return "S5"
	case 500:
		return "S6"
	case 800:
		return "S7"
	case 1000:
		return "S8"
	default:
		log.Fatal("unknown rate")
		return ""
	}
}
