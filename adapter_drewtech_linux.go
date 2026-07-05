package gocan

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/roffe/gocan/pkg/drewtech"
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "Drewtech Mongoose",
		Description:        "Drewtech Mongoose Linux Driver",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: true,
		},
		New: NewDrewtech,
	}); err != nil {
		panic(err)
	}
}

// Drewtech adapts the native Mongoose Pro GM II client (pkg/drewtech) to the
// gocan Adapter interface.
type Drewtech struct {
	*BaseAdapter
	dev     *drewtech.Device
	filters []uint32
}

func NewDrewtech(cfg *AdapterConfig) (Adapter, error) {
	return &Drewtech{
		BaseAdapter: NewSyncBaseAdapter("Drewtech Mongoose", cfg),
	}, nil
}

func (a *Drewtech) Open(ctx context.Context) error {
	a.dev = drewtech.New(
		drewtech.WithCANFrameHandler(a.recvFrame),
		drewtech.WithErrorHandler(a.Error),
	)
	if err := a.dev.Open(a.cfg.Port); err != nil {
		return fmt.Errorf("open device: %w", err)
	}

	log.Println("Opening device...")
	if err := a.dev.PassThruOpen(); err != nil {
		a.dev.Close()
		return fmt.Errorf("PassThruOpen failed: %w", err)
	}
	v := a.dev.PassThruReadVersion()
	log.Printf("FW: %s BL: %s SN: %s", v.Firmware, v.Bootloader, v.Serial)

	baudRate := uint32(math.Round(a.cfg.CANRate * 1000))
	log.Printf("Connecting to CAN at %d bit/s...", baudRate)
	if err := a.dev.PassThruConnect(baudRate); err != nil {
		a.dev.Close()
		return fmt.Errorf("PassThruConnect failed: %w", err)
	}

	if len(a.cfg.CANFilter) > 0 {
		if err := a.SetFilter(a.cfg.CANFilter); err != nil {
			return err
		}
	}

	go a.sendManager(ctx)

	return nil
}

// recvFrame bridges drewtech RX pushes onto the adapter's receive channel.
func (a *Drewtech) recvFrame(f *drewtech.CANFrame) {
	frame := &CANFrame{Identifier: f.ID, Data: f.Data, FrameType: Incoming}
	select {
	case a.recvChan <- frame:
	default:
		a.Error(fmt.Errorf("recvChan full, frame 0x%03X dropped", f.ID))
	}
}

// SetFilter replaces the installed PASS filters. A filter per CAN id is
// required: with none installed the device forwards nothing.
func (a *Drewtech) SetFilter(filters []uint32) error {
	for _, id := range a.filters {
		if err := a.dev.PassThruStopMsgFilter(id); err != nil {
			log.Printf("StopMsgFilter: %v", err)
		}
	}
	a.filters = a.filters[:0]
	log.Printf("Starting filter... %02X", filters)
	for _, filter := range filters {
		id, err := a.dev.PassThruStartMsgFilter(drewtech.PASS_FILTER, filter, 0xffffffff)
		if err != nil {
			return fmt.Errorf("PassThruStartMsgFilter failed: %w", err)
		}
		a.filters = append(a.filters, id)
	}
	return nil
}

func (a *Drewtech) Close() error {
	log.Println("Close")
	a.BaseAdapter.Close()
	if a.dev != nil {
		for _, id := range a.filters {
			if err := a.dev.PassThruStopMsgFilter(id); err != nil {
				log.Printf("StopMsgFilter: %v", err)
			}
		}
		a.filters = nil

		// Small delay to process remaining frames
		time.Sleep(100 * time.Millisecond)

		if err := a.dev.PassThruDisconnect(); err != nil {
			log.Printf("Disconnect: %v", err)
		}
		// PassThruClose also tears down the read loop and serial port.
		if err := a.dev.PassThruClose(); err != nil {
			log.Printf("Close: %v", err)
		}
	}
	return nil
}

func (a *Drewtech) sendManager(ctx context.Context) {
	defer log.Println("exit sendmanager")
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.closeChan:
			return
		case f := <-a.sendChan:
			if err := a.dev.PassThruWriteMsgs(f.Identifier, f.Data); err != nil {
				a.Error(fmt.Errorf("send error: %w", err))
			}
			f.markSent()
		}
	}
}
