package kwp2000

import "fmt"

func TranslateErrorCode(p byte) string {
	switch p {
	case 0x00:
		return "Affirmative response"
	case 0x10:
		return "General reject"
	case 0x11:
		return "Mode not supported"
	case 0x12:
		return "Sub-function not supported - invalid format"
	case 0x21:
		return "Busy, repeat request"
	case 0x22:
		return "conditions not correct or request sequence error"
	case 0x23:
		return "Routine not completed or service in progress"
	case 0x31:
		return "Request out of range or session dropped"
	case 0x33:
		return "Security access denied"
	case 0x34:
		return "Security access allowed"
	case 0x35:
		return "Invalid key supplied"
	case 0x36:
		return "Exceeded number of attempts to get security access"
	case 0x37:
		return "Required time delay not expired, you cannot gain security access at this moment"
	case 0x40:
		return "Download (PC -> ECU) not accepted"
	case 0x41:
		return "Improper download (PC -> ECU) type"
	case 0x42:
		return "Unable to download (PC -> ECU) to specified address"
	case 0x43:
		return "Unable to download (PC -> ECU) number of bytes requested"
	case 0x44:
		return "Ready for download"
	case 0x50:
		return "Upload (ECU -> PC) not accepted"
	case 0x51:
		return "Improper upload (ECU -> PC) type"
	case 0x52:
		return "Unable to upload (ECU -> PC) for specified address"
	case 0x53:
		return "Unable to upload (ECU -> PC) number of bytes requested"
	case 0x54:
		return "Ready for upload"
	case 0x61:
		return "Normal exit with results available"
	case 0x62:
		return "Normal exit without results available"
	case 0x63:
		return "Abnormal exit with results"
	case 0x64:
		return "Abnormal exit without results"
	case 0x71:
		return "Transfer suspended"
	case 0x72:
		return "Transfer aborted"
	case 0x74:
		return "Illegal address in block transfer"
	case 0x75:
		return "Illegal byte count in block transfer"
	case 0x76:
		return "Illegal block transfer type"
	case 0x77:
		return "Block transfer data checksum error"
	case 0x78:
		return "Response pending"
	case 0x79:
		return "Incorrect byte count during block transfer"
	case 0x80:
		return "Service not supported in current diagnostics session"
	default:
		return fmt.Sprintf("Unknown error %X", p)
	}
}
