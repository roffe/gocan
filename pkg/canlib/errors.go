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

func NewError(code int) error {
	if code >= int(StatusOK) {
		return nil
	}
	err := [64]C.char{}
	status := int(C.canGetErrorText(C.canStatus(code), &err[0], C.uint(unsafe.Sizeof(err))))
	if status < int(StatusOK) {
		return fmt.Errorf("unable to get description for error code %v (%v)", code, status)
	}
	return &Error{Code: code, Description: C.GoString(&err[0])}
}

var (
	ErrParam                     error = NewError(int(ERR_PARAM))
	ErrNoMsg                     error = NewError(int(ERR_NOMSG))
	ErrNotFound                  error = NewError(int(ERR_NOTFOUND))
	ErrNoChannels                error = NewError(int(ERR_NOCHANNELS))
	ErrInterrupted               error = NewError(int(ERR_INTERRUPTED))
	ErrTimeout                   error = NewError(int(ERR_TIMEOUT))
	ErrNotInitialized            error = NewError(int(ERR_NOTINITIALIZED))
	ErrNoHandles                 error = NewError(int(ERR_NOHANDLES))
	ErrInvHandle                 error = NewError(int(ERR_INVHANDLE))
	ErrIniFile                   error = NewError(int(ERR_INIFILE))
	ErrDriver                    error = NewError(int(ERR_DRIVER))
	ErrTxBufOfl                  error = NewError(int(ERR_TXBUFOFL))
	ErrReserved1                 error = NewError(int(ERR_RESERVED_1))
	ErrHardware                  error = NewError(int(ERR_HARDWARE))
	ErrDynaLoad                  error = NewError(int(ERR_DYNALOAD))
	ErrDynaLib                   error = NewError(int(ERR_DYNALIB))
	ErrDynaInit                  error = NewError(int(ERR_DYNAINIT))
	ErrNotSupported              error = NewError(int(ERR_NOT_SUPPORTED))
	ErrReserved5                 error = NewError(int(ERR_RESERVED_5))
	ErrReserved6                 error = NewError(int(ERR_RESERVED_6))
	ErrReserved2                 error = NewError(int(ERR_RESERVED_2))
	ErrDriverLoad                error = NewError(int(ERR_DRIVERLOAD))
	ErrDriverFailed              error = NewError(int(ERR_DRIVERFAILED))
	ErrNoConfigMgr               error = NewError(int(ERR_NOCONFIGMGR))
	ErrNoCard                    error = NewError(int(ERR_NOCARD))
	ErrReserved7                 error = NewError(int(ERR_RESERVED_7))
	ErrRegistry                  error = NewError(int(ERR_REGISTRY))
	ErrLicense                   error = NewError(int(ERR_LICENSE))
	ErrInternal                  error = NewError(int(ERR_INTERNAL))
	ErrNoAccess                  error = NewError(int(ERR_NO_ACCESS))
	ErrNotImplemented            error = NewError(int(ERR_NOT_IMPLEMENTED))
	ErrDeviceFile                error = NewError(int(ERR_DEVICE_FILE))
	ErrHostFile                  error = NewError(int(ERR_HOST_FILE))
	ErrDisk                      error = NewError(int(ERR_DISK))
	ErrCrc                       error = NewError(int(ERR_CRC))
	ErrConfig                    error = NewError(int(ERR_CONFIG))
	ErrMemoFail                  error = NewError(int(ERR_MEMO_FAIL))
	ErrScriptFail                error = NewError(int(ERR_SCRIPT_FAIL))
	ErrScriptWrongVersion        error = NewError(int(ERR_SCRIPT_WRONG_VERSION))
	ErrScriptTxeContainerVersion error = NewError(int(ERR_SCRIPT_TXE_CONTAINER_VERSION))
	ErrScriptTxeContainerFormat  error = NewError(int(ERR_SCRIPT_TXE_CONTAINER_FORMAT))
	ErrBufferTooSmall            error = NewError(int(ERR_BUFFER_TOO_SMALL))
	ErrIoWrongPinType            error = NewError(int(ERR_IO_WRONG_PIN_TYPE))
	ErrIoNotConfirmed            error = NewError(int(ERR_IO_NOT_CONFIRMED))
	ErrIoConfigChanged           error = NewError(int(ERR_IO_CONFIG_CHANGED))
	ErrIoPending                 error = NewError(int(ERR_IO_PENDING))
	ErrIoNoValidConfig           error = NewError(int(ERR_IO_NO_VALID_CONFIG))
	ErrReserved                  error = NewError(int(ERR__RESERVED))
)

type CANStatus int

const (
	StatusOK                         CANStatus = C.canOK
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
