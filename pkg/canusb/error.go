package canusb

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
	ErrGeneral          = &Error{ERROR_CANUSB_GENERAL, "general error (-1)"}
	ErrOpenSubsystem    = &Error{ERROR_CANUSB_OPEN_SUBSYSTEM, "open subsystem error (-2)"}
	ErrCommandSubsystem = &Error{ERROR_CANUSB_COMMAND_SUBSYSTEM, "command subsystem error (-3)"}
	ErrNotOpen          = &Error{ERROR_CANUSB_NOT_OPEN, "not open error (-4)"}
	ErrTxFifoFull       = &Error{ERROR_CANUSB_TX_FIFO_FULL, "transmit FIFO full (-5)"}
	ErrInvalidParam     = &Error{ERROR_CANUSB_INVALID_PARAM, "invalid parameter (-6)"}
	ErrNoMessage        = &Error{ERROR_CANUSB_NO_MESSAGE, "no message (-7)"}
	ErrMemoryError      = &Error{ERROR_CANUSB_MEMORY_ERROR, "memory error (-8)"}
	ErrNoDevice         = &Error{ERROR_CANUSB_NO_DEVICE, "no device (-9)"}
	ErrTimeout          = &Error{ERROR_CANUSB_TIMEOUT, "timeout (-10)"}
	ErrInvalidHardware  = &Error{ERROR_CANUSB_INVALID_HARDWARE, "invalid hardware (-11)"}
)
