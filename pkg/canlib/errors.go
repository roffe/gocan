package canlib

// #cgo LDFLAGS: -lcanlib32
// #include <canlib.h>
import "C"
import (
	"fmt"
	"unsafe"
)

type Error struct {
	Code        int
	Description string
}

func (ke *Error) Error() string {
	return fmt.Sprintf("%v (%v)", ke.Description, ke.Code)
}

func createError(code int) error {
	if code >= int(OK) {
		return nil
	}
	err := [64]C.char{}
	status := int(C.canGetErrorText(C.canStatus(code), &err[0], C.uint(unsafe.Sizeof(err))))
	if status < int(OK) {
		return fmt.Errorf("unable to get description for error code %v (%v)", code, status)
	}
	return &Error{Code: code, Description: C.GoString(&err[0])}
}

func NewError[T ~int | ~int32 | int64](code T) error {
	if code >= T(OK) {
		return nil
	}
	switch code {
	case T(ERR_PARAM):
		return ErrParam
	case T(ERR_NOMSG):
		return ErrNoMsg
	case T(ERR_NOTFOUND):
		return ErrNotFound
	case T(ERR_NOCHANNELS):
		return ErrNoChannels
	case T(ERR_INTERRUPTED):
		return ErrInterrupted
	case T(ERR_TIMEOUT):
		return ErrTimeout
	case T(ERR_NOTINITIALIZED):
		return ErrNotInitialized
	case T(ERR_NOHANDLES):
		return ErrNoHandles
	case T(ERR_INVHANDLE):
		return ErrInvHandle
	case T(ERR_INIFILE):
		return ErrIniFile
	case T(ERR_DRIVER):
		return ErrDriver
	case T(ERR_TXBUFOFL):
		return ErrTxBufOfl
	case T(ERR_RESERVED_1):
		return ErrReserved1
	case T(ERR_HARDWARE):
		return ErrHardware
	case T(ERR_DYNALOAD):
		return ErrDynaLoad
	case T(ERR_DYNALIB):
		return ErrDynaLib
	case T(ERR_DYNAINIT):
		return ErrDynaInit
	case T(ERR_NOT_SUPPORTED):
		return ErrNotSupported
	case T(ERR_RESERVED_5):
		return ErrReserved5
	case T(ERR_RESERVED_6):
		return ErrReserved6
	case T(ERR_RESERVED_2):
		return ErrReserved2
	case T(ERR_DRIVERLOAD):
		return ErrDriverLoad
	case T(ERR_DRIVERFAILED):
		return ErrDriverFailed
	case T(ERR_NOCONFIGMGR):
		return ErrNoConfigMgr
	case T(ERR_NOCARD):
		return ErrNoCard
	case T(ERR_RESERVED_7):
		return ErrReserved7
	case T(ERR_REGISTRY):
		return ErrRegistry
	case T(ERR_LICENSE):
		return ErrLicense
	case T(ERR_INTERNAL):
		return ErrInternal
	case T(ERR_NO_ACCESS):
		return ErrNoAccess
	case T(ERR_NOT_IMPLEMENTED):
		return ErrNotImplemented
	case T(ERR_DEVICE_FILE):
		return ErrDeviceFile
	case T(ERR_HOST_FILE):
		return ErrHostFile
	case T(ERR_DISK):
		return ErrDisk
	case T(ERR_CRC):
		return ErrCrc
	case T(ERR_CONFIG):
		return ErrConfig
	case T(ERR_MEMO_FAIL):
		return ErrMemoFail
	case T(ERR_SCRIPT_FAIL):
		return ErrScriptFail
	case T(ERR_SCRIPT_WRONG_VERSION):
		return ErrScriptWrongVersion
	case T(ERR_SCRIPT_TXE_CONTAINER_VERSION):
		return ErrScriptTxeContainerVersion
	case T(ERR_SCRIPT_TXE_CONTAINER_FORMAT):
		return ErrScriptTxeContainerFormat
	case T(ERR_BUFFER_TOO_SMALL):
		return ErrBufferTooSmall
	case T(ERR_IO_WRONG_PIN_TYPE):
		return ErrIoWrongPinType
	case T(ERR_IO_NOT_CONFIRMED):
		return ErrIoNotConfirmed
	case T(ERR_IO_CONFIG_CHANGED):
		return ErrIoConfigChanged
	case T(ERR_IO_PENDING):
		return ErrIoPending
	case T(ERR_IO_NO_VALID_CONFIG):
		return ErrIoNoValidConfig
	case T(ERR__RESERVED):
		return ErrReserved
	default:
		return &Error{Code: int(code), Description: "Unknown error"}
	}
}

var (
	ErrParam                     error = createError(int(ERR_PARAM))
	ErrNoMsg                     error = createError(int(ERR_NOMSG))
	ErrNotFound                  error = createError(int(ERR_NOTFOUND))
	ErrNoChannels                error = createError(int(ERR_NOCHANNELS))
	ErrInterrupted               error = createError(int(ERR_INTERRUPTED))
	ErrTimeout                   error = createError(int(ERR_TIMEOUT))
	ErrNotInitialized            error = createError(int(ERR_NOTINITIALIZED))
	ErrNoHandles                 error = createError(int(ERR_NOHANDLES))
	ErrInvHandle                 error = createError(int(ERR_INVHANDLE))
	ErrIniFile                   error = createError(int(ERR_INIFILE))
	ErrDriver                    error = createError(int(ERR_DRIVER))
	ErrTxBufOfl                  error = createError(int(ERR_TXBUFOFL))
	ErrReserved1                 error = createError(int(ERR_RESERVED_1))
	ErrHardware                  error = createError(int(ERR_HARDWARE))
	ErrDynaLoad                  error = createError(int(ERR_DYNALOAD))
	ErrDynaLib                   error = createError(int(ERR_DYNALIB))
	ErrDynaInit                  error = createError(int(ERR_DYNAINIT))
	ErrNotSupported              error = createError(int(ERR_NOT_SUPPORTED))
	ErrReserved5                 error = createError(int(ERR_RESERVED_5))
	ErrReserved6                 error = createError(int(ERR_RESERVED_6))
	ErrReserved2                 error = createError(int(ERR_RESERVED_2))
	ErrDriverLoad                error = createError(int(ERR_DRIVERLOAD))
	ErrDriverFailed              error = createError(int(ERR_DRIVERFAILED))
	ErrNoConfigMgr               error = createError(int(ERR_NOCONFIGMGR))
	ErrNoCard                    error = createError(int(ERR_NOCARD))
	ErrReserved7                 error = createError(int(ERR_RESERVED_7))
	ErrRegistry                  error = createError(int(ERR_REGISTRY))
	ErrLicense                   error = createError(int(ERR_LICENSE))
	ErrInternal                  error = createError(int(ERR_INTERNAL))
	ErrNoAccess                  error = createError(int(ERR_NO_ACCESS))
	ErrNotImplemented            error = createError(int(ERR_NOT_IMPLEMENTED))
	ErrDeviceFile                error = createError(int(ERR_DEVICE_FILE))
	ErrHostFile                  error = createError(int(ERR_HOST_FILE))
	ErrDisk                      error = createError(int(ERR_DISK))
	ErrCrc                       error = createError(int(ERR_CRC))
	ErrConfig                    error = createError(int(ERR_CONFIG))
	ErrMemoFail                  error = createError(int(ERR_MEMO_FAIL))
	ErrScriptFail                error = createError(int(ERR_SCRIPT_FAIL))
	ErrScriptWrongVersion        error = createError(int(ERR_SCRIPT_WRONG_VERSION))
	ErrScriptTxeContainerVersion error = createError(int(ERR_SCRIPT_TXE_CONTAINER_VERSION))
	ErrScriptTxeContainerFormat  error = createError(int(ERR_SCRIPT_TXE_CONTAINER_FORMAT))
	ErrBufferTooSmall            error = createError(int(ERR_BUFFER_TOO_SMALL))
	ErrIoWrongPinType            error = createError(int(ERR_IO_WRONG_PIN_TYPE))
	ErrIoNotConfirmed            error = createError(int(ERR_IO_NOT_CONFIRMED))
	ErrIoConfigChanged           error = createError(int(ERR_IO_CONFIG_CHANGED))
	ErrIoPending                 error = createError(int(ERR_IO_PENDING))
	ErrIoNoValidConfig           error = createError(int(ERR_IO_NO_VALID_CONFIG))
	ErrReserved                  error = createError(int(ERR__RESERVED))
)

type CANStatus int

const (
	OK                               CANStatus = C.canOK
	ERR_PARAM                        CANStatus = C.canERR_PARAM
	ERR_NOMSG                        CANStatus = C.canERR_NOMSG
	ERR_NOTFOUND                     CANStatus = C.canERR_NOTFOUND
	ERR_NOMEM                        CANStatus = C.canERR_NOMEM
	ERR_NOCHANNELS                   CANStatus = C.canERR_NOCHANNELS
	ERR_INTERRUPTED                  CANStatus = C.canERR_INTERRUPTED
	ERR_TIMEOUT                      CANStatus = C.canERR_TIMEOUT
	ERR_NOTINITIALIZED               CANStatus = C.canERR_NOTINITIALIZED
	ERR_NOHANDLES                    CANStatus = C.canERR_NOHANDLES
	ERR_INVHANDLE                    CANStatus = C.canERR_INVHANDLE
	ERR_INIFILE                      CANStatus = C.canERR_INIFILE
	ERR_DRIVER                       CANStatus = C.canERR_DRIVER
	ERR_TXBUFOFL                     CANStatus = C.canERR_TXBUFOFL
	ERR_RESERVED_1                   CANStatus = C.canERR_RESERVED_1
	ERR_HARDWARE                     CANStatus = C.canERR_HARDWARE
	ERR_DYNALOAD                     CANStatus = C.canERR_DYNALOAD
	ERR_DYNALIB                      CANStatus = C.canERR_DYNALIB
	ERR_DYNAINIT                     CANStatus = C.canERR_DYNAINIT
	ERR_NOT_SUPPORTED                CANStatus = C.canERR_NOT_SUPPORTED
	ERR_RESERVED_5                   CANStatus = C.canERR_RESERVED_5
	ERR_RESERVED_6                   CANStatus = C.canERR_RESERVED_6
	ERR_RESERVED_2                   CANStatus = C.canERR_RESERVED_2
	ERR_DRIVERLOAD                   CANStatus = C.canERR_DRIVERLOAD
	ERR_DRIVERFAILED                 CANStatus = C.canERR_DRIVERFAILED
	ERR_NOCONFIGMGR                  CANStatus = C.canERR_NOCONFIGMGR
	ERR_NOCARD                       CANStatus = C.canERR_NOCARD
	ERR_RESERVED_7                   CANStatus = C.canERR_RESERVED_7
	ERR_REGISTRY                     CANStatus = C.canERR_REGISTRY
	ERR_LICENSE                      CANStatus = C.canERR_LICENSE
	ERR_INTERNAL                     CANStatus = C.canERR_INTERNAL
	ERR_NO_ACCESS                    CANStatus = C.canERR_NO_ACCESS
	ERR_NOT_IMPLEMENTED              CANStatus = C.canERR_NOT_IMPLEMENTED
	ERR_DEVICE_FILE                  CANStatus = C.canERR_DEVICE_FILE
	ERR_HOST_FILE                    CANStatus = C.canERR_HOST_FILE
	ERR_DISK                         CANStatus = C.canERR_DISK
	ERR_CRC                          CANStatus = C.canERR_CRC
	ERR_CONFIG                       CANStatus = C.canERR_CONFIG
	ERR_MEMO_FAIL                    CANStatus = C.canERR_MEMO_FAIL
	ERR_SCRIPT_FAIL                  CANStatus = C.canERR_SCRIPT_FAIL
	ERR_SCRIPT_WRONG_VERSION         CANStatus = C.canERR_SCRIPT_WRONG_VERSION
	ERR_SCRIPT_TXE_CONTAINER_VERSION CANStatus = C.canERR_SCRIPT_TXE_CONTAINER_VERSION
	ERR_SCRIPT_TXE_CONTAINER_FORMAT  CANStatus = C.canERR_SCRIPT_TXE_CONTAINER_FORMAT
	ERR_BUFFER_TOO_SMALL             CANStatus = C.canERR_BUFFER_TOO_SMALL
	ERR_IO_WRONG_PIN_TYPE            CANStatus = C.canERR_IO_WRONG_PIN_TYPE
	ERR_IO_NOT_CONFIRMED             CANStatus = C.canERR_IO_NOT_CONFIRMED
	ERR_IO_CONFIG_CHANGED            CANStatus = C.canERR_IO_CONFIG_CHANGED
	ERR_IO_PENDING                   CANStatus = C.canERR_IO_PENDING
	ERR_IO_NO_VALID_CONFIG           CANStatus = C.canERR_IO_NO_VALID_CONFIG
	ERR__RESERVED                    CANStatus = C.canERR__RESERVED
)
