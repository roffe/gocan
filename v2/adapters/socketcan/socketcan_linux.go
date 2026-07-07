package socketcan

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/roffe/gocan/v2"
	"go.einride.tech/can"
	"go.einride.tech/can/pkg/candevice"
	"go.einride.tech/can/pkg/socketcan"
	"golang.org/x/sys/unix"
)

func init() {
	gocan.RegisterScanner(scanDevices)
}

func scanDevices() []gocan.AdapterInfo {
	var out []gocan.AdapterInfo
	for _, dev := range findDevices() {
		out = append(out, gocan.AdapterInfo{
			Name:         "SocketCAN " + dev,
			Description:  "Linux Driver",
			Capabilities: gocan.Capabilities{HSCAN: true, SWCAN: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				cfg.Port = dev
				return New(cfg)
			},
		})
	}
	return out
}

type SocketCAN struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	dev  *candevice.Device
	conn net.Conn
	tx   *socketcan.Transmitter
	rx   *socketcan.Receiver
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &SocketCAN{cfg: cfg}, nil
}

func (a *SocketCAN) Open(ctx context.Context, bus *gocan.Bus) error {
	a.bus = bus
	var err error
	a.dev, err = candevice.New(a.cfg.Port)
	if err != nil {
		return err
	}
	if err := a.dev.SetBitrate(uint32(a.cfg.CANRate * 1000)); err != nil {
		return err
	}
	if err := a.dev.SetUp(); err != nil {
		return err
	}

	filters := make([]socketcan.IDFilter, len(a.cfg.CANFilter))
	for i, filter := range a.cfg.CANFilter {
		filters[i].ID = filter
		filters[i].Mask = unix.CAN_SFF_MASK
	}

	a.conn, err = socketcan.DialContext(ctx, "can", a.cfg.Port, socketcan.WithFilterReceivedFramesByID(filters))
	if err != nil {
		return err
	}
	a.tx = socketcan.NewTransmitter(a.conn)
	a.rx = socketcan.NewReceiver(a.conn)

	go a.readLoop(ctx)
	return nil
}

func (a *SocketCAN) Close() error {
	if a.conn != nil {
		a.conn.Close() // unblocks the read loop
	}
	if a.dev != nil {
		return a.dev.SetDown()
	}
	return nil
}

// Send transmits one frame; TransmitFrame blocks until the kernel accepts it.
func (a *SocketCAN) Send(ctx context.Context, f gocan.Frame) error {
	frame := can.Frame{
		ID:         f.ID,
		Length:     f.Length,
		IsExtended: f.Extended || a.cfg.UseExtendedID,
	}
	copy(frame.Data[:], f.Data[:f.Length])
	if err := a.tx.TransmitFrame(ctx, frame); err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	return nil
}

func (a *SocketCAN) readLoop(ctx context.Context) {
	for a.rx.Receive() {
		if ctx.Err() != nil {
			return
		}
		f := a.rx.Frame()
		frame := gocan.Frame{ID: f.ID, Length: f.Length, Extended: f.IsExtended, Remote: f.IsRemote}
		copy(frame.Data[:], f.Data[:f.Length])
		a.bus.Deliver(frame)
	}
	if ctx.Err() == nil {
		if err := a.rx.Err(); err != nil {
			a.bus.Fatal(fmt.Errorf("socketcan receive: %w", err))
		}
	}
}

func findDevices() (dev []string) {
	iFaces, _ := net.Interfaces()
	for _, i := range iFaces {
		if strings.Contains(i.Name, "can") {
			dev = append(dev, i.Name)
		}
	}
	return
}
