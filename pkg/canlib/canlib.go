package canlib

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

var (
	canlib32                = syscall.NewLazyDLL("canlib32.dll")
	procInitializeLibrary   = canlib32.NewProc("canInitializeLibrary")
	procUnloadLibrary       = canlib32.NewProc("canUnloadLibrary")
	procGetNumberOfChannels = canlib32.NewProc("canGetNumberOfChannels")
	procGetChannelData      = canlib32.NewProc("canGetChannelData")
	procGetErrorText        = canlib32.NewProc("canGetErrorText")
	procOpenChannel         = canlib32.NewProc("canOpenChannel")
	procGetVersion          = canlib32.NewProc("canGetVersion")
	procAccept              = canlib32.NewProc("canAccept")
	procClose               = canlib32.NewProc("canClose")
	procBusOn               = canlib32.NewProc("canBusOn")
	procBusOff              = canlib32.NewProc("canBusOff")
	procFlushReceiveQueue   = canlib32.NewProc("canFlushReceiveQueue")
	procFlushTransmitQueue  = canlib32.NewProc("canFlushTransmitQueue")
	procObjBufAllocate      = canlib32.NewProc("canObjBufAllocate")
	procObjBufWrite         = canlib32.NewProc("canObjBufWrite")
	procResetBus            = canlib32.NewProc("canResetBus")
	procSetAcceptanceFilter = canlib32.NewProc("canSetAcceptanceFilter")
	procSetBitrate          = canlib32.NewProc("canSetBitrate")
	procSetBusParams        = canlib32.NewProc("canSetBusParams")
	procSetBusParamsC200    = canlib32.NewProc("canSetBusParamsC200")
	procSetBusOutputControl = canlib32.NewProc("canSetBusOutputControl")
	procReadErrorCounters   = canlib32.NewProc("canReadErrorCounters")
	procRead                = canlib32.NewProc("canRead")
	procReadWait            = canlib32.NewProc("canReadWait")
	procWrite               = canlib32.NewProc("canWrite")
	procWriteSync           = canlib32.NewProc("canWriteSync")
	procWriteWait           = canlib32.NewProc("canWriteWait")
	prockvSetNotifyCallback = canlib32.NewProc("kvSetNotifyCallback")
)

// Handle is a handle to a CAN channel (circuit).
type Handle int32

type CANMessage struct {
	Identifier uint32
	Data       []byte
	DLC        uint32
	Flags      uint32
	Timestamp  uint32
}

func InitializeLibrary() error {
	return checkErr(procInitializeLibrary.Call())
}

func UnloadLibrary() error {
	return checkErr(procUnloadLibrary.Call())
}

func GetNumberOfChannels() (int, error) {
	var noChannels int32
	r1, _, _ := procGetNumberOfChannels.Call(uintptr(unsafe.Pointer(&noChannels)))
	return int(noChannels), NewError(int32(r1))
}

type ChannelDataItem int32

const (
	CHANNELDATA_CHANNEL_CAP              ChannelDataItem = 1
	CHANNELDATA_TRANS_CAP                ChannelDataItem = 2
	CHANNELDATA_CHANNEL_FLAGS            ChannelDataItem = 3
	CHANNELDATA_CARD_TYPE                ChannelDataItem = 4
	CHANNELDATA_CARD_NUMBER              ChannelDataItem = 5
	CHANNELDATA_CHAN_NO_ON_CARD          ChannelDataItem = 6
	CHANNELDATA_CARD_SERIAL_NO           ChannelDataItem = 7
	CHANNELDATA_TRANS_SERIAL_NO          ChannelDataItem = 8
	CHANNELDATA_CARD_FIRMWARE_REV        ChannelDataItem = 9
	CHANNELDATA_CARD_HARDWARE_REV        ChannelDataItem = 10
	CHANNELDATA_CARD_UPC_NO              ChannelDataItem = 11
	CHANNELDATA_TRANS_UPC_NO             ChannelDataItem = 12
	CHANNELDATA_CHANNEL_NAME             ChannelDataItem = 13
	CHANNELDATA_DLL_FILE_VERSION         ChannelDataItem = 14
	CHANNELDATA_DLL_PRODUCT_VERSION      ChannelDataItem = 15
	CHANNELDATA_DLL_FILETYPE             ChannelDataItem = 16
	CHANNELDATA_TRANS_TYPE               ChannelDataItem = 17
	CHANNELDATA_DEVICE_PHYSICAL_POSITION ChannelDataItem = 18
	CHANNELDATA_UI_NUMBER                ChannelDataItem = 19
	CHANNELDATA_TIMESYNC_ENABLED         ChannelDataItem = 20
	CHANNELDATA_DRIVER_FILE_VERSION      ChannelDataItem = 21
	CHANNELDATA_DRIVER_PRODUCT_VERSION   ChannelDataItem = 22
	CHANNELDATA_MFGNAME_UNICODE          ChannelDataItem = 23
	CHANNELDATA_MFGNAME_ASCII            ChannelDataItem = 24
	CHANNELDATA_DEVDESCR_UNICODE         ChannelDataItem = 25
	CHANNELDATA_DEVDESCR_ASCII           ChannelDataItem = 26
	CHANNELDATA_DRIVER_NAME              ChannelDataItem = 27
	CHANNELDATA_CHANNEL_QUALITY          ChannelDataItem = 28
	CHANNELDATA_ROUNDTRIP_TIME           ChannelDataItem = 29
	CHANNELDATA_BUS_TYPE                 ChannelDataItem = 30
	CHANNELDATA_DEVNAME_ASCII            ChannelDataItem = 31
	CHANNELDATA_TIME_SINCE_LAST_SEEN     ChannelDataItem = 32
	CHANNELDATA_REMOTE_OPERATIONAL_MODE  ChannelDataItem = 33
	CHANNELDATA_REMOTE_PROFILE_NAME      ChannelDataItem = 34
	CHANNELDATA_REMOTE_HOST_NAME         ChannelDataItem = 35
	CHANNELDATA_REMOTE_MAC               ChannelDataItem = 36
	CHANNELDATA_MAX_BITRATE              ChannelDataItem = 37
	CHANNELDATA_CHANNEL_CAP_MASK         ChannelDataItem = 38
	CHANNELDATA_CUST_CHANNEL_NAME        ChannelDataItem = 39
	CHANNELDATA_IS_REMOTE                ChannelDataItem = 40
	CHANNELDATA_REMOTE_TYPE              ChannelDataItem = 41
	CHANNELDATA_LOGGER_TYPE              ChannelDataItem = 42
	CHANNELDATA_HW_STATUS                ChannelDataItem = 43
	CHANNELDATA_FEATURE_EAN              ChannelDataItem = 44
	CHANNELDATA_BUS_PARAM_LIMITS         ChannelDataItem = 45
	CHANNELDATA_CLOCK_INFO               ChannelDataItem = 46
	CHANNELDATA_CHANNEL_CAP_EX           ChannelDataItem = 47
)

// This function can be used to retrieve certain pieces of information about a channel.
func GetChannelDataString(channel int, item ChannelDataItem) (string, error) {
	data := make([]byte, 256)
	r1, _, _ := procGetChannelData.Call(uintptr(channel), uintptr(item), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	return cBytetoString(data), NewError(int32(r1))
}

type OpenFlag int32

const (
	OPEN_EXCLUSIVE           OpenFlag = 0x8  // Exclusive access
	OPEN_REQUIRE_EXTENDED    OpenFlag = 0x10 // Fail if can't use extended mode
	OPEN_ACCEPT_VIRTUAL      OpenFlag = 0x20 // Allow use of virtual CAN
	OPEN_OVERRIDE_EXCLUSIVE  OpenFlag = 0x40 // Open, even if in exclusive access
	OPEN_REQUIRE_INIT_ACCESS OpenFlag = 0x80 // Init access to bus
	OPEN_NO_INIT_ACCESS      OpenFlag = 0x100
	OPEN_ACCEPT_LARGE_DLC    OpenFlag = 0x200
	OPEN_CAN_FD              OpenFlag = 0x400
	OPEN_CAN_FD_NONISO       OpenFlag = 0x800
	OPEN_INTERNAL_L          OpenFlag = 0x1000
)

// Opens a CAN channel (circuit) and returns a handle which is used in subsequent calls to CANlib.
func OpenChannel(channel int, flags OpenFlag) (Handle, error) {
	r1, _, _ := procOpenChannel.Call(uintptr(channel), uintptr(flags))
	return Handle(r1), NewError(int32(r1))
}

func GetVersion() string {
	r1, _, _ := procGetVersion.Call()
	return fmt.Sprintf("%d.%d", r1>>8, r1&0xFF)
}

type AcceptFlag uint32

const (
	FILTER_ACCEPT       AcceptFlag = 0x01
	FILTER_REJECT       AcceptFlag = 0x02
	FILTER_SET_CODE_STD AcceptFlag = 0x03
	FILTER_SET_MASK_STD AcceptFlag = 0x04
	FILTER_SET_CODE_EXT AcceptFlag = 0x05
	FILTER_SET_MASK_EXT AcceptFlag = 0x06
	FILTER_NULL_MASK    AcceptFlag = 0x00
)

// This routine sets the message acceptance filters on a CAN channel.
// On some boards the acceptance filtering is done by the CAN hardware; on other boards (typically those with an embedded CPU,) the acceptance filtering is done by software. canAccept() behaves in the same way for all boards, however.
// SetAcceptanceFilter() and Accept() both serve the same purpose but the former can set the code and mask in just one call.
// If you want to remove a filter, call canAccept() with the mask set to 0.
func (h Handle) Accept(envelope int, flag AcceptFlag) error {
	return checkErr(procAccept.Call(uintptr(h), uintptr(int32(envelope)), uintptr(flag)))
}

// Closes the channel associated with the handle. If no other threads are using the CAN circuit, it is taken off bus. The handle can not be used for further references to the channel, so any variable containing it should be zeroed.
// Close() will almost always return canOK; the specified handle is closed on an best-effort basis.
func (h Handle) Close() error {
	defer func() {
		h = -1
	}()
	return checkErr(procClose.Call(uintptr(h)))
}

// Takes the specified channel on-bus.
// If you are using multiple handles to the same physical channel, for example if you are writing a threaded application, you must call canBusOn() once for each handle. The same applies to canBusOff() - the physical channel will not go off bus until the last handle to the channel goes off bus.
func (h Handle) BusOn() error {
	return checkErr(procBusOn.Call(uintptr(h)))
}

// Takes the specified handle off-bus. If no other handle is active on the same channel, the channel will also be taken off-bus
func (h Handle) BusOff() error {
	return checkErr(procBusOff.Call(uintptr(h)))
}

// This function removes all received messages from the handle's receive queue. Other handles open to the same channel are not affected by this operation. That is, only the messages belonging to the handle you are passing to canFlushReceiveQueue are discarded.
func (h Handle) FlushReceiveQueue() error {
	return checkErr(procFlushReceiveQueue.Call(uintptr(h)))
}

// This function removes all messages pending transmission from the transmit queue of the circuit.
func (h Handle) FlushTransmitQueue() error {
	return checkErr(procFlushTransmitQueue.Call(uintptr(h)))
}

// Allocates an object buffer associated with a handle to a CAN circuit.
func (h Handle) ObjBufAllocate(typ int) (int, error) {
	r1, _, _ := procObjBufAllocate.Call(uintptr(h), uintptr(typ))
	return int(r1), NewError(int32(r1))
}

type MsgFlag uint32

const (
	MSG_MASK        MsgFlag = 0xFF
	MSG_RTR         MsgFlag = 0x01
	MSG_STD         MsgFlag = 0x02
	MSG_EXT         MsgFlag = 0x04
	MSG_WAKEUP      MsgFlag = 0x08
	MSG_NERR        MsgFlag = 0x10
	MSG_ERROR_FRAME MsgFlag = 0x20
	MSG_TXACK       MsgFlag = 0x40
	MSG_TXRQ        MsgFlag = 0x80
	MSG_DELAY_MSG   MsgFlag = 0x100
	MSG_LOCAL_TXACK MsgFlag = 0x10000000
	MSG_SINGLE_SHOT MsgFlag = 0x1000000
	MSG_TXNACK      MsgFlag = 0x2000000
	MSG_ABL         MsgFlag = 0x4000000

	FDMSG_MASK MsgFlag = 0xff0000
	FDMSG_EDL  MsgFlag = 0x10000
	FDMSG_FDF  MsgFlag = 0x10000
	FDMSG_BRS  MsgFlag = 0x20000
	FDMSG_ESI  MsgFlag = 0x40000
)

func (hnd Handle) ObjBufWrite(idx, id int, message []byte, flags MsgFlag) error {
	return checkErr(procObjBufWrite.Call(uintptr(hnd), uintptr(idx), uintptr(id), uintptr(unsafe.Pointer(&message[0])), uintptr(len(message)), uintptr(flags)))
}

// This function tries to reset a CAN bus controller by taking the channel off bus and then on bus again (if it was on bus before the call to canResetBus().)
// This function will affect the hardware (and cause a real reset of the CAN chip) only if hnd is the only handle open on the channel. If there are other open handles, this operation will not affect the hardware.
func (hnd Handle) ResetBus() error {
	return checkErr(procResetBus.Call(uintptr(hnd)))
}

type BusParamsFreq int32

const (
	BITRATE_1M   BusParamsFreq = -0x01
	BITRATE_500K BusParamsFreq = -0x02
	BITRATE_250K BusParamsFreq = -0x03
	BITRATE_125K BusParamsFreq = -0x04
	BITRATE_100K BusParamsFreq = -0x05
	BITRATE_62K  BusParamsFreq = -0x06
	BITRATE_50K  BusParamsFreq = -0x07
	BITRATE_83K  BusParamsFreq = -0x08
	BITRATE_10K  BusParamsFreq = -0x09
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
	var ext int
	if extended {
		ext = 1
	}
	return checkErr(procSetAcceptanceFilter.Call(uintptr(hnd), uintptr(code), uintptr(mask), uintptr(ext)))
}

// The canSetBitrate() function sets the nominal bit rate of the specified CAN channel. The sampling point is recalculated and kept as close as possible to the value before the call.
//
// Parameters:
//
//	[bitrate]	bitrate	The new bit rate, in bits/second.
func (hnd Handle) SetBitrate(bitrate int) error {
	return checkErr(procSetBitrate.Call(uintptr(hnd), uintptr(int32(bitrate))))
}

// This function sets the nominal bus timing parameters for the specified CAN controller.
// The library provides default values for tseg1, tseg2, sjw and noSamp when freq is specified to one of the pre-defined constants, canBITRATE_xxx for classic CAN and canFD_BITRATE_xxx for CAN FD.
// If freq is any other value, no default values are supplied by the library.
// If you are using multiple handles to the same physical channel, for example if you are writing a threaded application, you must call canBusOff() once for each handle. The same applies to canBusOn() - the physical channel will not go off bus until the last handle to the channel goes off bus.
func (hnd Handle) SetBusParams(freq BusParamsFreq, tseg1, tseg2, sjw, noSamp, syncmode uint32) error {
	return checkErr(procSetBusParams.Call(uintptr(hnd), uintptr(freq), uintptr(tseg1), uintptr(tseg2), uintptr(sjw), uintptr(noSamp), uintptr(syncmode)))
}

// This function sets the bus timing parameters using the same convention as the 82c200 CAN controller (which is the same as many other CAN controllers, for example, the 82527.)
// To calculate the bit timing parameters, you can use the bit timing calculator that is included with CANlib SDK. Look in the BIN directory.
func (hnd Handle) SetBusParamsC200(btr0, btr1 uint8) error {
	return checkErr(procSetBusParamsC200.Call(uintptr(hnd), uintptr(btr0), uintptr(btr1)))
}

type DriverType uint32

const (
	DRIVER_NORMAL        DriverType = 0x04
	DRIVER_SILENT        DriverType = 0x01
	DRIVER_SELFRECEPTION DriverType = 0x08
	DRIVER_OFF           DriverType = 0x00
)

// This function sets the driver type for a CAN controller.
// This corresponds loosely to the bus output control register in the CAN controller, hence the name of this function.
// CANlib does not allow for direct manipulation of the bus output control register; instead, symbolic constants are used to select the desired driver type.
func SetBusOutputControl(hnd Handle, drivertype DriverType) error {
	return checkErr(procSetBusOutputControl.Call(uintptr(hnd), uintptr(drivertype)))
}

// Reads the error counters of the CAN controller
//
// returns: tx error counter, rx error counter, overrun error counter
func (hnd Handle) ReadErrorCounters() (uint32, uint32, uint32, error) {
	var tx, rx, overrun uint32
	r1, _, _ := procReadErrorCounters.Call(uintptr(hnd), uintptr(unsafe.Pointer(&tx)), uintptr(unsafe.Pointer(&rx)), uintptr(unsafe.Pointer(&overrun)))
	return tx, rx, overrun, NewError(int32(r1))
}

func (hnd Handle) Read() (*CANMessage, error) {
	msg := new(CANMessage)
	msg.Data = make([]byte, 64)
	r1, _, _ := procRead.Call(uintptr(hnd), uintptr(unsafe.Pointer(&msg.Identifier)), uintptr(unsafe.Pointer(&msg.Data[0])), uintptr(unsafe.Pointer(&msg.DLC)), uintptr(unsafe.Pointer(&msg.Flags)), uintptr(unsafe.Pointer(&msg.Timestamp)))
	if err := NewError(int32(r1)); err != nil {
		return nil, err
	}
	return msg, nil
}

// Reads a message from the receive buffer. If no message is available, the function waits until a message arrives or a timeout occurs.
func (hnd Handle) ReadWait(timeout uint32) (*CANMessage, error) {
	msg := new(CANMessage)
	msg.Data = make([]byte, 64)
	r1, _, _ := procReadWait.Call(uintptr(hnd), uintptr(unsafe.Pointer(&msg.Identifier)), uintptr(unsafe.Pointer(&msg.Data[0])), uintptr(unsafe.Pointer(&msg.DLC)), uintptr(unsafe.Pointer(&msg.Flags)), uintptr(unsafe.Pointer(&msg.Timestamp)), uintptr(timeout))
	if err := NewError(int32(r1)); err != nil {
		return nil, err
	}
	return msg, nil
}

// This function sends a CAN message.
// The call returns immediately after queuing the message to the driver so the message has not necessarily been transmitted.
func (hnd Handle) Write(identifier uint32, data []byte, flags MsgFlag) error {
	return checkErr(procWrite.Call(uintptr(hnd), uintptr(identifier), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(flags)))
}

// Waits until all CAN messages for the specified handle are sent, or the timeout period expires.
func (hnd Handle) WriteSync(timeoutMS uint32) error {
	return checkErr(procWriteSync.Call(uintptr(hnd), uintptr(timeoutMS)))
}

// This function sends a CAN message and returns when the message has been successfully transmitted, or the timeout expires.
func (hnd Handle) WriteWait(identifier uint32, data []byte, flags MsgFlag, timeoutMS uint32) error {
	return checkErr(procWriteWait.Call(uintptr(hnd), uintptr(identifier), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(flags), uintptr(timeoutMS)))
}

func GetErrorText(status int) (string, error) {
	err := make([]byte, 64)
	r1, _, _ := procGetErrorText.Call(uintptr(status), uintptr(unsafe.Pointer(&err[0])), uintptr(len(err)))
	if int32(r1) < int32(ERR_OK) {
		return "", fmt.Errorf("unable to get description for error code %v (%v)", status, int32(r1))
	}
	return cBytetoString(err), nil
}

func GetChannelDataBytes(channel int, item ChannelDataItem) ([]byte, error) {
	data := make([]byte, 256)
	r1, _, _ := procGetChannelData.Call(uintptr(channel), uintptr(item), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	return data, NewError(int32(r1))
}

func cBytetoString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

type NotifyFlag uint32

const (
	NOTIFY_NONE     NotifyFlag = 0x00
	NOTIFY_RX       NotifyFlag = 0x01
	NOTIFY_TX       NotifyFlag = 0x02
	NOTIFY_ERROR    NotifyFlag = 0x04
	NOTIFY_STATUS   NotifyFlag = 0x08
	NOTIFY_ENVVAR   NotifyFlag = 0x10
	NOTIFY_REMOVED  NotifyFlag = 0x40
	NOTIFY_BUSONOFF NotifyFlag = 0x20
)

type NotifyCallback func(hnd int32, ctx uintptr, event NotifyFlag) uintptr

func (hnd Handle) SetNotifyCallback(cb NotifyCallback, flags NotifyFlag) error {
	if cb == nil {
		log.Println("Setting callback to nil")
		r1, _, _ := prockvSetNotifyCallback.Call(uintptr(hnd), 0, 0, uintptr(flags))
		return NewError(int32(r1))
	}
	return checkErr(prockvSetNotifyCallback.Call(uintptr(hnd), syscall.NewCallback(cb), 0, uintptr(flags)))
}

func checkErr(r1, _ uintptr, _ error) error {
	if r1 != 0 {
		return fmt.Errorf("error code: %d", r1)
	}
	return NewError(int32(r1))
}
