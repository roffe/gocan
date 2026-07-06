//go:build pcan

package pcan

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"unsafe"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/pcan"
)

func init() {
	if err := pcan.Init(); err != nil {
		log.Println("PCANBasic driver not loaded:", err)
		return
	}
	channels, err := pcan.GetAttachedChannelsCount()
	if err != nil {
		log.Println(err)
		return
	}
	for i, channel := range channels {
		name := fmt.Sprintf("PCAN #%d %s", i, cString(channel.DeviceName[:]))
		handle := channel.ChannelHandle
		gocan.Register(gocan.AdapterInfo{
			Name:         name,
			Description:  "PEAK-System CAN adapter for Windows",
			Capabilities: gocan.Capabilities{HSCAN: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				return New(handle, cfg)
			},
		})
	}
}

type PCAN struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	ch   pcan.TPCANHandle
	rate pcan.TPCANBaudrate
}

func New(ch pcan.TPCANHandle, cfg gocan.Config) (gocan.Adapter, error) {
	rate, err := canRate(cfg.CANRate * 1000)
	if err != nil {
		return nil, err
	}
	return &PCAN{cfg: cfg, ch: ch, rate: rate}, nil
}

func (p *PCAN) Open(ctx context.Context, bus *gocan.Bus) error {
	p.bus = bus
	hwName, err := pcan.GetHardwareName(p.ch)
	if err != nil {
		return fmt.Errorf("failed to get hardware name: %w", err)
	}
	if err := pcan.CAN_Initialize(p.ch, p.rate); err != nil {
		return err
	}
	param := pcan.DWORD(pcan.PCAN_PARAMETER_ON)
	if err := pcan.CAN_SetValue(p.ch, pcan.PCAN_BUSOFF_AUTORESET, uintptr(unsafe.Pointer(&param)), 4); err != nil {
		bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "Failed to set BUSOFF_AUTORESET parameter"})
	}
	firmwareVersion, err := pcan.GetFirmwareVersion(p.ch)
	if err != nil {
		bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "Failed to get firmware version"})
	}
	bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: fmt.Sprintf("Name: %s %s", hwName, firmwareVersion)})

	go p.readLoop(ctx)
	return nil
}

func (p *PCAN) Close() error {
	return pcan.CAN_Uninitialize(p.ch)
}

// Send writes one frame; CAN_Write queues it on the controller, which is the
// closest the PCANBasic API offers to a write confirmation.
func (p *PCAN) Send(ctx context.Context, f gocan.Frame) error {
	msg := pcan.TPCANMsg{
		ID:  f.ID,
		LEN: f.Length,
	}
	copy(msg.DATA[:], f.Data[:f.Length])
	if err := pcan.CAN_Write(p.ch, &msg); err != nil {
		return fmt.Errorf("failed to send frame: %w", err)
	}
	return nil
}

func (p *PCAN) readLoop(ctx context.Context) {
	rxEvent, err := pcan.SetReceiveEvent(p.ch)
	if err != nil {
		p.bus.Fatal(fmt.Errorf("SetReceiveEvent failed: %w", err))
		return
	}
	defer rxEvent.ClearReceiveEvent(p.ch)

	for ctx.Err() == nil {
		if err := rxEvent.Wait(10); err != nil {
			if err == syscall.ETIMEDOUT {
				continue
			}
			if ctx.Err() == nil {
				p.bus.Fatal(fmt.Errorf("wait failed: %w", err))
			}
			return
		}
		for {
			var msg pcan.TPCANMsg
			var timestamp pcan.TPCANTimestamp
			err := pcan.CAN_Read(p.ch, &msg, &timestamp)
			if err != nil {
				if pe, ok := err.(pcan.PCANError); ok && pe.Code == pcan.PCAN_ERROR_QRCVEMPTY {
					break
				}
				if ctx.Err() == nil {
					p.bus.Fatal(err)
				}
				return
			}
			if msg.LEN > 8 {
				continue
			}
			f := gocan.Frame{ID: msg.ID, Length: msg.LEN}
			copy(f.Data[:], msg.DATA[:msg.LEN])
			p.bus.Deliver(f)
		}
	}
}

func cString(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func canRate(rate float64) (pcan.TPCANBaudrate, error) {
	switch rate {
	case 1_000_000:
		return pcan.PCAN_BAUD_1M, nil
	case 800_000:
		return pcan.PCAN_BAUD_800K, nil
	case 615384:
		return pcan.TPCANBaudrate(0x4037), nil
	case 500_000:
		return pcan.PCAN_BAUD_500K, nil
	case 250_000:
		return pcan.PCAN_BAUD_250K, nil
	case 125_000:
		return pcan.PCAN_BAUD_125K, nil
	case 100_000:
		return pcan.PCAN_BAUD_100K, nil
	case 95_000:
		return pcan.PCAN_BAUD_95K, nil
	case 83_000:
		return pcan.PCAN_BAUD_83K, nil
	case 50_000:
		return pcan.PCAN_BAUD_50K, nil
	case 47_000:
		return pcan.PCAN_BAUD_47K, nil
	case 33_000:
		return pcan.PCAN_BAUD_33K, nil
	case 20_000:
		return pcan.PCAN_BAUD_20K, nil
	case 10_000:
		return pcan.PCAN_BAUD_10K, nil
	case 5_000:
		return pcan.PCAN_BAUD_5K, nil
	default:
		return 0, fmt.Errorf("unsupported CAN rate: %v", rate)
	}
}
