//go:build canlib

package canlib

// #cgo LDFLAGS: -lcanlib32
// #include <canlib.h>
import "C"
import (
	"fmt"
	"unsafe"
)

type BusControlDriverType uint

const (
	DRIVER_NORMAL        BusControlDriverType = C.canDRIVER_NORMAL
	DRIVER_SILENT        BusControlDriverType = C.canDRIVER_SILENT
	DRIVER_SELFRECEPTION BusControlDriverType = C.canDRIVER_SELFRECEPTION
	DRIVER_OFF           BusControlDriverType = C.canDRIVER_OFF
)

type BusParamsFreq int

const (
	BITRATE_1M   BusParamsFreq = C.canBITRATE_1M
	BITRATE_500K BusParamsFreq = C.canBITRATE_500K
	BITRATE_250K BusParamsFreq = C.canBITRATE_250K
	BITRATE_125K BusParamsFreq = C.canBITRATE_125K
	BITRATE_100K BusParamsFreq = C.canBITRATE_100K
	BITRATE_62K  BusParamsFreq = C.canBITRATE_62K
	BITRATE_50K  BusParamsFreq = C.canBITRATE_50K
	BITRATE_83K  BusParamsFreq = C.canBITRATE_83K
	BITRATE_10K  BusParamsFreq = C.canBITRATE_10K
)

type CANMessage struct {
	Identifier uint32
	Data       []byte
	DLC        uint32
	Flags      uint32
	Time       uint32
}

type CANMessageFlag uint

const (
	MSG_STD   CANMessageFlag = C.canMSG_STD
	MSG_EXT   CANMessageFlag = C.canMSG_EXT
	FDMSG_FDF CANMessageFlag = C.canFDMSG_FDF
	FDMSG_BRS CANMessageFlag = C.canFDMSG_BRS
)

type CANStatus int

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

type ChannelDataItem int

const (
	CHANNELDATA_CHANNEL_CAP              ChannelDataItem = C.canCHANNELDATA_CHANNEL_CAP
	CHANNELDATA_TRANS_CAP                ChannelDataItem = C.canCHANNELDATA_TRANS_CAP
	CHANNELDATA_CHANNEL_FLAGS            ChannelDataItem = C.canCHANNELDATA_CHANNEL_FLAGS
	CHANNELDATA_CARD_TYPE                ChannelDataItem = C.canCHANNELDATA_CARD_TYPE
	CHANNELDATA_CARD_NUMBER              ChannelDataItem = C.canCHANNELDATA_CARD_NUMBER
	CHANNELDATA_CHAN_NO_ON_CARD          ChannelDataItem = C.canCHANNELDATA_CHAN_NO_ON_CARD
	CHANNELDATA_CARD_SERIAL_NO           ChannelDataItem = C.canCHANNELDATA_CARD_SERIAL_NO
	CHANNELDATA_TRANS_SERIAL_NO          ChannelDataItem = C.canCHANNELDATA_TRANS_SERIAL_NO
	CHANNELDATA_CARD_FIRMWARE_REV        ChannelDataItem = C.canCHANNELDATA_CARD_FIRMWARE_REV
	CHANNELDATA_CARD_HARDWARE_REV        ChannelDataItem = C.canCHANNELDATA_CARD_HARDWARE_REV
	CHANNELDATA_CARD_UPC_NO              ChannelDataItem = C.canCHANNELDATA_CARD_UPC_NO
	CHANNELDATA_TRANS_UPC_NO             ChannelDataItem = C.canCHANNELDATA_TRANS_UPC_NO
	CHANNELDATA_CHANNEL_NAME             ChannelDataItem = C.canCHANNELDATA_CHANNEL_NAME
	CHANNELDATA_DLL_FILE_VERSION         ChannelDataItem = C.canCHANNELDATA_DLL_FILE_VERSION
	CHANNELDATA_DLL_PRODUCT_VERSION      ChannelDataItem = C.canCHANNELDATA_DLL_PRODUCT_VERSION
	CHANNELDATA_DLL_FILETYPE             ChannelDataItem = C.canCHANNELDATA_DLL_FILETYPE
	CHANNELDATA_TRANS_TYPE               ChannelDataItem = C.canCHANNELDATA_TRANS_TYPE
	CHANNELDATA_DEVICE_PHYSICAL_POSITION ChannelDataItem = C.canCHANNELDATA_DEVICE_PHYSICAL_POSITION
	CHANNELDATA_UI_NUMBER                ChannelDataItem = C.canCHANNELDATA_UI_NUMBER
	CHANNELDATA_TIMESYNC_ENABLED         ChannelDataItem = C.canCHANNELDATA_TIMESYNC_ENABLED
	CHANNELDATA_DRIVER_FILE_VERSION      ChannelDataItem = C.canCHANNELDATA_DRIVER_FILE_VERSION
	CHANNELDATA_DRIVER_PRODUCT_VERSION   ChannelDataItem = C.canCHANNELDATA_DRIVER_PRODUCT_VERSION
	CHANNELDATA_MFGNAME_UNICODE          ChannelDataItem = C.canCHANNELDATA_MFGNAME_UNICODE
	CHANNELDATA_MFGNAME_ASCII            ChannelDataItem = C.canCHANNELDATA_MFGNAME_ASCII
	CHANNELDATA_DEVDESCR_UNICODE         ChannelDataItem = C.canCHANNELDATA_DEVDESCR_UNICODE
	CHANNELDATA_DEVDESCR_ASCII           ChannelDataItem = C.canCHANNELDATA_DEVDESCR_ASCII
	CHANNELDATA_DRIVER_NAME              ChannelDataItem = C.canCHANNELDATA_DRIVER_NAME
	CHANNELDATA_CHANNEL_QUALITY          ChannelDataItem = C.canCHANNELDATA_CHANNEL_QUALITY
	CHANNELDATA_ROUNDTRIP_TIME           ChannelDataItem = C.canCHANNELDATA_ROUNDTRIP_TIME
	CHANNELDATA_BUS_TYPE                 ChannelDataItem = C.canCHANNELDATA_BUS_TYPE
	CHANNELDATA_DEVNAME_ASCII            ChannelDataItem = C.canCHANNELDATA_DEVNAME_ASCII
	CHANNELDATA_TIME_SINCE_LAST_SEEN     ChannelDataItem = C.canCHANNELDATA_TIME_SINCE_LAST_SEEN
	CHANNELDATA_REMOTE_OPERATIONAL_MODE  ChannelDataItem = C.canCHANNELDATA_REMOTE_OPERATIONAL_MODE
	CHANNELDATA_REMOTE_PROFILE_NAME      ChannelDataItem = C.canCHANNELDATA_REMOTE_PROFILE_NAME
	CHANNELDATA_REMOTE_HOST_NAME         ChannelDataItem = C.canCHANNELDATA_REMOTE_HOST_NAME
	CHANNELDATA_REMOTE_MAC               ChannelDataItem = C.canCHANNELDATA_REMOTE_MAC
	CHANNELDATA_MAX_BITRATE              ChannelDataItem = C.canCHANNELDATA_MAX_BITRATE
	CHANNELDATA_CHANNEL_CAP_MASK         ChannelDataItem = C.canCHANNELDATA_CHANNEL_CAP_MASK
	CHANNELDATA_CUST_CHANNEL_NAME        ChannelDataItem = C.canCHANNELDATA_CUST_CHANNEL_NAME
	CHANNELDATA_IS_REMOTE                ChannelDataItem = C.canCHANNELDATA_IS_REMOTE
	CHANNELDATA_REMOTE_TYPE              ChannelDataItem = C.canCHANNELDATA_REMOTE_TYPE
	CHANNELDATA_LOGGER_TYPE              ChannelDataItem = C.canCHANNELDATA_LOGGER_TYPE
	CHANNELDATA_HW_STATUS                ChannelDataItem = C.canCHANNELDATA_HW_STATUS
	CHANNELDATA_FEATURE_EAN              ChannelDataItem = C.canCHANNELDATA_FEATURE_EAN
	CHANNELDATA_BUS_PARAM_LIMITS         ChannelDataItem = C.canCHANNELDATA_BUS_PARAM_LIMITS
	CHANNELDATA_CLOCK_INFO               ChannelDataItem = C.canCHANNELDATA_CLOCK_INFO
	CHANNELDATA_CHANNEL_CAP_EX           ChannelDataItem = C.canCHANNELDATA_CHANNEL_CAP_EX
)

type OpenChannelFlag int

const (
	OPEN_EXCLUSIVE           OpenChannelFlag = C.canOPEN_EXCLUSIVE           // Exclusive access
	OPEN_REQUIRE_EXTENDED    OpenChannelFlag = C.canOPEN_REQUIRE_EXTENDED    // Fail if can't use extended mode
	OPEN_ACCEPT_VIRTUAL      OpenChannelFlag = C.canOPEN_ACCEPT_VIRTUAL      // Allow use of virtual CAN
	OPEN_OVERRIDE_EXCLUSIVE  OpenChannelFlag = C.canOPEN_OVERRIDE_EXCLUSIVE  // Open, even if in exclusive access
	OPEN_REQUIRE_INIT_ACCESS OpenChannelFlag = C.canOPEN_REQUIRE_INIT_ACCESS // Init access to bus
	OPEN_NO_INIT_ACCESS      OpenChannelFlag = C.canOPEN_NO_INIT_ACCESS
	OPEN_ACCEPT_LARGE_DLC    OpenChannelFlag = C.canOPEN_ACCEPT_LARGE_DLC
	OPEN_CAN_FD              OpenChannelFlag = C.canOPEN_CAN_FD
	OPEN_CAN_FD_NONISO       OpenChannelFlag = C.canOPEN_CAN_FD_NONISO
	OPEN_INTERNAL_L          OpenChannelFlag = C.canOPEN_INTERNAL_L
)

type CanlibError struct {
	Code        int
	Description string
}

func (ke *CanlibError) Error() string {
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
	return &CanlibError{Code: code, Description: C.GoString(&err[0])}
}

func InitializeLibrary() {
	C.canInitializeLibrary()
}

func UnloadLibrary() error {
	return NewError(int(C.canUnloadLibrary()))
}

func GetNumberOfChannels() (int, error) {
	noChannels := C.int(0)
	status := int(C.canGetNumberOfChannels(&noChannels))
	return int(noChannels), NewError(status)
}

// This function can be used to retrieve certain pieces of information about a channel.
func GetChannelDataString(channel int, item ChannelDataItem) (string, error) {
	msg := [256]C.char{}
	err := NewError(int(C.canGetChannelData(C.int(channel), C.int(item), unsafe.Pointer(&msg[0]), C.size_t(unsafe.Sizeof(msg)))))
	if err != nil {
		return "", err
	}
	return C.GoString(&msg[0]), nil
}

func GetChannelByte(channel int, item ChannelDataItem) ([]byte, error) {
	msg := make([]byte, 256, 256)
	status := int(C.canGetChannelData(C.int(channel), C.int(item), unsafe.Pointer(&msg[0]), C.size_t(len(msg))))
	if err := NewError(status); err != nil {
		return nil, err
	}
	return msg, nil
}

// Opens a CAN channel (circuit) and returns a handle which is used in subsequent calls to CANlib.
func OpenChannel(channel int, flags OpenChannelFlag) (Handle, error) {
	handle := C.canOpenChannel(C.int(channel), C.int(flags))
	return Handle(handle), NewError(int(handle))
}

// This API call returns the version of the CANlib API DLL (canlib32.dll).
func GetVersion() string {
	version := C.canGetVersion()
	return fmt.Sprintf("%d.%d", version>>8, version&0xFF)
}

// Handle is a handle to a CAN channel (circuit).

type Handle int

// Takes the specified channel on-bus.
// If you are using multiple handles to the same physical channel, for example if you are writing a threaded application, you must call canBusOn() once for each handle. The same applies to canBusOff() - the physical channel will not go off bus until the last handle to the channel goes off bus.
func (hnd Handle) BusOn() error {
	status := int(C.canBusOn(C.int(hnd)))
	return NewError(status)
}

// Takes the specified handle off-bus. If no other handle is active on the same channel, the channel will also be taken off-bus
func (hnd Handle) BusOff() error {
	status := int(C.canBusOff(C.int(hnd)))
	return NewError(status)
}

// Closes the channel associated with the handle. If no other threads are using the CAN circuit, it is taken off bus. The handle can not be used for further references to the channel, so any variable containing it should be zeroed.
// Close() will almost always return canOK; the specified handle is closed on an best-effort basis.
func (hnd Handle) Close() error {
	status := int(C.canClose(C.int(hnd)))
	hnd = -1
	return NewError(status)
}

func (hnd Handle) SetBusParams(freq BusParamsFreq, tseg1, tseg2, sjw, noSamp, syncmode uint) error {
	status := int(C.canSetBusParams(C.int(hnd), C.long(freq), C.uint(tseg1), C.uint(tseg2), C.uint(sjw), C.uint(noSamp), C.uint(syncmode)))
	return NewError(status)
}

func (hnd Handle) SetBusParamsC200(btr0, btr1 byte) error {
	status := int(C.canSetBusParamsC200(C.int(hnd), C.uchar(btr0), C.uchar(btr1)))
	return NewError(status)
}

// This function sets the driver type for a CAN controller.
// This corresponds loosely to the bus output control register in the CAN controller, hence the name of this function.
// CANlib does not allow for direct manipulation of the bus output control register; instead, symbolic constants are used to select the desired driver type.
func SetBusOutputControl(hnd Handle, drivertype BusControlDriverType) error {
	status := int(C.canSetBusOutputControl(C.int(hnd), C.uint(drivertype)))
	return NewError(status)
}

// Reads the error counters of the CAN controller
//
// returns: tx error counter, rx error counter, overrun error counter
func (hnd Handle) ReadErrorCounters() (uint32, uint32, uint32, error) {
	var tec, rec, ovr C.uint
	status := int(C.canReadErrorCounters(C.int(hnd), &tec, &rec, &ovr))
	return uint32(tec), uint32(rec), uint32(ovr), NewError(status)
}

// Reads a message from the receive buffer. If no message is available, the function waits until a message arrives or a timeout occurs.
func (hnd Handle) ReadWait(timeout int) (*CANMessage, error) {
	identifier := C.long(0)
	var data [16]byte
	dlc := C.uint(0)
	flags := C.uint(0)
	time := C.ulong(0)
	timeOut := C.ulong(timeout)
	status := int(C.canReadWait(C.int(hnd), &identifier, unsafe.Pointer(&data), &dlc, &flags, &time, timeOut))
	if err := NewError(status); err != nil {
		return nil, err
	}
	return &CANMessage{
		Identifier: uint32(identifier),
		Data:       data[:dlc],
		DLC:        uint32(dlc),
		Flags:      uint32(flags),
		Time:       uint32(time),
	}, nil
}

// This function sends a CAN message. The call returns immediately after queuing the message to the driver so the message has not necessarily been transmitted.
func (hnd Handle) Write(identifier uint32, data []byte, flags CANMessageFlag) error {
	status := int(C.canWrite(C.int(hnd), C.long(identifier), unsafe.Pointer(&data[0]), C.uint(len(data)), C.uint(flags)))
	return NewError(status)
}

// Waits until all CAN messages for the specified handle are sent, or the timeout period expires.
func (hnd Handle) WriteSync(timeoutMS int) error {
	status := C.canWriteSync(C.int(hnd), C.ulong(timeoutMS))
	return NewError(int(status))
}

// This function sends a CAN message and returns when the message has been successfully transmitted, or the timeout expires.
func (hnd Handle) WriteWait(identifier uint32, data []byte, flags CANMessageFlag, timeoutMS int) error {
	status := int(C.canWriteWait(C.int(hnd), C.long(identifier), unsafe.Pointer(&data[0]), C.uint(len(data)), C.uint(flags), C.ulong(timeoutMS)))
	return NewError(status)

}
