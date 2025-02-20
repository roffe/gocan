package canusbcgo

import "fmt"

type Error struct {
	Code        ErrorCode
	Description string
}

func (ke *Error) Error() string {
	return fmt.Sprintf("%v (%v)", ke.Description, ke.Code)
}

func NewError[T ~int | ~int32 | int64](code T) error {
	if code >= T(ERROR_CANUSB_OK) {
		return nil
	}
	switch code {
	case T(ERROR_CANUSB_GENERAL):
		return ErrGeneral
	case T(ERROR_CANUSB_OPEN_SUBSYSTEM):
		return ErrOpenSubsystem
	case T(ERROR_CANUSB_COMMAND_SUBSYSTEM):
		return ErrCommandSubsystem
	case T(ERROR_CANUSB_NOT_OPEN):
		return ErrNotOpen
	case T(ERROR_CANUSB_TX_FIFO_FULL):
		return ErrTxFifoFull
	case T(ERROR_CANUSB_INVALID_PARAM):
		return ErrInvalidParam
	case T(ERROR_CANUSB_NO_MESSAGE):
		return ErrNoMessage
	case T(ERROR_CANUSB_MEMORY_ERROR):
		return ErrMemoryError
	case T(ERROR_CANUSB_NO_DEVICE):
		return ErrNoDevice
	case T(ERROR_CANUSB_TIMEOUT):
		return ErrTimeout
	case T(ERROR_CANUSB_INVALID_HARDWARE):
		return ErrInvalidHardware
	default:
		return &Error{ErrorCode(code), "Unknown error"}
	}
}

func IsNoMessage(err error) bool {
	return err == ErrNoMessage
}

type ErrorCode int

const (
	ERROR_CANUSB_OK                ErrorCode = 0x01
	ERROR_CANUSB_GENERAL           ErrorCode = -0x01
	ERROR_CANUSB_OPEN_SUBSYSTEM    ErrorCode = -0x02
	ERROR_CANUSB_COMMAND_SUBSYSTEM ErrorCode = -0x03
	ERROR_CANUSB_NOT_OPEN          ErrorCode = -0x04
	ERROR_CANUSB_TX_FIFO_FULL      ErrorCode = -0x05
	ERROR_CANUSB_INVALID_PARAM     ErrorCode = -0x06
	ERROR_CANUSB_NO_MESSAGE        ErrorCode = -0x07
	ERROR_CANUSB_MEMORY_ERROR      ErrorCode = -0x08
	ERROR_CANUSB_NO_DEVICE         ErrorCode = -0x09
	ERROR_CANUSB_TIMEOUT           ErrorCode = -0x10
	ERROR_CANUSB_INVALID_HARDWARE  ErrorCode = -0x11
)

var (
	ErrOK               = &Error{ERROR_CANUSB_OK, "OK"}
	ErrGeneral          = &Error{ERROR_CANUSB_GENERAL, "General error"}
	ErrOpenSubsystem    = &Error{ERROR_CANUSB_OPEN_SUBSYSTEM, "Open subsystem error"}
	ErrCommandSubsystem = &Error{ERROR_CANUSB_COMMAND_SUBSYSTEM, "Command subsystem error"}
	ErrNotOpen          = &Error{ERROR_CANUSB_NOT_OPEN, "Not open error"}
	ErrTxFifoFull       = &Error{ERROR_CANUSB_TX_FIFO_FULL, "Transmit FIFO full"}
	ErrInvalidParam     = &Error{ERROR_CANUSB_INVALID_PARAM, "Invalid parameter"}
	ErrNoMessage        = &Error{ERROR_CANUSB_NO_MESSAGE, "No message"}
	ErrMemoryError      = &Error{ERROR_CANUSB_MEMORY_ERROR, "Memory error"}
	ErrNoDevice         = &Error{ERROR_CANUSB_NO_DEVICE, "No device"}
	ErrTimeout          = &Error{ERROR_CANUSB_TIMEOUT, "Timeout"}
	ErrInvalidHardware  = &Error{ERROR_CANUSB_INVALID_HARDWARE, "Invalid hardware"}
)
