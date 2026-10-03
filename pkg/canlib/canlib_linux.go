package canlib

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// libcanlib is loaded through purego, so no cgo or Kvaser headers are
// needed at build time. C long / unsigned long map to Go int / uint, which
// match their width on every Linux ABI Go supports.
//
// The per-frame calls are raw symbol addresses called through SyscallN:
// a RegisterFunc'd func goes through reflect on every call, which costs
// allocations per frame. The rest stay typed for readability.
var (
	procRead, procReadWait, procWrite, procWriteSync uintptr

	InitErr  error
	initOnce sync.Once

	canInitializeLibrary   func()
	canUnloadLibrary       func() int32
	canGetNumberOfChannels func(n *int32) int32
	canGetChannelData      func(channel, item int32, buf unsafe.Pointer, size uintptr) int32
	canGetErrorText        func(status int32, buf *byte, size uint32) int32
	canOpenChannel         func(channel, flags int32) int32
	canGetVersion          func() uint16
	canAccept              func(h Handle, envelope int, flag uint32) int32
	canClose               func(h Handle) int32
	canBusOn               func(h Handle) int32
	canBusOff              func(h Handle) int32
	canFlushReceiveQueue   func(h Handle) int32
	canFlushTransmitQueue  func(h Handle) int32
	canObjBufAllocate      func(h Handle, typ int32) int32
	canObjBufWrite         func(h Handle, idx, id int32, msg unsafe.Pointer, dlc, flags uint32) int32
	canResetBus            func(h Handle) int32
	canSetAcceptanceFilter func(h Handle, code, mask uint32, extended int32) int32
	canSetBusParams        func(h Handle, freq int, tseg1, tseg2, sjw, noSamp, syncmode uint32) int32
	canSetBusParamsC200    func(h Handle, btr0, btr1 uint8) int32
	canSetBusOutputControl func(h Handle, drivertype uint32) int32
	canReadErrorCounters   func(h Handle, tx, rx, overrun *uint32) int32
	canWriteWait           func(h Handle, id int, msg unsafe.Pointer, dlc, flags uint32, timeout uint) int32
)

func Init() error {
	initOnce.Do(func() {
		lib, err := purego.Dlopen("libcanlib.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			InitErr = err
			return
		}
		for _, s := range []struct {
			name string
			fptr any
		}{
			{"canInitializeLibrary", &canInitializeLibrary},
			{"canUnloadLibrary", &canUnloadLibrary},
			{"canGetNumberOfChannels", &canGetNumberOfChannels},
			{"canGetChannelData", &canGetChannelData},
			{"canGetErrorText", &canGetErrorText},
			{"canOpenChannel", &canOpenChannel},
			{"canGetVersion", &canGetVersion},
			{"canAccept", &canAccept},
			{"canClose", &canClose},
			{"canBusOn", &canBusOn},
			{"canBusOff", &canBusOff},
			{"canFlushReceiveQueue", &canFlushReceiveQueue},
			{"canFlushTransmitQueue", &canFlushTransmitQueue},
			{"canObjBufAllocate", &canObjBufAllocate},
			{"canObjBufWrite", &canObjBufWrite},
			{"canResetBus", &canResetBus},
			{"canSetAcceptanceFilter", &canSetAcceptanceFilter},
			{"canSetBusParams", &canSetBusParams},
			{"canSetBusParamsC200", &canSetBusParamsC200},
			{"canSetBusOutputControl", &canSetBusOutputControl},
			{"canReadErrorCounters", &canReadErrorCounters},
			{"canRead", &procRead},
			{"canReadWait", &procReadWait},
			{"canWrite", &procWrite},
			{"canWriteSync", &procWriteSync},
			{"canWriteWait", &canWriteWait},
		} {
			sym, err := purego.Dlsym(lib, s.name)
			if err != nil {
				InitErr = fmt.Errorf("failed to find procedure %s: %w", s.name, err)
				purego.Dlclose(lib)
				return
			}
			if p, ok := s.fptr.(*uintptr); ok {
				*p = sym
			} else {
				purego.RegisterFunc(s.fptr, sym)
			}
		}
		canInitializeLibrary()
	})
	return InitErr
}

// Handle is a handle to a CAN channel (circuit).
type Handle int32

// CANMessage is filled in place by Read/ReadWait; Identifier and Timestamp
// have the width of the C longs the driver writes into them.
type CANMessage struct {
	Identifier int
	Timestamp  uint
	DLC        uint32
	Flags      uint32
	Data       [64]byte
}

func InitializeLibrary() error {
	canInitializeLibrary()
	return nil
}

func UnloadLibrary() error {
	return NewError(canUnloadLibrary())
}

func GetNumberOfChannels() (int, error) {
	var n int32
	r := canGetNumberOfChannels(&n)
	return int(n), NewError(r)
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
	r := canGetChannelData(int32(channel), int32(item), unsafe.Pointer(&data[0]), uintptr(len(data)))
	return data, NewError(r)
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
	r := canOpenChannel(int32(channel), int32(flags))
	return Handle(r), NewError(r)
}

func GetVersion() string {
	r := canGetVersion()
	return fmt.Sprintf("%d.%d", r>>8, r&0xFF)
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
	return NewError(canAccept(h, envelope, uint32(flag)))
}

func (h Handle) Close() error {
	return NewError(canClose(h))
}

func (h Handle) BusOn() error {
	return NewError(canBusOn(h))
}

func (h Handle) BusOff() error {
	return NewError(canBusOff(h))
}

func (h Handle) FlushReceiveQueue() error {
	return NewError(canFlushReceiveQueue(h))
}

func (h Handle) FlushTransmitQueue() error {
	return NewError(canFlushTransmitQueue(h))
}

func (h Handle) ObjBufAllocate(typ int) (int, error) {
	r := canObjBufAllocate(h, int32(typ))
	return int(r), NewError(r)
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
	return NewError(canObjBufWrite(h, int32(idx), int32(id), dataPtr(message), uint32(len(message)), uint32(flags)))
}

func (h Handle) ResetBus() error {
	return NewError(canResetBus(h))
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
	var ext int32
	if extended {
		ext = 1
	}
	return NewError(canSetAcceptanceFilter(h, uint32(code), uint32(mask), ext))
}

// SetBitrate sets a custom bit rate. Linux canlib does not export
// canSetBitrate, so we fall back to canSetBusParams with conservative
// default bit-timing values that work for most standard rates. For
// non-standard rates that require precise timing, call SetBusParams
// directly with the desired tseg1/tseg2/sjw/noSamp values.
func (h Handle) SetBitrate(bitrate int) error {
	return NewError(canSetBusParams(h, bitrate, 4, 3, 1, 1, 0))
}

func (h Handle) SetBusParams(freq BusParamsFreq, tseg1, tseg2, sjw, noSamp, syncmode uint32) error {
	return NewError(canSetBusParams(h, int(freq), tseg1, tseg2, sjw, noSamp, syncmode))
}

func (h Handle) SetBusParamsC200(btr0, btr1 uint8) error {
	return NewError(canSetBusParamsC200(h, btr0, btr1))
}

type DriverType uint32

const (
	DRIVER_OFF           DriverType = 0x00
	DRIVER_SILENT        DriverType = 0x01
	DRIVER_NORMAL        DriverType = 0x04
	DRIVER_SELFRECEPTION DriverType = 0x08
)

func SetBusOutputControl(h Handle, drivertype DriverType) error {
	return NewError(canSetBusOutputControl(h, uint32(drivertype)))
}

func (h Handle) ReadErrorCounters() (uint32, uint32, uint32, error) {
	var tx, rx, overrun uint32
	r := canReadErrorCounters(h, &tx, &rx, &overrun)
	return tx, rx, overrun, NewError(r)
}

// Read fetches the next queued frame into msg. msg must outlive the call
// on the heap (e.g. a struct field) or every call allocates it there.
func (h Handle) Read(msg *CANMessage) error {
	r, _, _ := purego.SyscallN(procRead, uintptr(h), uintptr(unsafe.Pointer(&msg.Identifier)), uintptr(unsafe.Pointer(&msg.Data)),
		uintptr(unsafe.Pointer(&msg.DLC)), uintptr(unsafe.Pointer(&msg.Flags)), uintptr(unsafe.Pointer(&msg.Timestamp)))
	return NewError(int32(r))
}

// ReadWait is Read, waiting up to timeout ms for a frame.
func (h Handle) ReadWait(msg *CANMessage, timeout uint32) error {
	r, _, _ := purego.SyscallN(procReadWait, uintptr(h), uintptr(unsafe.Pointer(&msg.Identifier)), uintptr(unsafe.Pointer(&msg.Data)),
		uintptr(unsafe.Pointer(&msg.DLC)), uintptr(unsafe.Pointer(&msg.Flags)), uintptr(unsafe.Pointer(&msg.Timestamp)), uintptr(timeout))
	return NewError(int32(r))
}

// Write queues a frame. data escapes to the heap, so pass a buffer that
// already lives there to keep the call allocation free.
func (h Handle) Write(identifier uint32, data []byte, flags MsgFlag) error {
	r, _, _ := purego.SyscallN(procWrite, uintptr(h), uintptr(identifier), uintptr(dataPtr(data)), uintptr(len(data)), uintptr(flags))
	return NewError(int32(r))
}

func (h Handle) WriteSync(timeoutMS uint32) error {
	r, _, _ := purego.SyscallN(procWriteSync, uintptr(h), uintptr(timeoutMS))
	return NewError(int32(r))
}

func (h Handle) WriteWait(identifier uint32, data []byte, flags MsgFlag, timeoutMS uint32) error {
	return NewError(canWriteWait(h, int(identifier), dataPtr(data), uint32(len(data)), uint32(flags), uint(timeoutMS)))
}

// dataPtr returns nil for an empty frame instead of panicking on &data[0].
// Go memory is fine to hand over: canlib copies the frame before returning.
func dataPtr(data []byte) unsafe.Pointer {
	if len(data) == 0 {
		return nil
	}
	return unsafe.Pointer(&data[0])
}

func GetErrorText(status int) (string, error) {
	buf := make([]byte, 64)
	r := canGetErrorText(int32(status), &buf[0], uint32(len(buf)))
	if r < int32(ERR_OK) {
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
