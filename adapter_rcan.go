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
	rCANVID = 0xFFFF
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
	rCANMaxCommandSize = 64
)

type RCanDevice struct {
	*BaseAdapter

	usbCtx *gousb.Context
	dev    *gousb.Device
	devCfg *gousb.Config
	iface  *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint

	closeOnce sync.Once
}

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "rCAN",
		Description:        "WinUSB CAN device by roffe.nu",
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

	// Start I/O goroutines after endpoints are ready
	go r.recvManager(ctx)
	go r.sendManager(ctx)

	if _, err := r.out.WriteContext(ctx, []byte{0x03}); err != nil {
		r.closeUSB()
		return err
	}

	time.Sleep(50 * time.Millisecond)

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
	time.Sleep(150 * time.Millisecond)
	r.closeUSB()
	return nil
}

func (r *RCanDevice) closeUSB() {
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
		cmdCAN        = 0x05
		canHeaderSize = 4
		maxDLC        = 8
		readTimeout   = 100 * time.Millisecond
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

		// Non-blocking read with short timeout
		rctx, cancel := context.WithTimeout(ctx, readTimeout)
		n, err := r.in.ReadContext(rctx, readBuf)
		cancel()

		if err != nil {
			r.Fatal(fmt.Errorf("failed to read from rCAN: %w", err))
			continue
		}
		if n == 0 {
			continue
		}

		// Process all bytes from this read
		for i := 0; i < n; i++ {
			b := readBuf[i]

			// Prevent buffer overflow
			if commandBuffPtr >= len(commandBuff) {
				log.Printf("Command buffer overflow, resetting")
				commandBuffPtr = 0
				continue
			}

			commandBuff[commandBuffPtr] = b
			commandBuffPtr++

			// Early exit if we don't have enough data yet
			if commandBuffPtr < canHeaderSize {
				continue
			}

			// Check if we have a full command
			if commandBuff[0] == cmdCAN {
				dlc := int(commandBuff[3])
				// Validate DLC
				if dlc > maxDLC {
					log.Printf("Invalid DLC: %d, resetting buffer", dlc)
					commandBuffPtr = 0
					continue
				}

				if commandBuffPtr == canHeaderSize+dlc {
					// Full command received
					r.processCommand(commandBuff[:commandBuffPtr])
					commandBuffPtr = 0
				}
			} else {
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

func (r *RCanDevice) recvManager23(ctx context.Context) {
	if r.cfg.Debug {
		defer log.Println("recvManager exited")
	}
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
			rctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			n, err := r.in.ReadContext(rctx, readBuf)
			cancel()
			if err != nil {
				//r.Error(fmt.Errorf("failed to read from rCAN: %w", err))
				r.Fatal(fmt.Errorf("failed to read from rCAN: %w", err))
				continue
			}
			if n == 0 {
				continue
			}

			for _, b := range readBuf[:n] {
				commandBuff[commandBuffPtr] = b
				commandBuffPtr++
				// Check if we have a full command
				switch commandBuff[0] {
				case 0x05: // CAN frame
					// byte 0: command (0x05)
					// byte 1: ID high
					// byte 2: ID low
					// byte 3: DLC
					// byte 4-11: data
					if commandBuffPtr >= 4 {
						dlc := int(commandBuff[3])
						if commandBuffPtr == 4+dlc {
							// Full command received
							r.processCommand(commandBuff[:commandBuffPtr])
							commandBuffPtr = 0
						}
					}
				default:
					//r.Error(fmt.Errorf("unknown command received: % 02X", commandBuff[0]))
					log.Printf("Unknown command received: % 02X\n", commandBuff[0])
					//commandBuffPtr = 0
					// shift commandBuff left by one
					copy(commandBuff, commandBuff[1:commandBuffPtr])
					commandBuffPtr--
					commandBuff = append(commandBuff, []byte{0x00}...)
				}
			}
		}
	}
}

func (r *RCanDevice) processCommand(commandBuff []byte) {
	switch commandBuff[0] {
	case 0x05: // CAN frame
		id := binary.LittleEndian.Uint16([]byte{commandBuff[2], commandBuff[1]})
		dlc := int(commandBuff[3])
		data := make([]byte, dlc)
		copy(data, commandBuff[4:4+dlc])
		frame := &CANFrame{
			Identifier: uint32(id),
			Extended:   false,
			Data:       data,
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
