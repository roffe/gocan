//go:build canlib

package canlib

// #cgo LDFLAGS: -lcanlib32
// #include <canlib.h>
import "C"
import (
	"fmt"
	"unsafe"
)

type CANMessage struct {
	Identifier uint32
	Data       []byte
	DLC        uint32
	Flags      uint
	Time       uint
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

type AcceptFlag uint

const (
	FILTER_ACCEPT       AcceptFlag = C.canFILTER_ACCEPT
	FILTER_REJECT       AcceptFlag = C.canFILTER_REJECT
	FILTER_SET_CODE_STD AcceptFlag = C.canFILTER_SET_CODE_STD
	FILTER_SET_MASK_STD AcceptFlag = C.canFILTER_SET_MASK_STD
	FILTER_SET_CODE_EXT AcceptFlag = C.canFILTER_SET_CODE_EXT
	FILTER_SET_MASK_EXT AcceptFlag = C.canFILTER_SET_MASK_EXT
	FILTER_NULL_MASK    AcceptFlag = C.canFILTER_NULL_MASK
)

// This routine sets the message acceptance filters on a CAN channel.
// On some boards the acceptance filtering is done by the CAN hardware; on other boards (typically those with an embedded CPU,) the acceptance filtering is done by software. canAccept() behaves in the same way for all boards, however.
// SetAcceptanceFilter() and Accept() both serve the same purpose but the former can set the code and mask in just one call.
// If you want to remove a filter, call canAccept() with the mask set to 0.

func (hnd Handle) Accept(envelope int, flag AcceptFlag) error {
	status := int(C.canAccept(C.int(hnd), C.long(envelope), C.uint(flag)))
	return NewError(status)
}

// Closes the channel associated with the handle. If no other threads are using the CAN circuit, it is taken off bus. The handle can not be used for further references to the channel, so any variable containing it should be zeroed.
// Close() will almost always return canOK; the specified handle is closed on an best-effort basis.
func (hnd Handle) Close() error {
	status := int(C.canClose(C.int(hnd)))
	hnd = -1
	return NewError(status)
}

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

// This function removes all received messages from the handle's receive queue. Other handles open to the same channel are not affected by this operation. That is, only the messages belonging to the handle you are passing to canFlushReceiveQueue are discarded.
func (hnd Handle) FlushReceiveQueue() error {
	status := int(C.canFlushReceiveQueue(C.int(hnd)))
	return NewError(status)
}

// This function removes all messages pending transmission from the transmit queue of the circuit.
func (hnd Handle) FlushTransmitQueue() error {
	status := int(C.canFlushTransmitQueue(C.int(hnd)))
	return NewError(status)
}

func (hnd Handle) ObjBufAllocate(typ int) (int, error) {
	idx := int(C.canObjBufAllocate(C.int(hnd), C.int(typ)))
	if idx < 0 {
		return -1, NewError(idx)
	}
	return idx, nil
}

type MSG_FLAG uint

const (
	MSG_MASK        MSG_FLAG = C.canMSG_MASK
	MSG_RTR         MSG_FLAG = C.canMSG_RTR
	MSG_STD         MSG_FLAG = C.canMSG_STD
	MSG_EXT         MSG_FLAG = C.canMSG_EXT
	MSG_WAKEUP      MSG_FLAG = C.canMSG_WAKEUP
	MSG_NERR        MSG_FLAG = C.canMSG_NERR
	MSG_ERROR_FRAME MSG_FLAG = C.canMSG_ERROR_FRAME
	MSG_TXACK       MSG_FLAG = C.canMSG_TXACK
	MSG_TXRQ        MSG_FLAG = C.canMSG_TXRQ
	MSG_DELAY_MSG   MSG_FLAG = C.canMSG_DELAY_MSG
	MSG_LOCAL_TXACK MSG_FLAG = C.canMSG_LOCAL_TXACK
	MSG_SINGLE_SHOT MSG_FLAG = C.canMSG_SINGLE_SHOT
	MSG_TXNACK      MSG_FLAG = C.canMSG_TXNACK
	MSG_ABL         MSG_FLAG = C.canMSG_ABL

	FDMSG_MASK MSG_FLAG = C.canFDMSG_MASK
	FDMSG_EDL  MSG_FLAG = C.canFDMSG_EDL
	FDMSG_FDF  MSG_FLAG = C.canFDMSG_FDF
	FDMSG_BRS  MSG_FLAG = C.canFDMSG_BRS
	FDMSG_ESI  MSG_FLAG = C.canFDMSG_ESI
)

func (hnd Handle) ObjBufWrite(idx, id int, message []byte, flags MSG_FLAG) error {
	status := int(C.canObjBufWrite(C.int(hnd), C.int(idx), C.int(id), unsafe.Pointer(&message[0]), C.uint(len(message)), C.uint(flags)))
	return NewError(status)
}

// This function tries to reset a CAN bus controller by taking the channel off bus and then on bus again (if it was on bus before the call to canResetBus().)
// This function will affect the hardware (and cause a real reset of the CAN chip) only if hnd is the only handle open on the channel. If there are other open handles, this operation will not affect the hardware.
func (hnd Handle) ResetBus() error {
	status := int(C.canResetBus(C.int(hnd)))
	return NewError(status)
}

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

// This routine sets the message acceptance filters on a CAN channel.
//
// Format of code and mask:
//
//   - A binary 1 in a mask means "the corresponding bit in the code is relevant"
//   - A binary 0 in a mask means "the corresponding bit in the code is not relevant"
//   - A relevant binary 1 in a code means "the corresponding bit in the identifier must be 1"
//   - A relevant binary 0 in a code means "the corresponding bit in the identifier must be 0"
//
// In other words, the message is accepted if ((code XOR id) AND mask) == 0.
//
// extended should be set to:
//
//  0. (FALSE): if the code and mask shall apply to 11-bit CAN identifiers.
//  1. (TRUE): if the code and mask shall apply to 29-bit CAN identifiers.
//
// If you want to remove a filter, call canSetAcceptanceFilter() with the mask set to 0.
//
// On some boards the acceptance filtering is done by the CAN hardware; on other boards (typically those with an embedded CPU,) the acceptance filtering is done by software. canSetAcceptanceFilter() behaves in the same way for all boards, however.
// canSetAcceptanceFilter() and canAccept() both serve the same purpose but the former can set the code and mask in just one call.
func (hnd Handle) SetAcceptanceFilter(code, mask uint, extended bool) error {
	ext := C.int(0)
	if extended {
		ext = 1
	}
	status := int(C.canSetAcceptanceFilter(C.int(hnd), C.uint(code), C.uint(mask), ext))
	return NewError(status)
}

// The canSetBitrate() function sets the nominal bit rate of the specified CAN channel. The sampling point is recalculated and kept as close as possible to the value before the call.
//
// Parameters:
//
//	[bitrate]	bitrate	The new bit rate, in bits/second.
func (hnd Handle) SetBitrate(bitrate int) error {
	status := int(C.canSetBitrate(C.int(hnd), C.int(bitrate)))
	return NewError(status)
}

// This function sets the nominal bus timing parameters for the specified CAN controller.
// The library provides default values for tseg1, tseg2, sjw and noSamp when freq is specified to one of the pre-defined constants, canBITRATE_xxx for classic CAN and canFD_BITRATE_xxx for CAN FD.
// If freq is any other value, no default values are supplied by the library.
// If you are using multiple handles to the same physical channel, for example if you are writing a threaded application, you must call canBusOff() once for each handle. The same applies to canBusOn() - the physical channel will not go off bus until the last handle to the channel goes off bus.
func (hnd Handle) SetBusParams(freq BusParamsFreq, tseg1, tseg2, sjw, noSamp, syncmode uint) error {
	status := int(C.canSetBusParams(C.int(hnd), C.long(freq), C.uint(tseg1), C.uint(tseg2), C.uint(sjw), C.uint(noSamp), C.uint(syncmode)))
	return NewError(status)
}

// This function sets the bus timing parameters using the same convention as the 82c200 CAN controller (which is the same as many other CAN controllers, for example, the 82527.)
// To calculate the bit timing parameters, you can use the bit timing calculator that is included with CANlib SDK. Look in the BIN directory.
func (hnd Handle) SetBusParamsC200(btr0, btr1 uint8) error {
	status := int(C.canSetBusParamsC200(C.int(hnd), C.uchar(btr0), C.uchar(btr1)))
	return NewError(status)
}

type BusControlDriverType uint

const (
	DRIVER_NORMAL        BusControlDriverType = C.canDRIVER_NORMAL
	DRIVER_SILENT        BusControlDriverType = C.canDRIVER_SILENT
	DRIVER_SELFRECEPTION BusControlDriverType = C.canDRIVER_SELFRECEPTION
	DRIVER_OFF           BusControlDriverType = C.canDRIVER_OFF
)

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
func (hnd Handle) ReadErrorCounters() (uint, uint, uint, error) {
	var tec, rec, ovr C.uint
	status := int(C.canReadErrorCounters(C.int(hnd), &tec, &rec, &ovr))
	return uint(tec), uint(rec), uint(ovr), NewError(status)
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
		Flags:      uint(flags),
		Time:       uint(time),
	}, nil
}

// This function sends a CAN message. The call returns immediately after queuing the message to the driver so the message has not necessarily been transmitted.
func (hnd Handle) Write(identifier uint32, data []byte, flags MSG_FLAG) error {
	status := int(C.canWrite(C.int(hnd), C.long(identifier), unsafe.Pointer(&data[0]), C.uint(len(data)), C.uint(flags)))
	return NewError(status)
}

// Waits until all CAN messages for the specified handle are sent, or the timeout period expires.
func (hnd Handle) WriteSync(timeoutMS int) error {
	status := C.canWriteSync(C.int(hnd), C.ulong(timeoutMS))
	return NewError(int(status))
}

// This function sends a CAN message and returns when the message has been successfully transmitted, or the timeout expires.
func (hnd Handle) WriteWait(identifier uint32, data []byte, flags MSG_FLAG, timeoutMS int) error {
	status := int(C.canWriteWait(C.int(hnd), C.long(identifier), unsafe.Pointer(&data[0]), C.uint(len(data)), C.uint(flags), C.ulong(timeoutMS)))
	return NewError(status)

}
