//go:build pcan

package gocan

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"github.com/roffe/gopcan"
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
	channels, err := gopcan.GetAttachedChannelsCount()
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("Attached Channels Count: %d\n", len(channels))
	for _, channel := range channels {
		name := cString(channel.DeviceName[:])
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
	BaseAdapter
	ch     gopcan.TPCANHandle
	rate   gopcan.TPCANBaudrate
	closed bool
}

func NewPCANCHelper(name string, ch gopcan.TPCANHandle) func(*AdapterConfig) (Adapter, error) {
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
	hwName, err := gopcan.GetHardwareName(p.ch)
	if err != nil {
		return fmt.Errorf("failed to get hardware name: %w", err)
	}

	if err := gopcan.CAN_Initialize(p.ch, p.rate); err != nil {
		return err
	}

	firmwareVersion, err := gopcan.GetFirmwareVersion(p.ch)
	if err != nil {
		log.Println(err)
	}
	p.sendInfoEvent(fmt.Sprintf("Name: %s %s", hwName, firmwareVersion))
	go p.recvManager(ctx)
	go p.sendManager(ctx)
	return nil
}

func (p *PCAN) Close() error {
	p.closed = true
	p.BaseAdapter.Close()
	return gopcan.CAN_Uninitialize(p.ch)
}

func (p *PCAN) recvManager(ctx context.Context) {
	defer log.Println("exit recvManager")

	rxEvent, err := gopcan.SetReceiveEvent(p.ch)
	if err != nil {
		p.setError(fmt.Errorf("SetReceiveEvent failed: %w", err))
		return
	}
	defer rxEvent.ClearReceiveEvent(p.ch)

	for ctx.Err() == nil {
		if err := rxEvent.Wait(10); err != nil {
			if err == syscall.ETIMEDOUT {
				continue
			}
			p.setError(fmt.Errorf("wait failed: %w", err))
			return
		}
		for {
			var msg gopcan.TPCANMsg
			var timestamp gopcan.TPCANTimestamp
			err := gopcan.CAN_Read(p.ch, &msg, &timestamp)
			if err != nil {
				if err.(gopcan.PCANError).Code == gopcan.PCAN_ERROR_QRCVEMPTY {
					break
				}
				if !p.closed {
					p.setError(err)
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
				p.sendErrorEvent(ErrDroppedFrame)
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
			msg := gopcan.TPCANMsg{
				ID:  uint32(frame.Identifier),
				LEN: uint8(len(frame.Data)),
			}
			copy(msg.DATA[:], frame.Data)
			if err := gopcan.CAN_Write(p.ch, &msg); err != nil {
				p.sendErrorEvent(fmt.Errorf("failed to send frame: %w", err))
			}
		}
	}
}

func pcanCANrate(rate float64) (gopcan.TPCANBaudrate, error) {
	switch rate {
	case 1_000_000:
		return gopcan.PCAN_BAUD_1M, nil
	case 800_000:
		return gopcan.PCAN_BAUD_800K, nil
	case 615384:
		return gopcan.TPCANBaudrate(0x4037), nil
	case 500_000:
		return gopcan.PCAN_BAUD_500K, nil
	case 250_000:
		return gopcan.PCAN_BAUD_250K, nil
	case 125_000:
		return gopcan.PCAN_BAUD_125K, nil
	case 100_000:
		return gopcan.PCAN_BAUD_100K, nil
	case 95_000:
		return gopcan.PCAN_BAUD_95K, nil
	case 83_000:
		return gopcan.PCAN_BAUD_83K, nil
	case 50_000:
		return gopcan.PCAN_BAUD_50K, nil
	case 47_000:
		return gopcan.PCAN_BAUD_47K, nil
	case 33_000:
		return gopcan.PCAN_BAUD_33K, nil
	case 20_000:
		return gopcan.PCAN_BAUD_20K, nil
	case 10_000:
		return gopcan.PCAN_BAUD_10K, nil
	case 5_000:
		return gopcan.PCAN_BAUD_5K, nil
	default:
		return 0, fmt.Errorf("unsupported CAN rate: %v", rate)
	}
}
