//go:build rcan

package rcan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotmc/libusb/v2"
	gocan "github.com/roffe/gocan/v2"
)

const (
	usbVID = 0xffff
	usbPID = 0x1337

	usbInterfaceNum   = 0
	usbOutEndpointNum = 0x01
	usbInEndpointNum  = 0x81

	maxCommandSize = 512

	cmdCANFrame = 0x05
	cmdSetSpeed = 0x01
	cmdOpen     = 0x02
	cmdClose    = 0x03
)

const libusbErrTimeout = libusb.ErrorCode(-7) // LIBUSB_ERROR_TIMEOUT

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:         "rCAN",
		Description:  "CAN device by roffe.nu",
		Capabilities: gocan.Capabilities{HSCAN: true},
		New:          New,
	})
}

type RCan struct {
	cfg gocan.Config
	bus *gocan.Bus

	usbCtx *libusb.Context
	dev    *libusb.Device
	handle *libusb.DeviceHandle

	closeOnce sync.Once
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &RCan{cfg: cfg}, nil
}

func (r *RCan) Open(ctx context.Context, bus *gocan.Bus) error {
	r.bus = bus
	usbCtx, err := libusb.NewContext()
	if err != nil {
		return fmt.Errorf("libusb init: %w", err)
	}
	dev, handle, err := usbCtx.OpenDeviceWithVendorProduct(usbVID, usbPID)
	if err != nil {
		usbCtx.Close()
		return fmt.Errorf("rCAN device not found: %w", err)
	}
	_ = handle.SetAutoDetachKernelDriver(true) // no-op on Windows, helpful on Linux
	if err := handle.ClaimInterface(usbInterfaceNum); err != nil {
		handle.Close()
		dev.Close()
		usbCtx.Close()
		return fmt.Errorf("claim interface %d: %w", usbInterfaceNum, err)
	}
	r.usbCtx, r.dev, r.handle = usbCtx, dev, handle

	go r.readLoop(ctx)

	// close (reset), set speed, open — mirrors the v1 bring-up.
	if _, err := r.bulkOut([]byte{cmdClose}, 200); err != nil {
		r.closeUSB()
		return err
	}
	speed := []byte{cmdSetSpeed, 0, 0}
	binary.LittleEndian.PutUint16(speed[1:], uint16(r.cfg.CANRate))
	if _, err := r.bulkOut(speed, 200); err != nil {
		r.closeUSB()
		return err
	}
	if _, err := r.bulkOut([]byte{cmdOpen}, 200); err != nil {
		r.closeUSB()
		return err
	}
	return nil
}

func (r *RCan) Close() error {
	r.closeOnce.Do(func() {
		if r.handle != nil {
			r.bulkOut([]byte{cmdClose}, 40) // best-effort channel close
			time.Sleep(20 * time.Millisecond)
		}
		r.closeUSB()
	})
	return nil
}

func (r *RCan) closeUSB() {
	if r.handle != nil {
		_ = r.handle.ReleaseInterface(usbInterfaceNum)
		_ = r.handle.Close()
	}
	if r.dev != nil {
		r.dev.Close()
	}
	if r.usbCtx != nil {
		_ = r.usbCtx.Close()
	}
}

func (r *RCan) bulkOut(data []byte, timeoutMs int) (int, error) {
	return r.handle.BulkTransferOut(usbOutEndpointNum, data, timeoutMs)
}

func (r *RCan) bulkIn(buf []byte, timeoutMs int) (int, error) {
	return r.handle.BulkTransfer(usbInEndpointNum, buf, len(buf), timeoutMs)
}

func isUSBTimeout(err error) bool {
	ec, ok := err.(libusb.ErrorCode)
	return ok && ec == libusbErrTimeout
}

// Send writes one standard frame: cmd, idHi, idLo, dlc, data. Extended IDs
// are not implemented by the firmware yet (matches v1).
func (r *RCan) Send(_ context.Context, f gocan.Frame) error {
	if f.Extended {
		return errors.New("rCAN: extended IDs not implemented")
	}
	var buf [4 + 8]byte
	buf[0] = cmdCANFrame
	buf[1] = byte(f.ID >> 8)
	buf[2] = byte(f.ID)
	buf[3] = f.Length
	copy(buf[4:], f.Data[:f.Length])
	_, err := r.bulkOut(buf[:4+f.Length], 10)
	return err
}

func (r *RCan) readLoop(ctx context.Context) {
	const canHeaderSize = 3
	readBuf := make([]byte, maxCommandSize)
	commandBuff := make([]byte, maxCommandSize)
	var commandBuffPtr int

	for ctx.Err() == nil {
		n, err := r.bulkIn(readBuf, 100)
		if err != nil {
			if isUSBTimeout(err) {
				continue // idle, no data
			}
			if ctx.Err() == nil { // dead device, not a shutdown
				r.bus.Fatal(fmt.Errorf("failed to read from usb device: %w", err))
			}
			return
		}
		for i := range n {
			if commandBuffPtr >= maxCommandSize {
				commandBuffPtr = 0
				continue
			}
			commandBuff[commandBuffPtr] = readBuf[i]
			commandBuffPtr++
			if commandBuffPtr < canHeaderSize {
				continue
			}
			switch commandBuff[0] {
			case cmdCANFrame:
				dlc := int(commandBuff[1] >> 4)
				if dlc > 8 {
					r.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("invalid DLC: %d", dlc)})
					commandBuffPtr = 0
					continue
				}
				if commandBuffPtr == canHeaderSize+dlc {
					r.deliver(commandBuff[:commandBuffPtr])
					commandBuffPtr = 0
				}
			default:
				// unknown command byte: shift the buffer one byte and resync
				commandBuffPtr--
				if commandBuffPtr > 0 {
					copy(commandBuff[0:], commandBuff[1:commandBuffPtr+1])
				}
			}
		}
	}
}

// deliver decodes a cmdCANFrame packet: [cmd][dlc<<4|idHi][idLo][data...].
func (r *RCan) deliver(commandBuff []byte) {
	id := binary.LittleEndian.Uint16([]byte{commandBuff[2], commandBuff[1] & 0x0F})
	dlc := int(commandBuff[1] & 0xF0 >> 4)
	f := gocan.Frame{ID: uint32(id), Length: uint8(dlc)}
	copy(f.Data[:], commandBuff[3:3+dlc])
	r.bus.Deliver(f)
}
