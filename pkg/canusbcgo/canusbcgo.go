package canusbcgo

// #include <lawicel_can.h>
// extern void goReceiveCallback(CANMsg *pMsg);
import "C"

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"unsafe"
)

var (
	callbackChan chan *CANMsg
	callbackMu   sync.Mutex
)

// Open CAN interface to device
//
// Returs handle to device if open was successfull or zero
// or negative error code on falure.
//
// szID
// ====
// Serial number for adapter or NULL to open the first found.
//
// szBitrate
// =========
// "10" for 10kbps
// "20" for 20kbps
// "50" for 50kbps
// "100" for 100kbps
// "250" for 250kbps
// "500" for 500kbps
// "800" for 800kbps
// "1000" for 1Mbps
//
// or
//
// btr0:btr1 pair  ex. "0x03:0x1c" or 3:28
//
// acceptance_code
// ===============
// Set to CANUSB_ACCEPTANCE_CODE_ALL to  get all messages.
//
// acceptance_mask
// ===============
// Set to CANUSB_ACCEPTANCE_MASk_ALL to  get all messages.
//
// flags
// =====
// CANUSB_FLAG_TIMESTAMP - Timestamp will be set by adapter.
func Open(szAdapter, bitrate string, code, mask uint32, flags OpenFlag) (*CANHANDLE, error) {
	cAdapter := C.CString(szAdapter)
	defer C.free(unsafe.Pointer(cAdapter))

	cBitrate := C.CString(bitrate)
	defer C.free(unsafe.Pointer(cBitrate))

	ret := C.canusb_Open(
		cAdapter,
		cBitrate,
		C.uint(code),
		C.uint(mask),
		C.uint(flags),
	)
	return &CANHANDLE{h: int(ret)}, NewError(ret)
}

// Close channel with handle h.
func (ch *CANHANDLE) Close() error {
	err := C.canusb_Close(C.CANHANDLE(ch.h))
	if err < 0 {
		return &Error{ErrorCode(err), "Close failed"}
	}
	return nil
}

// Read message from channel with handle h.
func (ch *CANHANDLE) Read() (*CANMsg, error) {
	cmsg := new(C.CANMsg)
	ret := C.canusb_Read(C.CANHANDLE(ch.h), cmsg)
	if ret <= 0 {
		return nil, NewError(ret)
	}
	msg := cmsgToGo(cmsg)
	return msg, nil
}

// Read message from channel with handle h and id "id" which satisfy flags.
func (ch *CANHANDLE) ReadFirst(id uint32, flags MessageFlag) (*CANMsg, error) {
	cmsg := new(C.CANMsg)
	ret := C.canusb_ReadFirst(C.CANHANDLE(ch.h), C.uint(id), C.uint(flags), cmsg)
	if ret <= 0 {
		return nil, NewError(ret)
	}
	return cmsgToGo(cmsg), nil
}

func cmsgToGo(cmsg *C.CANMsg) *CANMsg {
	msg := &CANMsg{
		Id:        uint32(cmsg.id),
		Timestamp: uint32(cmsg.timestamp),
		Flags:     uint8(cmsg.flags),
		Len:       uint8(cmsg.len),
	}
	copy(msg.Data[:], C.GoBytes(unsafe.Pointer(&cmsg.data), 8)) // Copy data bytes
	return msg
}

// Write message to channel with handle h.
func (ch *CANHANDLE) Write(msg *CANMsg) error {
	cmsg := &C.CANMsg{
		id:        C.uint(msg.Id),
		timestamp: C.uint(msg.Timestamp),
		flags:     C.uchar(msg.Flags),
		len:       C.uchar(msg.Len),
		data:      *(*[8]C.uchar)(unsafe.Pointer(&msg.Data[0])),
	}
	return NewError(C.canusb_Write(C.CANHANDLE(ch.h), cmsg))
}

// Get Adaper status for channel with handle h.
func (ch *CANHANDLE) Status() (err error) {
	status := C.canusb_Status(C.CANHANDLE(ch.h))
	if status == 0 {
		return
	}
	var errs []error
	if err = NewError(status); err != nil {
		return err
	}
	if status&CANSTATUS_RECEIVE_FIFO_FULL != 0 {
		errs = append(errs, errors.New("receive fifo full"))
	}
	if status&CANSTATUS_TRANSMIT_FIFO_FULL != 0 {
		errs = append(errs, errors.New("transmit fifo full"))
	}
	if status&CANSTATUS_ERROR_WARNING != 0 {
		errs = append(errs, errors.New("error warning"))
	}
	if status&CANSTATUS_DATA_OVERRUN != 0 {
		errs = append(errs, errors.New("data overrun"))
	}
	if status&CANSTATUS_ERROR_PASSIVE != 0 {
		errs = append(errs, errors.New("error passive"))
	}
	if status&CANSTATUS_ARBITRATION_LOST != 0 {
		errs = append(errs, errors.New("arbitration lost"))
	}
	if status&CANSTATUS_BUS_ERROR != 0 {
		errs = append(errs, errors.New("bus error"))
	}

	return fmt.Errorf("status: %v", errs)
}

// Get hardware/firmware and driver version for channel with handle h.
func (ch *CANHANDLE) VersionInfo() (string, error) {
	var szVersion [64]C.char
	ret := C.canusb_VersionInfo(C.CANHANDLE(ch.h), &szVersion[0])
	return C.GoString(&szVersion[0]), NewError(ret)
}

// Flush output buffer on channel with handle h.
//
// If flushflags is set to FLUSH_DONTWAIT the queue is just emptied and there will be no wait for any frames in it to be sent
func (ch *CANHANDLE) Flush(flags FlushFlag) error {
	return NewError(C.canusb_Flush(C.CANHANDLE(ch.h), C.uchar(flags)))
}

// Get transmission statistics for channel with handle h.
func (ch *CANHANDLE) GetStatistics() (CANUsbStatistics, error) {
	stat := new(C.CANUsbStatistics)
	ret := C.canusb_GetStatistics(C.CANHANDLE(ch.h), stat)
	return CANUsbStatistics{
		ReceiveFrames:  uint32(stat.cntReceiveFrames),
		TransmitFrames: uint32(stat.cntTransmitFrames),
		ReceiveData:    uint32(stat.cntReceiveData),
		TransmitData:   uint32(stat.cntTransmitData),
		Overruns:       uint32(stat.cntOverruns),
		BusWarnings:    uint32(stat.cntBusWarnings),
		BusOff:         uint32(stat.cntBusOff),
	}, NewError(ret)
}

// Set timeouts used for blocking calls for channel with handle h.
func (ch *CANHANDLE) SetTimeout(receiveTimeout, sendTimeout uint32) error {
	return NewError(C.canusb_SetTimeouts(C.CANHANDLE(ch.h), C.uint(receiveTimeout), C.uint(sendTimeout)))
}

//export goReceiveCallback
func goReceiveCallback(msg *C.CANMsg) {
	runtime.LockOSThread()
	// Make a copy of the message immediately to avoid issues with C memory
	goMsg := cmsgToGo(msg)

	// Use a non-blocking send with logging
	if callbackChan != nil {
		select {
		case callbackChan <- goMsg:
		default:
			log.Println("callbackChan full, dropping frame")
		}
	}
}

// Set a receive call back function. Set the callback to NULL to reset it.
func (ch CANHANDLE) SetReceiveCallBack(cc chan *CANMsg) error {
	callbackMu.Lock()
	callbackChan = cc
	callbackMu.Unlock()

	if cc == nil {
		return NewError(C.canusb_setReceiveCallBack(C.CANHANDLE(ch.h), nil))
	}
	return NewError(C.canusb_setReceiveCallBack(C.CANHANDLE(ch.h), C.LPFNDLL_RECEIVE_CALLBACK(C.goReceiveCallback)))
}

func GetAdapters() (adapters []string, err error) {
	noAdapters, sz, err := getFirstAdapter()
	if err != nil {
		return
	}
	adapters = append(adapters, sz)
	if noAdapters > 1 {
		for i := 1; i < noAdapters; i++ {
			noAdapters, sz, err = getNextAdapter()
			if err != nil {
				return
			}
			adapters = append(adapters, sz)
		}
	}
	return
}

// Get the first found adapter that is connected to this machine.
// Returns <= 0 on failure. 0 if no adapter found. >0 if one or more adapters
// is found.
func getFirstAdapter() (int, string, error) {
	szAdapter := make([]C.char, 10)
	res := C.canusb_getFirstAdapter(&szAdapter[0], 10)
	return int(res), C.GoString(&szAdapter[0]), NewError(res)
}

// Get the found adapter(s) in turn that is connected to this machine.
// Returns <= 0 on failure.  >0 for a valid adapter return.
func getNextAdapter() (int, string, error) {
	szAdapter := make([]C.char, 10)
	res := C.canusb_getNextAdapter(&szAdapter[0], 10)
	return int(res), C.GoString(&szAdapter[0]), NewError(res)
}
