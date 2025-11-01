package canusb

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

var InitErr error

var (
	dllFuncs = map[string]**syscall.Proc{
		"canusb_Open":               &procOpen,
		"canusb_Close":              &procClose,
		"canusb_Read":               &procRead,
		"canusb_ReadEx":             &procReadEx,
		"canusb_ReadFirst":          &procReadFirst,
		"canusb_Write":              &procWrite,
		"canusb_WriteEx":            &procWriteEx,
		"canusb_Status":             &procStatus,
		"canusb_VersionInfo":        &procVersionInfo,
		"canusb_Flush":              &procFlush,
		"canusb_GetStatistics":      &procGetStatistics,
		"canusb_SetTimeouts":        &procSetTimeout,
		"canusb_setReceiveCallBack": &procSetReceiveCallBack,
		"canusb_getFirstAdapter":    &procGetFirstAdapter,
		"canusb_getNextAdapter":     &procGetNextAdapter,
	}
	procOpen               *syscall.Proc
	procClose              *syscall.Proc
	procRead               *syscall.Proc
	procReadEx             *syscall.Proc
	procReadFirst          *syscall.Proc
	procWrite              *syscall.Proc
	procWriteEx            *syscall.Proc
	procStatus             *syscall.Proc
	procVersionInfo        *syscall.Proc
	procFlush              *syscall.Proc
	procGetStatistics      *syscall.Proc
	procSetTimeout         *syscall.Proc
	procSetReceiveCallBack *syscall.Proc
	procGetFirstAdapter    *syscall.Proc
	procGetNextAdapter     *syscall.Proc
)

func init() {
	canusb, err := syscall.LoadDLL(dllName)
	if err != nil {
		InitErr = err
		return
	}

	for funcName, procPtr := range dllFuncs {
		*procPtr, err = canusb.FindProc(funcName)
		if err != nil {
			InitErr = fmt.Errorf("failed to find procedure %s: %w", funcName, err)
			return
		}
	}

}

// Open CAN interface to device
//
// Returs handle to device if open was successfull or zero
// or negative error code on falure.
//
// szID
//
//	Serial number for adapter or emptry string to open the first found.
//
// szBitrate
//
//	"10" for 10kbps
//	"20" for 20kbps
//	"50" for 50kbps
//	"100" for 100kbps
//	"250" for 250kbps
//	"500" for 500kbps
//	"800" for 800kbps
//	"1000" for 1Mbps
//
// or
//
//	btr0:btr1 pair  ex. "0x03:0x1c" or 3:28
//
// acceptance_code
//
// Set to ACCEPTANCE_CODE_ALL to  get all messages.
//
// acceptance_mask
//
// Set to ACCEPTANCE_MASK_ALL to  get all messages.
//
// flags
//
//	FLAG_TIMESTAMP - Timestamp will be set by adapter.
//	FLAG_QUEUE_REPLACE - If input queue is full remove oldest message and insert new message.
//	FLAG_BLOCK - Block receive/transmit
//	FLAG_SLOW - Check ACK/NACK's
//	FLAG_NO_LOCAL_SEND - Don't send transmited frames on other local channels for the same interface
func Open(szID, szBitrate string, code, mask uint32, flags OpenFlag) (*CANHANDLE, error) {
	cAdapter := make([]byte, 10)
	cBitrate := make([]byte, 10)
	copy(cAdapter, []byte(szID))
	copy(cBitrate, []byte(szBitrate))
	if szID == "" {
		r1, _, _ := procOpen.Call(uintptr(0), uintptr(unsafe.Pointer(&cBitrate[0])), uintptr(code), uintptr(mask), uintptr(flags))
		if int32(r1) == 0 {
			return nil, ErrNoDeviceAvailable
		}
		return &CANHANDLE{h: int32(r1)}, NewError(int32(r1))
	}
	r1, _, _ := procOpen.Call(uintptr(unsafe.Pointer(&cAdapter[0])), uintptr(unsafe.Pointer(&cBitrate[0])), uintptr(code), uintptr(mask), uintptr(flags))
	if int32(r1) == 0 {
		return nil, ErrNoDeviceAvailable
	}
	return &CANHANDLE{h: int32(r1)}, NewError(int32(r1))
}

// Close channel
func (ch *CANHANDLE) Close() error {
	defer func() {
		ch.h = -1
	}()
	return checkErr(procClose.Call(uintptr(ch.h)))
}

// Read message from channel
func (ch *CANHANDLE) Read() (msg *CANMsg, err error) {
	msg = new(CANMsg)
	r1, _, _ := procRead.Call(uintptr(ch.h), uintptr(unsafe.Pointer(msg)))
	err = NewError(int32(r1))
	return
}

// Read message from channel
//
// This is a version without a data-array in the structure
func (ch *CANHANDLE) ReadEx() (msg *CANMsgEx, data []byte, err error) {
	msg = new(CANMsgEx)
	data = make([]byte, 8)
	r1, _, _ := procReadEx.Call(uintptr(ch.h), uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(&data[0])))
	err = NewError(int32(r1))
	if err != nil {
		return
	}
	data = data[:msg.Len]
	return
}

// Read message with id which satisfy flags.
func (ch *CANHANDLE) ReadFirst(id uint32, flags MessageFlag) (msg *CANMsg, err error) {
	msg = new(CANMsg)
	r1, _, _ := procReadFirst.Call(uintptr(ch.h), uintptr(id), uintptr(flags), uintptr(unsafe.Pointer(msg)))
	err = NewError(int32(r1))
	return
}

// Write message to channel
func (ch *CANHANDLE) Write(msg *CANMsg) error {
	if msg.Len > 8 {
		return ErrMessageDataToLarge
	}
	return checkErr(procWrite.Call(uintptr(ch.h), uintptr(unsafe.Pointer(msg))))
}

// Write message to channel with handle h.
//
// This is a version without a data-array in the structure
func (ch *CANHANDLE) WriteEx(msg *CANMsgEx, data []byte) error {
	dataLen := len(data)
	msgLen := int(msg.Len)
	switch {
	case dataLen > 8 || dataLen > msgLen:
		return ErrMessageDataToLarge
	case dataLen < msgLen:
		return ErrMessageDataToSmall
	case dataLen != msgLen:
		return ErrMessageDataSize
	}
	return checkErr(procWriteEx.Call(uintptr(ch.h), uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(&data[0]))))
}

// Get Adaper status for channel
func (ch *CANHANDLE) Status() error {
	r1, _, _ := procStatus.Call(uintptr(ch.h))
	if r1 == 0 {
		return nil
	}
	status := int32(r1)
	if err := NewError(status); err != nil {
		return err
	}
	if status&CANSTATUS_RECEIVE_FIFO_FULL != 0 {
		return ErrReceiveFifoFull
	}
	if status&CANSTATUS_TRANSMIT_FIFO_FULL != 0 {
		return ErrTransmitFifoFull
	}
	if status&CANSTATUS_ERROR_WARNING != 0 {
		return ErrWarning
	}
	if status&CANSTATUS_DATA_OVERRUN != 0 {
		return ErrDataOverrun
	}
	if status&CANSTATUS_ERROR_PASSIVE != 0 {
		return ErrErrorPassive
	}
	if status&CANSTATUS_ARBITRATION_LOST != 0 {
		return ErrArbitrationLost
	}
	if status&CANSTATUS_BUS_ERROR != 0 {
		return ErrBussError
	}
	return &Error{ErrorCode(status), "Unknown"}
}

// Get hardware/firmware and driver version for channel
func (ch *CANHANDLE) VersionInfo() (string, error) {
	data := make([]byte, 64)
	r1, _, _ := procVersionInfo.Call(uintptr(ch.h), uintptr(unsafe.Pointer(&data[0])))
	return cStringtoString(data), NewError(int32(r1))
}

// Flush output buffer on channel
//
// If flushflags is set to FLUSH_DONTWAIT the queue is just emptied and there will be no wait for any frames in it to be sent
func (ch *CANHANDLE) Flush(flags FlushFlag) error {
	return checkErr(procFlush.Call(uintptr(ch.h), uintptr(flags)))
}

// Get statistics for channel
func (ch *CANHANDLE) GetStatistics() (*CANUSBStatistics, error) {
	stat := new(CANUSBStatistics)
	r1, _, _ := procGetStatistics.Call(uintptr(ch.h), uintptr(unsafe.Pointer(stat)))
	return stat, NewError(int32(r1))
}

// Set timeouts used for blocking calls for channel.
func (ch *CANHANDLE) SetTimeouts(receiveTimeout, sendTimeout uint32) error {
	return checkErr(procSetTimeout.Call(uintptr(ch.h), uintptr(receiveTimeout), uintptr(sendTimeout)))
}

// Set a receive callback function. Set the callback to nil to reset it.
//
// The callback will be called in a separate goroutine using a buffered channel to prevent blocking the device.
func (ch *CANHANDLE) SetReceiveCallback(fn CallbackFunc) error {
	if fn == nil {
		return checkErr(procSetReceiveCallBack.Call(uintptr(ch.h), 0))
	}
	return checkErr(procSetReceiveCallBack.Call(uintptr(ch.h), syscall.NewCallback(createWrapper(fn))))
}

// Set a receive callback function. Set the callback to nil to reset it.
// the callback will be called in a separate go routine to prevent blocking the receive loop if your callback takes long to execute.
// this will allow the receive loop to continue receiving messages while the callback is processing the message.
func (ch *CANHANDLE) SetAsyncReceiveCallback(fn CallbackFunc) error {
	if fn == nil {
		return checkErr(procSetReceiveCallBack.Call(uintptr(ch.h), 0))
	}
	return checkErr(procSetReceiveCallBack.Call(uintptr(ch.h), syscall.NewCallback(createAsyncWrapper(fn))))
}

func createWrapper(fn CallbackFunc) func(cbmsg *CANMsg) uintptr {
	return func(canMsg *CANMsg) uintptr {
		msg := &CANMsg{
			ID:        canMsg.ID,
			Timestamp: canMsg.Timestamp,
			Flags:     canMsg.Flags,
			Len:       canMsg.Len,
		}
		copy(msg.Data[:], canMsg.Data[:])
		return fn(msg)
	}
}

func createAsyncWrapper(fn CallbackFunc) func(cbmsg *CANMsg) uintptr {
	asyncMsg := make(chan *CANMsg, 128)
	go func() {
		for {
			for msg := range asyncMsg {
				fn(msg)
			}
		}
	}()
	return func(canMsg *CANMsg) uintptr {
		msg := &CANMsg{
			ID:        canMsg.ID,
			Timestamp: canMsg.Timestamp,
			Flags:     canMsg.Flags,
			Len:       canMsg.Len,
		}
		copy(msg.Data[:], canMsg.Data[:])
		select {
		case asyncMsg <- msg:
		default:
			// Drop message if channel is full
			log.Printf("async handler channel full, dropped message %3X", msg.ID)
		}
		return 1
	}
}

// Get all found adapters that is connected to this machine.
func GetAdapters() (adapters []string, err error) {
	noAdapters, szAdapter, err := GetFirstAdapter()
	if err != nil {
		return
	}
	adapters = append(adapters, szAdapter)
	if noAdapters > 1 {
		for i := 1; i < noAdapters; i++ {
			szAdapter, err = GetNextAdapter()
			if err != nil {
				return
			}
			adapters = append(adapters, szAdapter)
		}
	}
	return
}

// Get the first found adapter that is connected to this machine.
//
// Returns the number of adapters found and the serial number of the first adapter.
func GetFirstAdapter() (int, string, error) {
	data := make([]byte, 10)
	r1, _, _ := procGetFirstAdapter.Call(uintptr(unsafe.Pointer(&data[0])), 10)
	return int(r1), cStringtoString(data), NewError(int32(r1))
}

// Get the found adapter(s) in turn that is connected to this machine.
//
// Returns the serial number of the next adapter.
func GetNextAdapter() (string, error) {
	data := make([]byte, 10)
	r1, _, _ := procGetNextAdapter.Call(uintptr(unsafe.Pointer(&data[0])), 10)
	return cStringtoString(data), NewError(int32(r1))
}

func cStringtoString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}
