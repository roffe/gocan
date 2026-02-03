//go:build rcan

package gocan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/gousb"
)

const (
	rCANVID = 0xffff
	rCANPID = 0x1337
)

const (
	// USB topology
	rCANusbConfigNumber   = 1
	rCANusbInterfaceNum   = 0
	rCANusbAltSetting     = 0
	rCANusbOutEndpointNum = 0x01
	rCANusbInEndpointNum  = 0x81
)

const (
	rCANMaxCommandSize = 512
)

type RCanDevice struct {
	*BaseAdapter

	usbCtx *gousb.Context
	dev    *gousb.Device
	devCfg *gousb.Config
	iface  *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint

	readStream *gousb.ReadStream

	closeOnce sync.Once
}

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "rCAN",
		Description:        "CAN device by roffe.nu",
		RequiresSerialPort: false,
		Capabilities:       AdapterCapabilities{HSCAN: true},
		New:                NewrCAN,
	}); err != nil {
		panic(err)
	}
}

func NewrCAN(cfg *AdapterConfig) (Adapter, error) {
	return &RCanDevice{
		BaseAdapter: NewBaseAdapter("rCAN", cfg),
	}, nil
}

func (r *RCanDevice) Open(ctx context.Context) error {
	// --- Discover & bind USB ---
	ctxUSB := gousb.NewContext()
	ctxUSB.Debug(3)
	dev, err := ctxUSB.OpenDeviceWithVIDPID(rCANVID, rCANPID)
	if err != nil {
		if dev == nil {
			ctxUSB.Close()
			return errors.New("rCAN not found")
		}
		dev.Close()
		ctxUSB.Close()
		return err
	}
	if dev == nil {
		ctxUSB.Close()
		return errors.New("rCAN device not found")
	}

	cfg, err := dev.Config(rCANusbConfigNumber)
	if err != nil {
		dev.Close()
		ctxUSB.Close()
		return err
	}
	iface, err := cfg.Interface(rCANusbInterfaceNum, rCANusbAltSetting)
	if err != nil {
		cfg.Close()
		dev.Close()
		ctxUSB.Close()
		return err
	}

	in, err := iface.InEndpoint(rCANusbInEndpointNum)
	if err != nil {
		iface.Close()
		cfg.Close()
		dev.Close()
		ctxUSB.Close()
		return fmt.Errorf("InEndpoint(%d): %w", rCANusbInEndpointNum, err)
	}

	out, err := iface.OutEndpoint(rCANusbOutEndpointNum)
	if err != nil {
		iface.Close()
		cfg.Close()
		dev.Close()
		ctxUSB.Close()
		return err
	}

	// Save
	r.usbCtx, r.dev, r.devCfg = ctxUSB, dev, cfg
	r.iface, r.in, r.out = iface, in, out

	stream, err := r.in.NewStream(rCANMaxCommandSize, 2) // 2 transfers in flight
	if err != nil {
		r.closeUSB()
		return err
	}
	r.readStream = stream

	// Start I/O goroutines after endpoints are ready
	go r.recvManager(ctx)
	go r.sendManager(ctx)

	if _, err := r.out.WriteContext(ctx, []byte{0x03}); err != nil {
		r.closeUSB()
		return err
	}

	speedBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(speedBytes, uint16(r.cfg.CANRate))

	if _, err := r.out.WriteContext(ctx, append([]byte{0x01}, speedBytes...)); err != nil {
		r.closeUSB()
		return err
	}

	if _, err := r.out.WriteContext(ctx, []byte{0x02}); err != nil {
		r.closeUSB()
		return err
	}

	return nil
}

func (r *RCanDevice) Close() error {
	r.BaseAdapter.Close()
	var err error
	r.closeOnce.Do(func() { err = r.closeAdapter() })
	return err
}

func (r *RCanDevice) closeAdapter() error {
	r.out.Write([]byte{0x03})
	time.Sleep(20 * time.Millisecond)
	r.closeUSB()
	return nil
}

func (r *RCanDevice) closeUSB() {
	if r.readStream != nil {
		r.readStream.Close()
	}

	if r.iface != nil {
		r.iface.Close()
	}
	if r.devCfg != nil {
		_ = r.devCfg.Close()
	}
	if r.dev != nil {
		_ = r.dev.Close()
	}
	if r.usbCtx != nil {
		_ = r.usbCtx.Close()
	}
}

func (r *RCanDevice) recvManager(ctx context.Context) {
	if r.cfg.Debug {
		defer log.Println("recvManager exited")
	}

	const (
		canHeaderSize = 3
		maxDLC        = 8
	)

	readBuf := make([]byte, rCANMaxCommandSize)
	commandBuff := make([]byte, rCANMaxCommandSize)
	var commandBuffPtr int

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.closeChan:
			return
		default:
		}

		n, err := r.readStream.ReadContext(ctx, readBuf)
		if err != nil {
			r.Fatal(fmt.Errorf("failed to read from usb device: %w", err))
			return
		}
		if n == 0 {
			continue
		}

		// Process all bytes from this read
		for i := range n {
			// Prevent buffer overflow
			if commandBuffPtr >= rCANMaxCommandSize {
				log.Printf("Command buffer overflow, resetting")
				commandBuffPtr = 0
				continue
			}
			commandBuff[commandBuffPtr] = readBuf[i]
			commandBuffPtr++

			// Early exit if we don't have enough data yet
			if commandBuffPtr < canHeaderSize {
				continue
			}

			switch commandBuff[0] {
			case 0x05:
				dlc := int(commandBuff[1] >> 4)
				// Validate DLC
				if dlc > maxDLC {
					log.Printf("Invalid DLC: %d, resetting buffer", dlc)
					commandBuffPtr = 0
					continue
				}
				if commandBuffPtr == canHeaderSize+dlc {
					r.processCommand(commandBuff[:commandBuffPtr])
					commandBuffPtr = 0
				}
			default:
				// Unknown command - shift buffer
				log.Printf("Unknown command received: 0x%02X", commandBuff[0])
				commandBuffPtr--
				if commandBuffPtr > 0 {
					copy(commandBuff[0:], commandBuff[1:commandBuffPtr+1])
				}
			}
		}
	}
}

func (r *RCanDevice) recvManager2(ctx context.Context) {
	if r.cfg.Debug {
		defer log.Println("recvManager exited")
	}

	const (
		cmdCAN        = 0x05
		canHeaderSize = 3
		maxDLC        = 8
	)

	readBuf := make([]byte, rCANMaxCommandSize)
	commandBuff := make([]byte, rCANMaxCommandSize)
	var commandBuffPtr int

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.closeChan:
			return
		default:
		}

		n, err := r.in.Read(readBuf)

		if err != nil {
			log.Println(err)
			r.Fatal(fmt.Errorf("failed to read from usb device: %w", err))
			return
		}
		if n == 0 {
			log.Println(0)
			continue
		}

		// Process all bytes from this read
		for i := range n {
			// Prevent buffer overflow
			if commandBuffPtr >= rCANMaxCommandSize {
				log.Printf("Command buffer overflow, resetting")
				commandBuffPtr = 0
				continue
			}
			commandBuff[commandBuffPtr] = readBuf[i]
			commandBuffPtr++

			// Early exit if we don't have enough data yet
			if commandBuffPtr < canHeaderSize {
				continue
			}

			switch commandBuff[0] {
			case cmdCAN:
				dlc := int((commandBuff[1] & 0xF0) >> 4)
				// Validate DLC
				if dlc > maxDLC {
					log.Printf("Invalid DLC: %d, resetting buffer", dlc)
					commandBuffPtr = 0
					continue
				}
				if commandBuffPtr == canHeaderSize+dlc {
					r.processCommand(commandBuff[:commandBuffPtr])
					commandBuffPtr = 0
				}
			default:
				// Unknown command - shift buffer
				log.Printf("Unknown command received: 0x%02X", commandBuff[0])
				commandBuffPtr--
				if commandBuffPtr > 0 {
					copy(commandBuff[0:], commandBuff[1:commandBuffPtr+1])
				}
			}
		}
	}
}

func (r *RCanDevice) processCommand(commandBuff []byte) {
	switch commandBuff[0] {
	case 0x05: // CAN frame
		id := binary.LittleEndian.Uint16([]byte{commandBuff[2], commandBuff[1] & 0x0F})
		dlc := int(commandBuff[1] & 0xF0 >> 4)
		data := make([]byte, dlc)
		copy(data, commandBuff[3:3+dlc])
		frame := &CANFrame{
			Identifier: uint32(id),
			Extended:   false,
			Data:       data,
		}
		if r.cfg.Debug {
			log.Println(frame.String())
		}
		select {
		case r.recvChan <- frame:
		default:
			r.Error(ErrDroppedFrame)
			log.Println("Dropped frame:", frame)
		}
	default:
		r.Error(fmt.Errorf("unknown command received: % 02X", commandBuff))
		log.Printf("Unknown command received: % 02X\n", commandBuff)
	}
}

func (r *RCanDevice) sendManager(ctx context.Context) {
	if r.cfg.Debug {
		defer log.Println("sendManager exited")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.closeChan:
			return
		case frame := <-r.sendChan:
			r.sendCANMessage(ctx, frame)
		}
	}
}

func (r *RCanDevice) sendCANMessage(ctx context.Context, frame *CANFrame) {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	dlc := frame.DLC()
	if dlc < 0 || dlc > 8 {
		return
	}

	var buf [4 + 8]byte
	if !frame.Extended {
		buf[0] = 0x05
		id := uint16(frame.Identifier)
		buf[1] = byte(id >> 8)
		buf[2] = byte(id)
		buf[3] = byte(dlc)
		copy(buf[4:4+dlc], frame.Data[:dlc])
		_, _ = r.out.WriteContext(wctx, buf[:4+dlc])
		return
	}

	// TODO: implement ext
}

func (r *RCanDevice) sendCANMessage2(ctx context.Context, frame *CANFrame) {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	buf := make([]byte, frame.DLC()+4)
	if !frame.Extended {
		buf[0] = 0x05 // transmit command
		id := make([]byte, 4)
		binary.LittleEndian.PutUint32(id, frame.Identifier)
		buf[1] = id[1]
		buf[2] = id[0]
		buf[3] = byte(frame.DLC())
		copy(buf[4:], frame.Data)

	} else {
		buf[0] = 0x06 // transmit extended command
		log.Println("EXTENDED NOT IMPLEMENTED")
		return
	}

	//log.Printf("Sending: % 02X\n", buf)
	r.out.WriteContext(wctx, buf)
}
