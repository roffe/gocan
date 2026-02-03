package ftdi

import (
	"errors"
	"io"
	"log"
	"sync"
	"unsafe"
)

// #cgo pkg-config: libftdi1
// #include <ftdi.h>
// #include <libusb.h>
// #include <stdlib.h>
import "C"

func Init() error {
	return nil
}

// Return Library version, formatted to match D2XX
func GetLibraryVersion() uint32 {
	// v := C.ftdi_get_library_version()
	// return uint32(v.major&0xFF<<16 +
	// 	v.minor&0xFF<<8 +
	// 	v.micro&0xFF)
	return 0x1ffff
}

type DeviceInfo struct {
	Index        uint64
	Id           uint32 // used as interface number
	SerialNumber string
	Description  string
	Manufacturer string
	handle       unsafe.Pointer // the libusb device pointer
}

// TODO: Need to expand multi-interface devices, and then to other FTDI chips
func GetDeviceList() (dl []DeviceInfo, e error) {
	ctx := C.ftdi_new()
	if ctx == nil {
		return nil, errors.New("Failed to create FTDI context")
	}
	defer C.ftdi_free(ctx)

	var dev_list *C.struct_ftdi_device_list

	num := C.ftdi_usb_find_all(ctx, &dev_list, 0, 0)

	if num < 0 {
		return nil, getErr(ctx)
	}
	if num == 0 {
		return nil, nil
	}

	dl = make([]DeviceInfo, num)

	for i := 0; i < int(num); i++ {

		const CHAR_SZ = 64
		var mnf_char, desc_char, ser_char [CHAR_SZ]C.char

		ret := C.ftdi_usb_get_strings(ctx, dev_list.dev,
			(*C.char)(&mnf_char[0]), CHAR_SZ,
			(*C.char)(&desc_char[0]), CHAR_SZ,
			(*C.char)(&ser_char[0]), CHAR_SZ)
		if ret != 0 {
			return nil, getErr(ctx)
		}
		d := DeviceInfo{
			handle:       unsafe.Pointer(dev_list.dev),
			Index:        uint64(i),
			Manufacturer: C.GoString(&mnf_char[0]),
			Description:  C.GoString(&desc_char[0]),
			SerialNumber: C.GoString(&ser_char[0]),
		}
		dl[i] = d
		dev_list = dev_list.next
	}

	return dl, nil
}

type Device struct {
	ctx  *C.struct_ftdi_context
	open bool
	lock sync.Mutex
}

func Open(di DeviceInfo, pid int) (d *Device, e error) {
	ctx := C.ftdi_new()
	if ctx == nil {
		C.ftdi_free(ctx)
		return d, errors.New("Failed to create FTDI context")
	}

	if ret := C.ftdi_set_interface(ctx, di.Id); ret != 0 {
		C.ftdi_free(ctx)
		return d, getErr(ctx)
	}

	cstr := C.CString(di.SerialNumber)
	defer C.free(unsafe.Pointer(cstr))

	log.Printf("%q", di.SerialNumber)

	if ret := C.ftdi_usb_open_desc(ctx, 0x0403, C.int(pid), nil, cstr); ret != 0 {
		C.ftdi_free(ctx)
		return d, getErr(ctx)
	}

	return &Device{ctx, true, sync.Mutex{}}, nil
}

func (d *Device) Close() (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	defer C.ftdi_free(d.ctx)
	d.open = false
	if ret := C.ftdi_usb_close(d.ctx); ret != 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) GetStatus() (rx_queue, tx_queue, events int32, e error) {
	return 0, 0, 0, errors.New("Not Implemented")
}

func (d *Device) Read(p []byte) (n int, e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if !d.open {
		return 0, io.EOF
	}
	ret := C.ftdi_read_data(d.ctx, (*C.uchar)(&p[0]), C.int(len(p)))
	if ret < 0 {
		return 0, getErr(d.ctx)
	}
	return int(ret), nil
}

func (d *Device) Write(p []byte) (n int, e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if !d.open {
		return 0, errors.New("FTDI device is already closed")
	}
	ret := C.ftdi_write_data(d.ctx, (*C.uchar)(&p[0]), C.int(len(p)))
	if ret < 0 {
		return 0, getErr(d.ctx)
	}
	return int(ret), nil
}

func (d *Device) SetBaudRate(baud uint) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_set_baudrate(d.ctx, C.int(baud)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetChars(event, err byte) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_set_event_char(d.ctx, C.uchar(event), C.uchar(event)); ret < 0 {
		return getErr(d.ctx)
	}
	if ret := C.ftdi_set_error_char(d.ctx, C.uchar(err), C.uchar(err)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetBitMode(mode BitMode) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	const mask = 0x00
	if ret := C.ftdi_set_bitmode(d.ctx, mask, C.uchar(mode)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetFlowControl(f FlowControl) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_setflowctrl(d.ctx, C.int(f)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetLatency(latency int) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_set_latency_timer(d.ctx, C.uchar(latency)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetTransferSize(read_size, write_size int) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_read_data_set_chunksize(d.ctx, C.uint(read_size)); ret < 0 {
		return getErr(d.ctx)
	}
	if ret := C.ftdi_write_data_set_chunksize(d.ctx, C.uint(write_size)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetLineProperty(props LineProperties) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_set_line_property(d.ctx,
		uint32(props.Bits),
		uint32(props.StopBits),
		uint32(props.Parity)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetTimeout(read_timeout, write_timeout int) (e error) {
	// NOP
	return nil
}

func (d *Device) Reset() (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_usb_reset(d.ctx); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

type PurgeFlag uint8

const (
	FT_PURGE_RX   PurgeFlag = 0x01
	FT_PURGE_TX   PurgeFlag = 0x02
	FT_PURGE_BOTH PurgeFlag = FT_PURGE_RX | FT_PURGE_TX
)

func (d *Device) Purge(flags PurgeFlag) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	switch flags {
	case FT_PURGE_RX:
		if ret := C.ftdi_tciflush(d.ctx); ret < 0 {
			return getErr(d.ctx)
		}
	case FT_PURGE_TX:
		if ret := C.ftdi_tcoflush(d.ctx); ret < 0 {
			return getErr(d.ctx)
		}
	case FT_PURGE_BOTH:
		if ret := C.ftdi_tcioflush(d.ctx); ret < 0 {
			return getErr(d.ctx)
		}
	}

	return nil
}

func (d *Device) SetBreakOn(props LineProperties) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_set_line_property2(d.ctx,
		uint32(props.Bits),
		uint32(props.StopBits),
		uint32(props.Parity),
		uint32(1)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetBreakOff(props LineProperties) (e error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if ret := C.ftdi_set_line_property2(d.ctx,
		uint32(props.Bits),
		uint32(props.StopBits),
		uint32(props.Parity),
		uint32(0)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func getErr(ctx *C.struct_ftdi_context) error {
	return errors.New(C.GoString(C.ftdi_get_error_string(ctx)))
}
