package canlib

import (
	"fmt"
)

type Error struct {
	Code        int
	Description string
}

func (ke *Error) Error() string {
	return fmt.Sprintf("%v (%v)", ke.Description, ke.Code)
}

func NewError[T ~int | ~int32 | int64](code T) error {
	if code >= T(ERR_OK) {
		return nil
	}
	switch code {
	case T(ERR_PARAM):
		return ErrParam
	case T(ERR_NOMSG):
		return ErrNoMsg
	case T(ERR_NOTFOUND):
		return ErrNotFound
	case T(ERR_NOMEM):
		return ErrNoMem
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
	ErrOK                        = &Error{Code: 0, Description: "No error"}
	ErrParam                     = &Error{Code: -1, Description: "Error in parameter"}
	ErrNoMsg                     = &Error{Code: -2, Description: "No messages available"}
	ErrNotFound                  = &Error{Code: -3, Description: "Specified device not found"}
	ErrNoMem                     = &Error{Code: -4, Description: "Out of memory"}
	ErrNoChannels                = &Error{Code: -5, Description: "No channels available"}
	ErrInterrupted               = &Error{Code: -6, Description: "Interrupted by signals"}
	ErrTimeout                   = &Error{Code: -7, Description: "Timeout occurred"}
	ErrNotInitialized            = &Error{Code: -8, Description: "Library not initialized"}
	ErrNoHandles                 = &Error{Code: -9, Description: "No more handles"}
	ErrInvHandle                 = &Error{Code: -10, Description: "Handle is invalid"}
	ErrIniFile                   = &Error{Code: -11, Description: "Error in the ini-file"}
	ErrDriver                    = &Error{Code: -12, Description: "CAN driver type not supported"}
	ErrTxBufOfl                  = &Error{Code: -13, Description: "Transmit buffer overflow"}
	ErrReserved1                 = &Error{Code: -14, Description: "Unknown error (-14)"}
	ErrHardware                  = &Error{Code: -15, Description: "A hardware error was detected"}
	ErrDynaLoad                  = &Error{Code: -16, Description: "Can not find requested DLL"}
	ErrDynaLib                   = &Error{Code: -17, Description: "DLL seems to be wrong version"}
	ErrDynaInit                  = &Error{Code: -18, Description: "Error initializing DLL or driver"}
	ErrNotSupported              = &Error{Code: -19, Description: "Operation not supported by hardware or firmware"}
	ErrReserved5                 = &Error{Code: -20, Description: "Unknown error (-20)"}
	ErrReserved6                 = &Error{Code: -21, Description: "Unknown error (-21)"}
	ErrReserved2                 = &Error{Code: -22, Description: "Unknown error (-22)"}
	ErrDriverLoad                = &Error{Code: -23, Description: "Can not load or open the device driver"}
	ErrDriverFailed              = &Error{Code: -24, Description: "The I/O request failed, probably due to resource shortage"}
	ErrNoConfigMgr               = &Error{Code: -25, Description: "Can't find required configuration software"}
	ErrNoCard                    = &Error{Code: -26, Description: "The card was removed or not inserted"}
	ErrReserved7                 = &Error{Code: -27, Description: "Unknown error (-27)"}
	ErrRegistry                  = &Error{Code: -28, Description: "The Registry information is incorrect"}
	ErrLicense                   = &Error{Code: -29, Description: "The license is not valid"}
	ErrInternal                  = &Error{Code: -30, Description: "Internal error in the driver"}
	ErrNoAccess                  = &Error{Code: -31, Description: "Access denied"}
	ErrNotImplemented            = &Error{Code: -32, Description: "Not implemented"}
	ErrDeviceFile                = &Error{Code: -33, Description: "Device File error"}
	ErrHostFile                  = &Error{Code: -34, Description: "Host File error"}
	ErrDisk                      = &Error{Code: -35, Description: "Disk error"}
	ErrCrc                       = &Error{Code: -36, Description: "CRC error"}
	ErrConfig                    = &Error{Code: -37, Description: "Config error"}
	ErrMemoFail                  = &Error{Code: -38, Description: "Memo failure"}
	ErrScriptFail                = &Error{Code: -39, Description: "Script error"}
	ErrScriptWrongVersion        = &Error{Code: -40, Description: "Script version mismatch"}
	ErrScriptTxeContainerVersion = &Error{Code: -41, Description: "Script container version mismatch"}
	ErrScriptTxeContainerFormat  = &Error{Code: -42, Description: "Script container format error"}
	ErrBufferTooSmall            = &Error{Code: -43, Description: "Buffer provided too small to hold data"}
	ErrIoWrongPinType            = &Error{Code: -44, Description: "I/O pin doesn't exist or I/O pin type mismatch"}
	ErrIoNotConfirmed            = &Error{Code: -45, Description: "I/O pin configuration is not confirmed"}
	ErrIoConfigChanged           = &Error{Code: -46, Description: "Configuration changed after last call to kvIoConfirmConfig"}
	ErrIoPending                 = &Error{Code: -47, Description: "The previous I/O pin value has not yet changed the output and i"}
	ErrIoNoValidConfig           = &Error{Code: -48, Description: "There is no valid I/O pin configuration"}
	ErrReserved                  = &Error{Code: -49, Description: "Unknown error (-49)"}
)

type CANStatus int32

const (
	ERR_OK                           CANStatus = 0
	ERR_PARAM                        CANStatus = -1
	ERR_NOMSG                        CANStatus = -2
	ERR_NOTFOUND                     CANStatus = -3
	ERR_NOMEM                        CANStatus = -4
	ERR_NOCHANNELS                   CANStatus = -5
	ERR_INTERRUPTED                  CANStatus = -6
	ERR_TIMEOUT                      CANStatus = -7
	ERR_NOTINITIALIZED               CANStatus = -8
	ERR_NOHANDLES                    CANStatus = -9
	ERR_INVHANDLE                    CANStatus = -10
	ERR_INIFILE                      CANStatus = -11
	ERR_DRIVER                       CANStatus = -12
	ERR_TXBUFOFL                     CANStatus = -13
	ERR_RESERVED_1                   CANStatus = -14
	ERR_HARDWARE                     CANStatus = -15
	ERR_DYNALOAD                     CANStatus = -16
	ERR_DYNALIB                      CANStatus = -17
	ERR_DYNAINIT                     CANStatus = -18
	ERR_NOT_SUPPORTED                CANStatus = -19
	ERR_RESERVED_5                   CANStatus = -20
	ERR_RESERVED_6                   CANStatus = -21
	ERR_RESERVED_2                   CANStatus = -22
	ERR_DRIVERLOAD                   CANStatus = -23
	ERR_DRIVERFAILED                 CANStatus = -24
	ERR_NOCONFIGMGR                  CANStatus = -25
	ERR_NOCARD                       CANStatus = -26
	ERR_RESERVED_7                   CANStatus = -27
	ERR_REGISTRY                     CANStatus = -28
	ERR_LICENSE                      CANStatus = -29
	ERR_INTERNAL                     CANStatus = -30
	ERR_NO_ACCESS                    CANStatus = -31
	ERR_NOT_IMPLEMENTED              CANStatus = -32
	ERR_DEVICE_FILE                  CANStatus = -33
	ERR_HOST_FILE                    CANStatus = -34
	ERR_DISK                         CANStatus = -35
	ERR_CRC                          CANStatus = -36
	ERR_CONFIG                       CANStatus = -37
	ERR_MEMO_FAIL                    CANStatus = -38
	ERR_SCRIPT_FAIL                  CANStatus = -39
	ERR_SCRIPT_WRONG_VERSION         CANStatus = -40
	ERR_SCRIPT_TXE_CONTAINER_VERSION CANStatus = -41
	ERR_SCRIPT_TXE_CONTAINER_FORMAT  CANStatus = -42
	ERR_BUFFER_TOO_SMALL             CANStatus = -43
	ERR_IO_WRONG_PIN_TYPE            CANStatus = -44
	ERR_IO_NOT_CONFIRMED             CANStatus = -45
	ERR_IO_CONFIG_CHANGED            CANStatus = -46
	ERR_IO_PENDING                   CANStatus = -47
	ERR_IO_NO_VALID_CONFIG           CANStatus = -48
	ERR__RESERVED                    CANStatus = -49
)
