package passthru

import (
	"errors"
	"fmt"
)

var (
	ErrNotSupported        = errors.New("device cannot support requested functionality mandated in J2534. Device is not fully SAE J2534 compliant")
	ErrInvalidChannelID    = errors.New("invalid ChannelID value")
	ErrInvalidProtocolID   = errors.New("invalid or unsupported ProtocolID, or there is a resource conflict (i.e. trying to connect to multiple mutually exclusive protocols such as J1850PWM and J1850VPW, or CAN and SCI, etc.)")
	ErrNullParameter       = errors.New("NULL pointer supplied where a valid pointer is required")
	ErrInvalidIoctlValue   = errors.New("invalid value for Ioctl parameter")
	ErrInvalidFlags        = errors.New("invalid flag values")
	ErrFailed              = errors.New("undefined error, use PassThruGetLastError() for text description")
	ErrDeviceNotConnected  = errors.New("unable to communicate with device")
	ErrTimeout             = errors.New("read or write timeout")
	ErrInvalidMsg          = errors.New("invalid message structure pointed to by pMsg")
	ErrInvalidTimeInterval = errors.New("invalid TimeInterval value")
	ErrExceededLimit       = errors.New("exceeded maximum number of message IDs or allocated space")
	ErrInvalidMsgID        = errors.New("invalid MsgID value")
	ErrDeviceInUse         = errors.New("device is currently open")
	ErrInvalidIoctlID      = errors.New("invalid IoctlID value")
	ErrBufferEmpty         = errors.New("protocol message buffer empty, no messages available to read")
	ErrBufferFull          = errors.New("protocol message buffer full. All the messages specified may not have been transmitted")
	ErrBufferOverflow      = errors.New("indicates a buffer overflow occurred and messages were lost")
	ErrPinInvalid          = errors.New("invalid pin number, pin number already in use, or voltage already applied to a different pin")
	ErrChannelInUse        = errors.New("channel number is currently connected")
	ErrMsgProtocolID       = errors.New("protocol type in the message does not match the protocol associated with the Channel ID")
	ErrInvalidFilterID     = errors.New("invalid Filter ID value")
	ErrNoFlowControl       = errors.New("no flow control filter set or matched (for ProtocolID ISO15765 only)")
	ErrNotUnique           = errors.New("a CAN ID in pPatternMsg or pFlowControlMsg matches either ID in an existing FLOW_CONTROL_FILTER")
	ErrInvalidBaudrate     = errors.New("the desired baud rate cannot be achieved within the tolerance specified in SAE J2534-1 Section 6.5")
	ErrInvalidDeviceID     = errors.New("device ID invalid")
	ErrUnknown             = errors.New("unknown error")
)

func CheckError(ret uint32) error {
	switch ret {
	case STATUS_NOERROR:
		//return errors.New("Function call successful")
		return nil
	case ERR_NOT_SUPPORTED:
		return ErrNotSupported
	case ERR_INVALID_CHANNEL_ID:
		return ErrInvalidChannelID
	case ERR_INVALID_PROTOCOL_ID:
		return ErrInvalidProtocolID
	case ERR_NULL_PARAMETER:
		return ErrNullParameter
	case ERR_INVALID_IOCTL_VALUE:
		return ErrInvalidIoctlValue
	case ERR_INVALID_FLAGS:
		return ErrInvalidFlags
	case ERR_FAILED:
		return ErrFailed
	case ERR_DEVICE_NOT_CONNECTED:
		return ErrDeviceNotConnected
	case ERR_TIMEOUT:
		return ErrTimeout
	case ERR_INVALID_MSG:
		return ErrInvalidMsg
	case ERR_INVALID_TIME_INTERVAL:
		return ErrInvalidTimeInterval
	case ERR_EXCEEDED_LIMIT:
		return ErrExceededLimit
	case ERR_INVALID_MSG_ID:
		return ErrInvalidMsgID
	case ERR_DEVICE_IN_USE:
		return ErrDeviceInUse
	case ERR_INVALID_IOCTL_ID:
		return ErrInvalidIoctlID
	case ERR_BUFFER_EMPTY:
		return ErrBufferEmpty
	case ERR_BUFFER_FULL:
		return ErrBufferFull
	case ERR_BUFFER_OVERFLOW:
		return ErrBufferOverflow
	case ERR_PIN_INVALID:
		return ErrPinInvalid
	case ERR_CHANNEL_IN_USE:
		return ErrChannelInUse
	case ERR_MSG_PROTOCOL_ID:
		return ErrMsgProtocolID
	case ERR_INVALID_FILTER_ID:
		return ErrInvalidFilterID
	case ERR_NO_FLOW_CONTROL:
		return ErrNoFlowControl
	case ERR_NOT_UNIQUE:
		return ErrNotUnique
	case ERR_INVALID_BAUDRATE:
		return ErrInvalidBaudrate
	case ERR_INVALID_DEVICE_ID:
		return ErrInvalidDeviceID
	default:
		return fmt.Errorf("unknown error: %d", ret)
	}
}
