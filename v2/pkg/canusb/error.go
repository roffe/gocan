package canusb

import "fmt"

func checkErr(r1, _ uintptr, _ error) error {
	return NewError(int32(r1))
}

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

type ErrorCode int32

const (
	ERROR_CANUSB_OK                ErrorCode = 1
	ERROR_CANUSB_GENERAL           ErrorCode = -1
	ERROR_CANUSB_OPEN_SUBSYSTEM    ErrorCode = -2
	ERROR_CANUSB_COMMAND_SUBSYSTEM ErrorCode = -3
	ERROR_CANUSB_NOT_OPEN          ErrorCode = -4
	ERROR_CANUSB_TX_FIFO_FULL      ErrorCode = -5
	ERROR_CANUSB_INVALID_PARAM     ErrorCode = -6
	ERROR_CANUSB_NO_MESSAGE        ErrorCode = -7
	ERROR_CANUSB_MEMORY_ERROR      ErrorCode = -8
	ERROR_CANUSB_NO_DEVICE         ErrorCode = -9
	ERROR_CANUSB_TIMEOUT           ErrorCode = -10
	ERROR_CANUSB_INVALID_HARDWARE  ErrorCode = -11
)

var (
	ErrOK               = &Error{ERROR_CANUSB_OK, "OK"}
	ErrGeneral          = &Error{ERROR_CANUSB_GENERAL, "general error"}
	ErrOpenSubsystem    = &Error{ERROR_CANUSB_OPEN_SUBSYSTEM, "open subsystem error"}
	ErrCommandSubsystem = &Error{ERROR_CANUSB_COMMAND_SUBSYSTEM, "command subsystem error"}
	ErrNotOpen          = &Error{ERROR_CANUSB_NOT_OPEN, "not open error"}
	ErrTxFifoFull       = &Error{ERROR_CANUSB_TX_FIFO_FULL, "transmit FIFO full"}
	ErrInvalidParam     = &Error{ERROR_CANUSB_INVALID_PARAM, "invalid parameter"}
	ErrNoMessage        = &Error{ERROR_CANUSB_NO_MESSAGE, "no message"}
	ErrMemoryError      = &Error{ERROR_CANUSB_MEMORY_ERROR, "memory error"}
	ErrNoDevice         = &Error{ERROR_CANUSB_NO_DEVICE, "no device"}
	ErrTimeout          = &Error{ERROR_CANUSB_TIMEOUT, "timeout"}
	ErrInvalidHardware  = &Error{ERROR_CANUSB_INVALID_HARDWARE, "invalid hardware"}

	ErrNoDeviceAvailable  = &Error{-100, "no device available"}
	ErrMessageDataToLarge = &Error{-101, "message data to large"}
	ErrMessageDataSize    = &Error{-102, "message data size missmatch"}
	ErrMessageDataToSmall = &Error{-103, "message data to small"}

	ErrReceiveFifoFull  = &Error{CANSTATUS_RECEIVE_FIFO_FULL, "receive FIFO full"}
	ErrTransmitFifoFull = &Error{CANSTATUS_TRANSMIT_FIFO_FULL, "transmit FIFO full"}
	ErrWarning          = &Error{CANSTATUS_ERROR_WARNING, "error warning (EI)"}
	ErrDataOverrun      = &Error{CANSTATUS_DATA_OVERRUN, "data overrun (DOI)"}
	ErrErrorPassive     = &Error{CANSTATUS_ERROR_PASSIVE, "error passive (EPI)"}
	ErrArbitrationLost  = &Error{CANSTATUS_ARBITRATION_LOST, "arbitration lost (ALI)"}
	ErrBussError        = &Error{CANSTATUS_BUS_ERROR, "bus error (BEI)"}
)
