package canlib

/*
#cgo LDFLAGS: -lcanlib
#include <stdlib.h>
#include <string.h>
#include <canlib.h>
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

var (
	InitErr  error
	initOnce sync.Once
)

func Init() error {
	initOnce.Do(func() {
		C.canInitializeLibrary()
	})
	return InitErr
}

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
	C.canInitializeLibrary()
	return nil
}

func UnloadLibrary() error {
	return checkErr(C.canUnloadLibrary())
}

func GetNumberOfChannels() (int, error) {
	var n C.int
	r := C.canGetNumberOfChannels(&n)
	return int(n), NewError(int32(r))
}

type ChannelData int32

const (
	CHANNELDATA_CHANNEL_CAP              ChannelData = 0x01
	CHANNELDATA_TRANS_CAP                ChannelData = 0x02
	CHANNELDATA_CHANNEL_FLAGS            ChannelData = 0x03
	CHANNELDATA_CARD_TYPE                ChannelData = 0x04
	CHANNELDATA_CARD_NUMBER              ChannelData = 0x05
	CHANNELDATA_CHAN_NO_ON_CARD          ChannelData = 0x06
	CHANNELDATA_CARD_SERIAL_NO           ChannelData = 0x07
	CHANNELDATA_TRANS_SERIAL_NO          ChannelData = 0x08
	CHANNELDATA_CARD_FIRMWARE_REV        ChannelData = 0x09
	CHANNELDATA_CARD_HARDWARE_REV        ChannelData = 0x0A
	CHANNELDATA_CARD_UPC_NO              ChannelData = 0x0B
	CHANNELDATA_TRANS_UPC_NO             ChannelData = 0x0C
	CHANNELDATA_CHANNEL_NAME             ChannelData = 0x0D
	CHANNELDATA_DLL_FILE_VERSION         ChannelData = 0x0E
	CHANNELDATA_DLL_PRODUCT_VERSION      ChannelData = 0x0F
	CHANNELDATA_DLL_FILETYPE             ChannelData = 0x10
	CHANNELDATA_TRANS_TYPE               ChannelData = 0x11
	CHANNELDATA_DEVICE_PHYSICAL_POSITION ChannelData = 0x12
	CHANNELDATA_UI_NUMBER                ChannelData = 0x13
	CHANNELDATA_TIMESYNC_ENABLED         ChannelData = 0x14
	CHANNELDATA_DRIVER_FILE_VERSION      ChannelData = 0x15
	CHANNELDATA_DRIVER_PRODUCT_VERSION   ChannelData = 0x16
	CHANNELDATA_MFGNAME_UNICODE          ChannelData = 0x17
	CHANNELDATA_MFGNAME_ASCII            ChannelData = 0x18
	CHANNELDATA_DEVDESCR_UNICODE         ChannelData = 0x19
	CHANNELDATA_DEVDESCR_ASCII           ChannelData = 0x1A
	CHANNELDATA_DRIVER_NAME              ChannelData = 0x1B
	CHANNELDATA_CHANNEL_QUALITY          ChannelData = 0x1C
	CHANNELDATA_ROUNDTRIP_TIME           ChannelData = 0x1D
	CHANNELDATA_BUS_TYPE                 ChannelData = 0x1E
	CHANNELDATA_DEVNAME_ASCII            ChannelData = 0x1F
	CHANNELDATA_TIME_SINCE_LAST_SEEN     ChannelData = 0x20
	CHANNELDATA_REMOTE_OPERATIONAL_MODE  ChannelData = 0x21
	CHANNELDATA_REMOTE_PROFILE_NAME      ChannelData = 0x22
	CHANNELDATA_REMOTE_HOST_NAME         ChannelData = 0x23
	CHANNELDATA_REMOTE_MAC               ChannelData = 0x24
	CHANNELDATA_MAX_BITRATE              ChannelData = 0x25
	CHANNELDATA_CHANNEL_CAP_MASK         ChannelData = 0x26
	CHANNELDATA_CUST_CHANNEL_NAME        ChannelData = 0x27
	CHANNELDATA_IS_REMOTE                ChannelData = 0x28
	CHANNELDATA_REMOTE_TYPE              ChannelData = 0x29
	CHANNELDATA_LOGGER_TYPE              ChannelData = 0x2A
	CHANNELDATA_HW_STATUS                ChannelData = 0x2B
	CHANNELDATA_FEATURE_EAN              ChannelData = 0x2C
	CHANNELDATA_BUS_PARAM_LIMITS         ChannelData = 0x2D
	CHANNELDATA_CLOCK_INFO               ChannelData = 0x2E
	CHANNELDATA_CHANNEL_CAP_EX           ChannelData = 0x2F
)

func GetChannelDataString(channel int, item ChannelData) (string, error) {
	data, err := GetChannelDataBytes(channel, item)
	return cBytetoString(data), err
}

func GetChannelDataBytes(channel int, item ChannelData) ([]byte, error) {
	data := make([]byte, 256)
	r := C.canGetChannelData(C.int(channel), C.int(item), unsafe.Pointer(&data[0]), C.size_t(len(data)))
	return data, NewError(int32(r))
}

type OpenFlag int32

const (
	OPEN_EXCLUSIVE           OpenFlag = 0x8
	OPEN_REQUIRE_EXTENDED    OpenFlag = 0x10
	OPEN_ACCEPT_VIRTUAL      OpenFlag = 0x20
	OPEN_OVERRIDE_EXCLUSIVE  OpenFlag = 0x40
	OPEN_REQUIRE_INIT_ACCESS OpenFlag = 0x80
	OPEN_NO_INIT_ACCESS      OpenFlag = 0x100
	OPEN_ACCEPT_LARGE_DLC    OpenFlag = 0x200
	OPEN_CAN_FD              OpenFlag = 0x400
	OPEN_CAN_FD_NONISO       OpenFlag = 0x800
	OPEN_INTERNAL_L          OpenFlag = 0x1000
)

func OpenChannel(channel int, flags OpenFlag) (Handle, error) {
	r := C.canOpenChannel(C.int(channel), C.int(flags))
	return Handle(r), NewError(int32(r))
}

func GetVersion() string {
	r := C.canGetVersion()
	return fmt.Sprintf("%d.%d", uint(r)>>8, uint(r)&0xFF)
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

func (h Handle) Accept(envelope int, flag AcceptFlag) error {
	return checkErr(C.canAccept(C.CanHandle(h), C.long(envelope), C.uint(flag)))
}

func (h Handle) Close() error {
	return checkErr(C.canClose(C.CanHandle(h)))
}

func (h Handle) BusOn() error {
	return checkErr(C.canBusOn(C.CanHandle(h)))
}

func (h Handle) BusOff() error {
	return checkErr(C.canBusOff(C.CanHandle(h)))
}

func (h Handle) FlushReceiveQueue() error {
	return checkErr(C.canFlushReceiveQueue(C.CanHandle(h)))
}

func (h Handle) FlushTransmitQueue() error {
	return checkErr(C.canFlushTransmitQueue(C.CanHandle(h)))
}

func (h Handle) ObjBufAllocate(typ int) (int, error) {
	r := C.canObjBufAllocate(C.CanHandle(h), C.int(typ))
	return int(r), NewError(int32(r))
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

func (h Handle) ObjBufWrite(idx, id int, message []byte, flags MsgFlag) error {
	if len(message) == 0 {
		return checkErr(C.canObjBufWrite(C.CanHandle(h), C.int(idx), C.int(id), nil, 0, C.uint(flags)))
	}
	return checkErr(C.canObjBufWrite(C.CanHandle(h), C.int(idx), C.int(id), unsafe.Pointer(&message[0]), C.uint(len(message)), C.uint(flags)))
}

func (h Handle) ResetBus() error {
	return checkErr(C.canResetBus(C.CanHandle(h)))
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

func (h Handle) SetAcceptanceFilter(code, mask uint, extended bool) error {
	var ext C.int
	if extended {
		ext = 1
	}
	return checkErr(C.canSetAcceptanceFilter(C.CanHandle(h), C.uint(code), C.uint(mask), ext))
}

// SetBitrate sets a custom bit rate. Linux canlib does not export
// canSetBitrate, so we fall back to canSetBusParams with conservative
// default bit-timing values that work for most standard rates. For
// non-standard rates that require precise timing, call SetBusParams
// directly with the desired tseg1/tseg2/sjw/noSamp values.
func (h Handle) SetBitrate(bitrate int) error {
	return checkErr(C.canSetBusParams(C.CanHandle(h), C.long(bitrate), 4, 3, 1, 1, 0))
}

func (h Handle) SetBusParams(freq BusParamsFreq, tseg1, tseg2, sjw, noSamp, syncmode uint32) error {
	return checkErr(C.canSetBusParams(C.CanHandle(h), C.long(freq), C.uint(tseg1), C.uint(tseg2), C.uint(sjw), C.uint(noSamp), C.uint(syncmode)))
}

func (h Handle) SetBusParamsC200(btr0, btr1 uint8) error {
	return checkErr(C.canSetBusParamsC200(C.CanHandle(h), C.uchar(btr0), C.uchar(btr1)))
}

type DriverType uint32

const (
	DRIVER_OFF           DriverType = 0x00
	DRIVER_SILENT        DriverType = 0x01
	DRIVER_NORMAL        DriverType = 0x04
	DRIVER_SELFRECEPTION DriverType = 0x08
)

func SetBusOutputControl(h Handle, drivertype DriverType) error {
	return checkErr(C.canSetBusOutputControl(C.CanHandle(h), C.uint(drivertype)))
}

func (h Handle) ReadErrorCounters() (uint32, uint32, uint32, error) {
	var tx, rx, overrun C.uint
	r := C.canReadErrorCounters(C.CanHandle(h), &tx, &rx, &overrun)
	return uint32(tx), uint32(rx), uint32(overrun), NewError(int32(r))
}

func (h Handle) Read() (*CANMessage, error) {
	var (
		id    C.long
		dlc   C.uint
		flags C.uint
		ts    C.ulong
		data  [64]C.uchar
	)
	r := C.canRead(C.CanHandle(h), &id, unsafe.Pointer(&data[0]), &dlc, &flags, &ts)
	if err := NewError(int32(r)); err != nil {
		return nil, err
	}
	out := make([]byte, dlc)
	for i := 0; i < int(dlc); i++ {
		out[i] = byte(data[i])
	}
	return &CANMessage{
		Identifier: uint32(id),
		Data:       out,
		DLC:        uint32(dlc),
		Flags:      uint32(flags),
		Timestamp:  uint32(ts),
	}, nil
}

func (h Handle) ReadWait(timeout uint32) (*CANMessage, error) {
	var (
		id    C.long
		dlc   C.uint
		flags C.uint
		ts    C.ulong
		data  [64]C.uchar
	)
	r := C.canReadWait(C.CanHandle(h), &id, unsafe.Pointer(&data[0]), &dlc, &flags, &ts, C.ulong(timeout))
	if err := NewError(int32(r)); err != nil {
		return nil, err
	}
	out := make([]byte, dlc)
	for i := 0; i < int(dlc); i++ {
		out[i] = byte(data[i])
	}
	return &CANMessage{
		Identifier: uint32(id),
		Data:       out,
		DLC:        uint32(dlc),
		Flags:      uint32(flags),
		Timestamp:  uint32(ts),
	}, nil
}

func (h Handle) Write(identifier uint32, data []byte, flags MsgFlag) error {
	if len(data) == 0 {
		return checkErr(C.canWrite(C.CanHandle(h), C.long(identifier), nil, 0, C.uint(flags)))
	}
	return checkErr(C.canWrite(C.CanHandle(h), C.long(identifier), unsafe.Pointer(&data[0]), C.uint(len(data)), C.uint(flags)))
}

func (h Handle) WriteSync(timeoutMS uint32) error {
	return checkErr(C.canWriteSync(C.CanHandle(h), C.ulong(timeoutMS)))
}

func (h Handle) WriteWait(identifier uint32, data []byte, flags MsgFlag, timeoutMS uint32) error {
	if len(data) == 0 {
		return checkErr(C.canWriteWait(C.CanHandle(h), C.long(identifier), nil, 0, C.uint(flags), C.ulong(timeoutMS)))
	}
	cb, pooled := getCBuf(len(data))
	dst := unsafe.Slice((*byte)(cb.ptr), len(data))
	copy(dst, data)
	err := checkErr(C.canWriteWait(C.CanHandle(h), C.long(identifier), cb.ptr, C.uint(len(data)), C.uint(flags), C.ulong(timeoutMS)))
	putCBuf(cb, pooled)
	return err
}

func GetErrorText(status int) (string, error) {
	buf := make([]byte, 64)
	r := C.canGetErrorText(C.canStatus(status), (*C.char)(unsafe.Pointer(&buf[0])), C.uint(len(buf)))
	if int32(r) < int32(ERR_OK) {
		return "", fmt.Errorf("unable to get description for error code %v (%v)", status, int32(r))
	}
	return cBytetoString(buf), nil
}

func cBytetoString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

func checkErr(r C.canStatus) error {
	return NewError(int32(r))
}
