package drewtech

import (
	"context"
	"fmt"
	"math"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/drewtech"
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:               "Drewtech Mongoose",
		Description:        "Drewtech Mongoose Linux Driver",
		RequiresSerialPort: true,
		Capabilities:       gocan.Capabilities{HSCAN: true, SWCAN: true, KLine: true},
		New:                New,
	})
}

// Drewtech adapts the native Mongoose Pro GM II client to the v2 Adapter
// interface.
type Drewtech struct {
	cfg     gocan.Config
	bus     *gocan.Bus
	dev     *drewtech.Device
	filters []uint32
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &Drewtech{cfg: cfg}, nil
}

func (a *Drewtech) Open(ctx context.Context, bus *gocan.Bus) error {
	a.bus = bus
	a.dev = drewtech.New(
		drewtech.WithCANFrameHandler(a.recvFrame),
		drewtech.WithErrorHandler(func(err error) {
			bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
		}),
	)
	if err := a.dev.Open(a.cfg.Port); err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	if err := a.dev.PassThruOpen(); err != nil {
		a.dev.Close()
		return fmt.Errorf("PassThruOpen failed: %w", err)
	}
	v := a.dev.PassThruReadVersion()
	bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: fmt.Sprintf("FW: %s BL: %s SN: %s", v.Firmware, v.Bootloader, v.Serial)})

	baudRate := uint32(math.Round(a.cfg.CANRate * 1000))
	if err := a.dev.PassThruConnect(baudRate); err != nil {
		a.dev.Close()
		return fmt.Errorf("PassThruConnect failed: %w", err)
	}
	return a.SetFilter(a.cfg.CANFilter)
}

func (a *Drewtech) recvFrame(f *drewtech.CANFrame) {
	if len(f.Data) > 8 {
		return
	}
	frame := gocan.Frame{ID: f.ID, Length: uint8(len(f.Data))}
	copy(frame.Data[:], f.Data)
	a.bus.Deliver(frame)
}

// SetFilter replaces the installed PASS filters. A filter per CAN id is
// required: with none installed the device forwards nothing, so an empty
// list installs a single pass-all filter (mask 0 matches every id).
func (a *Drewtech) SetFilter(filters []uint32) error {
	for _, id := range a.filters {
		a.dev.PassThruStopMsgFilter(id)
	}
	a.filters = a.filters[:0]
	if len(filters) == 0 {
		id, err := a.dev.PassThruStartMsgFilter(drewtech.PASS_FILTER, 0, 0)
		if err != nil {
			return fmt.Errorf("PassThruStartMsgFilter failed: %w", err)
		}
		a.filters = append(a.filters, id)
		return nil
	}
	for _, filter := range filters {
		id, err := a.dev.PassThruStartMsgFilter(drewtech.PASS_FILTER, filter, 0xffffffff)
		if err != nil {
			return fmt.Errorf("PassThruStartMsgFilter failed: %w", err)
		}
		a.filters = append(a.filters, id)
	}
	return nil
}

func (a *Drewtech) Send(ctx context.Context, f gocan.Frame) error {
	return a.dev.PassThruWriteMsgs(f.ID, f.Bytes())
}

func (a *Drewtech) Close() error {
	if a.dev == nil {
		return nil
	}
	for _, id := range a.filters {
		a.dev.PassThruStopMsgFilter(id)
	}
	a.filters = nil
	time.Sleep(100 * time.Millisecond) // let remaining frames drain
	a.dev.PassThruDisconnect()
	return a.dev.PassThruClose()
}
