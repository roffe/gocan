package gmlan

import (
	"bytes"
	"errors"

	"github.com/roffe/gocan"
)

type GMError struct {
	Service string
	Code    string
}

func (e GMError) Error() string {
	return e.Service + " - " + e.Code
}

func CheckErr(frame gocan.CANFrame) error {
	d := frame.Data()
	if d[1] == 0x7F {
		return &GMError{TranslateServiceCode(d[2]), TranslateErrorCode(d[3])}
	}
	if bytes.Equal(d, []byte{0x01, 0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) {
		return errors.New("Busy, repeat request")
	}
	return nil
}

func TranslateServiceCode(p byte) string {
	switch p {
	case 0x04:
		return "ClearDiagnosticInformation"
	case 0x10:
		return "InitiateDiagnosticOperation"
	case 0x12:
		return "ReadFailureRecordData"
	case 0x1A:
		return "ReadDataByIdentifier"
	case 0x20:
		return "ReturnToNormalMode"
	case 0x22:
		return "ReadDataByParameterIdentifier"
	case 0x23:
		return "ReadMemoryByAddress"
	case 0x27:
		return "SecurityAccess"
	case 0x28:
		return "DisableNormalCommunication"
	case 0x2C:
		return "DynamicallyDefineMessage"
	case 0x2D:
		return "DefinePIDByAddress"
	case 0x34:
		return "RequestDownload"
	case 0x36:
		return "TransferData"
	case 0x3B:
		return "WriteDataByIdentifier"
	case 0x3E:
		return "TesterPresent"
	case 0xA2:
		return "ReportProgrammedState"
	case 0xA5:
		return "ProgrammingMode"
	case 0xA9:
		return "ReadDiagnosticInformation"
	case 0xAA:
		return "ReadDataByPacketIdentifier"
	case 0xAE:
		return "DeviceControl"
	default:
		return "Unknown"
	}
}

func TranslateErrorCode(p byte) string {
	switch p {
	case 0x10:
		return "General reject"
	case 0x11:
		return "Service not supported"
	case 0x12:
		return "SubFunction not supported - invalid format"
	case 0x21:
		return "Busy, repeat request"
	case 0x22:
		return "Conditions not correct or request sequence error"
	case 0x23:
		return "Routine not completed or service in progress"
	case 0x31:
		return "Request out of range or session dropped"
	case 0x33:
		return "Security access denied"
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
	case 0x50:
		return "Upload (ECU -> PC) not accepted"
	case 0x51:
		return "Improper upload (ECU -> PC) type"
	case 0x52:
		return "Unable to upload (ECU -> PC) for specified address"
	case 0x53:
		return "Unable to upload (ECU -> PC) number of bytes requested"
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
	case 0x81:
		return "Scheduler full"
	case 0x83:
		return "Voltage out of range"
	case 0x85:
		return "General programming failure"
	case 0x89:
		return "Device type error"
	case 0x99:
		return "Ready for download"
	case 0xE3:
		return "DeviceControl Limits Exceeded"
	}
	return "Unknown errror"
}
