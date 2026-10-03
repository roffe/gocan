package ftdi

import (
	"errors"
	"io"
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
	ID           uint64 // VID<<16 | PID, as D2XX reports it
	SerialNumber string
	Description  string
	Manufacturer string
}

// GetDeviceList lists the FTDI devices libftdi knows by default PID
// (0x6001/6010/6011/6014/6015). Devices whose strings can't be read (no
// permission on the USB node) are skipped rather than failing the scan.
func GetDeviceList() (dl []DeviceInfo, e error) {
	ctx := C.ftdi_new()
	if ctx == nil {
		return nil, errors.New("Failed to create FTDI context")
	}
	defer C.ftdi_free(ctx)

	var list *C.struct_ftdi_device_list
	if num := C.ftdi_usb_find_all(ctx, &list, 0, 0); num < 0 {
		return nil, getErr(ctx)
	}
	defer C.ftdi_list_free(&list)

	for node := list; node != nil; node = node.next {
		const CHAR_SZ = 64
		var mnf_char, desc_char, ser_char [CHAR_SZ]C.char
		if C.ftdi_usb_get_strings(ctx, node.dev,
			(*C.char)(&mnf_char[0]), CHAR_SZ,
			(*C.char)(&desc_char[0]), CHAR_SZ,
			(*C.char)(&ser_char[0]), CHAR_SZ) != 0 {
			continue
		}
		var desc C.struct_libusb_device_descriptor
		if C.libusb_get_device_descriptor(node.dev, &desc) != 0 {
			continue
		}
		dl = append(dl, DeviceInfo{
			Index:        uint64(len(dl)),
			ID:           uint64(desc.idVendor)<<16 | uint64(desc.idProduct),
			Manufacturer: C.GoString(&mnf_char[0]),
			Description:  C.GoString(&desc_char[0]),
			SerialNumber: C.GoString(&ser_char[0]),
		})
	}
	return dl, nil
}

// Device serializes each data direction on its own lock: libftdi's read
// and write paths touch disjoint context state and libusb allows concurrent
// transfers, so a reader blocked in its ~latency-timer USB poll doesn't hold
// up writes (as D2XX on Windows doesn't). Everything else takes both.
type Device struct {
	ctx          *C.struct_ftdi_context
	open         bool
	rlock, wlock sync.Mutex
}

func (d *Device) lockAll() (unlock func()) {
	d.rlock.Lock()
	d.wlock.Lock()
	return func() { d.wlock.Unlock(); d.rlock.Unlock() }
}

// Open opens the FTDI device with di's serial number and product id pid.
// libftdi detaches the ftdi_sio kernel driver while the device is open (its
// /dev/ttyUSBn disappears); Close re-attaches it.
func Open(di DeviceInfo, pid int) (d *Device, e error) {
	ctx := C.ftdi_new()
	if ctx == nil {
		return d, errors.New("Failed to create FTDI context")
	}
	ctx.module_detach_mode = C.AUTO_DETACH_REATACH_SIO_MODULE

	cstr := C.CString(di.SerialNumber)
	defer C.free(unsafe.Pointer(cstr))

	if ret := C.ftdi_usb_open_desc(ctx, 0x0403, C.int(pid), nil, cstr); ret != 0 {
		err := getErr(ctx) // before ftdi_free releases the error string
		C.ftdi_free(ctx)
		return d, err
	}

	return &Device{ctx: ctx, open: true}, nil
}

func (d *Device) Close() (e error) {
	defer d.lockAll()()
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
	d.rlock.Lock()
	defer d.rlock.Unlock()
	if !d.open {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	// ftdi_read_data keeps polling until len(p) is filled or a status-only
	// packet arrives, so a short reply costs an extra latency-timer period.
	// Ask for one byte (one USB transfer), then take whatever that transfer
	// left in libftdi's buffer, which is served without touching USB.
	ret := C.ftdi_read_data(d.ctx, (*C.uchar)(&p[0]), 1)
	if ret <= 0 {
		if ret < 0 {
			return 0, getErr(d.ctx)
		}
		return 0, nil
	}
	if more := min(int(d.ctx.readbuffer_remaining), len(p)-1); more > 0 {
		if r := C.ftdi_read_data(d.ctx, (*C.uchar)(&p[1]), C.int(more)); r > 0 {
			ret += r
		}
	}
	return int(ret), nil
}

func (d *Device) Write(p []byte) (n int, e error) {
	d.wlock.Lock()
	defer d.wlock.Unlock()
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
	defer d.lockAll()()
	if ret := C.ftdi_set_baudrate(d.ctx, C.int(baud)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetChars(event, err byte) (e error) {
	defer d.lockAll()()
	if ret := C.ftdi_set_event_char(d.ctx, C.uchar(event), C.uchar(event)); ret < 0 {
		return getErr(d.ctx)
	}
	if ret := C.ftdi_set_error_char(d.ctx, C.uchar(err), C.uchar(err)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetBitMode(mode BitMode) (e error) {
	defer d.lockAll()()
	const mask = 0x00
	if ret := C.ftdi_set_bitmode(d.ctx, mask, C.uchar(mode)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetFlowControl(f FlowControl) (e error) {
	defer d.lockAll()()
	if ret := C.ftdi_setflowctrl(d.ctx, C.int(f)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetLatency(latency int) (e error) {
	defer d.lockAll()()
	if ret := C.ftdi_set_latency_timer(d.ctx, C.uchar(latency)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetTransferSize(read_size, write_size int) (e error) {
	defer d.lockAll()()
	if ret := C.ftdi_read_data_set_chunksize(d.ctx, C.uint(read_size)); ret < 0 {
		return getErr(d.ctx)
	}
	if ret := C.ftdi_write_data_set_chunksize(d.ctx, C.uint(write_size)); ret < 0 {
		return getErr(d.ctx)
	}
	return nil
}

func (d *Device) SetLineProperty(props LineProperties) (e error) {
	defer d.lockAll()()
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
	defer d.lockAll()()
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
	switch flags {
	case FT_PURGE_RX: // also resets libftdi's read buffer
		d.rlock.Lock()
		defer d.rlock.Unlock()
	case FT_PURGE_TX:
		d.wlock.Lock()
		defer d.wlock.Unlock()
	default:
		defer d.lockAll()()
	}

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
	defer d.lockAll()()
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
	defer d.lockAll()()
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
