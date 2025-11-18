//go:build pcan

package gocan

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"github.com/roffe/gocan/pkg/pcan"
)

func cString(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

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
	//log.Printf("Attached Channels Count: %d\n", len(channels))
	for i, channel := range channels {
		name := fmt.Sprintf("PCAN #%d %s", i, cString(channel.DeviceName[:]))
		if err := RegisterAdapter(&AdapterInfo{
			Name:               name,
			Description:        "PEAK-System CAN adapter for Windows",
			RequiresSerialPort: false,
			Capabilities: AdapterCapabilities{
				HSCAN: true,
				KLine: false,
				SWCAN: false,
			},
			New: NewPCANCHelper(name, channel.ChannelHandle),
		}); err != nil {
			panic(err)
		}
	}

}

type PCAN struct {
	*BaseAdapter
	ch     pcan.TPCANHandle
	rate   pcan.TPCANBaudrate
	closed bool
}

func NewPCANCHelper(name string, ch pcan.TPCANHandle) func(*AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		pcan, err := newPCAN(name, cfg)
		if err != nil {
			return nil, err
		}
		pcan.(*PCAN).ch = ch
		return pcan, nil
	}
}

func newPCAN(name string, cfg *AdapterConfig) (Adapter, error) {
	pcan := &PCAN{
		BaseAdapter: NewBaseAdapter(name, cfg),
	}
	var err error
	pcan.rate, err = pcanCANrate(cfg.CANRate * 1000)
	if err != nil {
		return nil, err
	}

	return pcan, nil
}

func (p *PCAN) Open(ctx context.Context) error {
	hwName, err := pcan.GetHardwareName(p.ch)
	if err != nil {
		return fmt.Errorf("failed to get hardware name: %w", err)
	}

	if err := pcan.CAN_Initialize(p.ch, p.rate); err != nil {
		return err
	}

	param := pcan.DWORD(pcan.PCAN_PARAMETER_ON)
	if err := pcan.CAN_SetValue(p.ch, pcan.PCAN_BUSOFF_AUTORESET, uintptr(unsafe.Pointer(&param)), 4); err != nil {
		p.Warn("Failed to set BUSOFF_AUTORESET parameter")
	}

	firmwareVersion, err := pcan.GetFirmwareVersion(p.ch)
	if err != nil {
		p.Warn("Failed to get firmware version")
	}
	p.Info(fmt.Sprintf("Name: %s %s", hwName, firmwareVersion))
	go p.recvManager(ctx)
	go p.sendManager(ctx)
	return nil
}

func (p *PCAN) Close() error {
	p.closed = true
	p.BaseAdapter.Close()
	return pcan.CAN_Uninitialize(p.ch)
}

func (p *PCAN) recvManager(ctx context.Context) {
	defer log.Println("exit recvManager")

	rxEvent, err := pcan.SetReceiveEvent(p.ch)
	if err != nil {
		p.Fatal(fmt.Errorf("SetReceiveEvent failed: %w", err))
		return
	}
	defer rxEvent.ClearReceiveEvent(p.ch)

	for ctx.Err() == nil {
		if err := rxEvent.Wait(10); err != nil {
			if err == syscall.ETIMEDOUT {
				continue
			}
			p.Fatal(fmt.Errorf("wait failed: %w", err))
			return
		}
		for {
			var msg pcan.TPCANMsg
			var timestamp pcan.TPCANTimestamp
			err := pcan.CAN_Read(p.ch, &msg, &timestamp)
			if err != nil {
				if err.(pcan.PCANError).Code == pcan.PCAN_ERROR_QRCVEMPTY {
					break
				}
				if !p.closed {
					p.Fatal(err)
				}
				return
			}
			frame := &CANFrame{
				Identifier: uint32(msg.ID),
				Data:       msg.DATA[:msg.LEN],
				FrameType:  Incoming,
			}
			select {
			case p.recvChan <- frame:
			default:
				p.Error(ErrDroppedFrame)
			}
		}
	}
}

func (p *PCAN) sendManager(ctx context.Context) {
	defer log.Println("exit sendManager")
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-p.sendChan:
			msg := pcan.TPCANMsg{
				ID:  uint32(frame.Identifier),
				LEN: uint8(len(frame.Data)),
			}
			copy(msg.DATA[:], frame.Data)
			if err := pcan.CAN_Write(p.ch, &msg); err != nil {
				p.Error(fmt.Errorf("failed to send frame: %w", err))
			}
		}
	}
}

func pcanCANrate(rate float64) (pcan.TPCANBaudrate, error) {
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
