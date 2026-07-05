//go:build rcan

package rcan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/gousb"
	gocan "github.com/roffe/gocan/v2"
)

const (
	usbVID = 0xffff
	usbPID = 0x1337

	usbConfigNumber   = 1
	usbInterfaceNum   = 0
	usbAltSetting     = 0
	usbOutEndpointNum = 0x01
	usbInEndpointNum  = 0x81

	maxCommandSize = 512

	cmdCANFrame = 0x05
	cmdSetSpeed = 0x01
	cmdOpen     = 0x02
	cmdClose    = 0x03
)

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

	usbCtx *gousb.Context
	dev    *gousb.Device
	devCfg *gousb.Config
	iface  *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint

	readStream *gousb.ReadStream
	closeOnce  sync.Once
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &RCan{cfg: cfg}, nil
}

func (r *RCan) Open(ctx context.Context, bus *gocan.Bus) error {
	r.bus = bus
	usbCtx := gousb.NewContext()
	dev, err := usbCtx.OpenDeviceWithVIDPID(usbVID, usbPID)
	if err != nil {
		if dev != nil {
			dev.Close()
		}
		usbCtx.Close()
		return err
	}
	if dev == nil {
		usbCtx.Close()
		return errors.New("rCAN device not found")
	}

	cfg, err := dev.Config(usbConfigNumber)
	if err != nil {
		dev.Close()
		usbCtx.Close()
		return err
	}
	iface, err := cfg.Interface(usbInterfaceNum, usbAltSetting)
	if err != nil {
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return err
	}
	in, err := iface.InEndpoint(usbInEndpointNum)
	if err != nil {
		iface.Close()
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return fmt.Errorf("InEndpoint(%d): %w", usbInEndpointNum, err)
	}
	out, err := iface.OutEndpoint(usbOutEndpointNum)
	if err != nil {
		iface.Close()
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return err
	}
	r.usbCtx, r.dev, r.devCfg = usbCtx, dev, cfg
	r.iface, r.in, r.out = iface, in, out

	stream, err := r.in.NewStream(maxCommandSize, 2) // 2 transfers in flight
	if err != nil {
		r.closeUSB()
		return err
	}
	r.readStream = stream

	go r.readLoop(ctx)

	// close (reset), set speed, open — mirrors the v1 bring-up.
	if _, err := r.out.WriteContext(ctx, []byte{cmdClose}); err != nil {
		r.closeUSB()
		return err
	}
	speedBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(speedBytes, uint16(r.cfg.CANRate))
	if _, err := r.out.WriteContext(ctx, append([]byte{cmdSetSpeed}, speedBytes...)); err != nil {
		r.closeUSB()
		return err
	}
	if _, err := r.out.WriteContext(ctx, []byte{cmdOpen}); err != nil {
		r.closeUSB()
		return err
	}
	return nil
}

func (r *RCan) Close() error {
	r.closeOnce.Do(func() {
		if r.out != nil {
			r.out.Write([]byte{cmdClose}) // best-effort channel close
			time.Sleep(20 * time.Millisecond)
		}
		r.closeUSB()
	})
	return nil
}

func (r *RCan) closeUSB() {
	if r.readStream != nil {
		r.readStream.Close()
	}
	if r.iface != nil {
		r.iface.Close()
	}
	if r.devCfg != nil {
		r.devCfg.Close()
	}
	if r.dev != nil {
		r.dev.Close()
	}
	if r.usbCtx != nil {
		r.usbCtx.Close()
	}
}

// Send writes one standard frame: cmd, idHi, idLo, dlc, data. Extended IDs
// are not implemented by the firmware yet (matches v1).
func (r *RCan) Send(ctx context.Context, f gocan.Frame) error {
	if f.Extended {
		return errors.New("rCAN: extended IDs not implemented")
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	var buf [4 + 8]byte
	buf[0] = cmdCANFrame
	buf[1] = byte(f.ID >> 8)
	buf[2] = byte(f.ID)
	buf[3] = f.Length
	copy(buf[4:], f.Data[:f.Length])
	_, err := r.out.WriteContext(wctx, buf[:4+f.Length])
	return err
}

func (r *RCan) readLoop(ctx context.Context) {
	const canHeaderSize = 3
	readBuf := make([]byte, maxCommandSize)
	commandBuff := make([]byte, maxCommandSize)
	var commandBuffPtr int

	for {
		n, err := r.readStream.ReadContext(ctx, readBuf)
		if err != nil {
			if ctx.Err() == nil {
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
