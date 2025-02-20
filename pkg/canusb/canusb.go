package canusb

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procOpen               = canusbdrv.NewProc("canusb_Open")
	procClose              = canusbdrv.NewProc("canusb_Close")
	procRead               = canusbdrv.NewProc("canusb_Read")
	procReadFirst          = canusbdrv.NewProc("canusb_ReadFirst")
	procWrite              = canusbdrv.NewProc("canusb_Write")
	procStatus             = canusbdrv.NewProc("canusb_Status")
	procVersionInfo        = canusbdrv.NewProc("canusb_VersionInfo")
	procFlush              = canusbdrv.NewProc("canusb_Flush")
	procGetStatistics      = canusbdrv.NewProc("canusb_GetStatistics")
	procSetTimeout         = canusbdrv.NewProc("canusb_SetTimeout")
	procSetReceiveCallBack = canusbdrv.NewProc("canusb_setReceiveCallBack")
	procGetFirstAdapter    = canusbdrv.NewProc("canusb_getFirstAdapter")
	procGetNextAdapter     = canusbdrv.NewProc("canusb_getNextAdapter")
)

func errWrap(r1, _ uintptr, _ error) error {
	return NewError(int(r1))
}

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
func Open(szID, szBitrate string, code, mask uint32, flags OpenFlag) (*CANHANDLE, error) {
	cAdapter := make([]byte, 10)
	cBitrate := make([]byte, 10)
	copy(cAdapter, []byte(szID))
	copy(cBitrate, []byte(szBitrate))
	// log.Printf("sz: %s, bt: %s, code: %X, mask: %X, flags: %X", szAdapter, bitrate, code, mask, flags)
	r1, _, _ := procOpen.Call(uintptr(unsafe.Pointer(&cAdapter[0])), uintptr(unsafe.Pointer(&cBitrate[0])), uintptr(code), uintptr(mask), uintptr(flags))
	return &CANHANDLE{h: int(r1)}, NewError(int(r1))
}

// Close channel
func (ch *CANHANDLE) Close() error {
	defer func() {
		ch.h = -1
	}()
	return errWrap(procClose.Call(uintptr(ch.h)))
}

// Read message from channel
func (ch *CANHANDLE) Read() (msg *CANMsg, err error) {
	r1, _, _ := procRead.Call(uintptr(ch.h), uintptr(unsafe.Pointer(msg)))
	err = NewError(int(r1))
	return
}

// Read message with id which satisfy flags.
func (ch *CANHANDLE) ReadFirst(id uint32, flags MessageFlag) (msg *CANMsg, err error) {
	r1, _, _ := procReadFirst.Call(uintptr(ch.h), uintptr(id), uintptr(flags), uintptr(unsafe.Pointer(msg)))
	err = NewError(int(r1))
	return
}

// Write message to channel
func (ch *CANHANDLE) Write(msg *CANMsg) error {
	return errWrap(procWrite.Call(uintptr(ch.h), uintptr(unsafe.Pointer(msg))))
}

// Get Adaper status for channel
func (ch *CANHANDLE) Status() error {
	r1, _, _ := procStatus.Call(uintptr(ch.h))
	if r1 == 0 || r1 == CANSTATUS_ARBITRATION_LOST {
		return nil

	}
	status := int(r1)
	if err := NewError(status); err != nil {
		return err
	}
	var errs []error
	if err := NewError(status); err != nil {
		return err
	}
	if status&CANSTATUS_RECEIVE_FIFO_FULL != 0 {
		errs = append(errs, errors.New("receive FIFO full"))
	}
	if status&CANSTATUS_TRANSMIT_FIFO_FULL != 0 {
		errs = append(errs, errors.New("transmit FIFO full"))
	}
	if status&CANSTATUS_ERROR_WARNING != 0 {
		errs = append(errs, errors.New("error warning (EI)"))
	}
	if status&CANSTATUS_DATA_OVERRUN != 0 {
		errs = append(errs, errors.New("data overrun (DOI)"))
	}
	if status&CANSTATUS_ERROR_PASSIVE != 0 {
		errs = append(errs, errors.New("error passive (EPI)"))
	}
	if status&CANSTATUS_ARBITRATION_LOST != 0 {
		errs = append(errs, errors.New("arbitration lost (ALI)"))
	}
	if status&CANSTATUS_BUS_ERROR != 0 {
		errs = append(errs, errors.New("bus error (BEI)"))
	}

	return fmt.Errorf("status (%X): %v", status, errs)
}

// Get hardware/firmware and driver version for channel
func (ch *CANHANDLE) VersionInfo() (string, error) {
	data := make([]byte, 64)
	r1, _, _ := procVersionInfo.Call(uintptr(ch.h), uintptr(unsafe.Pointer(&data[0])))
	return cBytetoString(data), NewError(int(r1))
}

// Flush output buffer on channel
//
// If flushflags is set to FLUSH_DONTWAIT the queue is just emptied and there will be no wait for any frames in it to be sent
func (ch *CANHANDLE) Flush(flags FlushFlag) error {
	return errWrap(procFlush.Call(uintptr(ch.h), uintptr(flags)))
}

func (ch *CANHANDLE) GetStatistics() (*CANUsbStatistics, error) {
	stat := new(CANUsbStatistics)
	r1, _, _ := procGetStatistics.Call(uintptr(ch.h), uintptr(unsafe.Pointer(stat)))
	return stat, NewError(int(r1))
}

// Set timeouts used for blocking calls for channel
func (ch *CANHANDLE) SetTimeout(receiveTimeout, sendTimeout uint32) error {
	return errWrap(procSetTimeout.Call(uintptr(ch.h), uintptr(receiveTimeout), uintptr(sendTimeout)))
}

// Set a receive call back function. Set the callback to nil to reset it.
func (ch *CANHANDLE) SetReceiveCallback(fn CallbackFunc) error {
	return errWrap(procSetReceiveCallBack.Call(uintptr(ch.h), syscall.NewCallback(fn)))
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
	return int(r1), cBytetoString(data), NewError(int(r1))
}

// Get the found adapter(s) in turn that is connected to this machine.
//
// Returns the serial number of the next adapter.
func GetNextAdapter() (string, error) {
	data := make([]byte, 10)
	r1, _, _ := procGetNextAdapter.Call(uintptr(unsafe.Pointer(&data[0])), 10)
	return cBytetoString(data), NewError(int(r1))
}

/*
func cSBytetoString(data []byte) string {
	index := bytes.IndexByte(data, 0)
	if index == -1 {
		return string(data)
	}
	return string(data[:index])
}
*/

func cBytetoString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}
