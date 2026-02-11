//go:build socketcan

package gocan

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"go.einride.tech/can"
	"go.einride.tech/can/pkg/candevice"
	"go.einride.tech/can/pkg/socketcan"
	"golang.org/x/sys/unix"
)

func init() {
	for _, dev := range FindDevices() {
		name := "SocketCAN " + dev
		if err := RegisterAdapter(&AdapterInfo{
			Name:               name,
			Description:        "Linux Driver",
			RequiresSerialPort: false,
			Capabilities: AdapterCapabilities{
				HSCAN: true,
				KLine: false,
				SWCAN: true,
			},
			New: NewSocketCANFromDevName(dev),
		}); err != nil {
			panic(err)
		}
	}
}

type SocketCAN struct {
	*BaseAdapter
	d  *candevice.Device
	tx *socketcan.Transmitter
	rx *socketcan.Receiver
}

func (a *SocketCAN) SetFilter(uint32s []uint32) error {

	return nil
}

func NewSocketCANFromDevName(dev string) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		cfg.Port = dev
		return NewSocketCAN(cfg)
	}
}

func NewSocketCAN(cfg *AdapterConfig) (Adapter, error) {
	return &SocketCAN{
		BaseAdapter: NewBaseAdapter("SocketCAN", cfg),
	}, nil
}

func (a *SocketCAN) Open(ctx context.Context) error {
	var err error = nil
	a.d, err = candevice.New(a.cfg.Port)
	if err != nil {
		return err
	}

	err = a.d.SetBitrate(uint32(a.cfg.CANRate * 1000))
	if err != nil {
		return err
	}
	err = a.d.SetUp()
	if err != nil {
		return err
	}

	var filters = make([]socketcan.IDFilter, len(a.cfg.CANFilter))
	for i, filter := range a.cfg.CANFilter {

		filters[i].ID = filter
		filters[i].Mask = unix.CAN_SFF_MASK
	}

	conn, err := socketcan.DialContext(ctx, "can", a.cfg.Port, socketcan.WithFilterReceivedFramesByID(filters))

	a.tx = socketcan.NewTransmitter(conn)
	a.rx = socketcan.NewReceiver(conn)

	go a.recvManager(ctx)
	go a.sendManager(ctx)
	return err
}

func (a *SocketCAN) Close() error {
	a.BaseAdapter.Close()
	defer func(d *candevice.Device) {
		err := d.SetDown()
		if err != nil {
		}
	}(a.d)
	return nil
}

func (a *SocketCAN) recvManager(ctx context.Context) {
	runtime.LockOSThread()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if a.rx.Receive() {
				f := a.rx.Frame()
				frame := NewFrame(
					f.ID,
					f.Data[0:f.Length],
					Incoming,
				)
				select {
				case a.recvChan <- frame:
				default:
					a.Error(ErrDroppedFrame)
				}
			}
		}
	}
}

func (a *SocketCAN) sendManager(ctx context.Context) {
	runtime.LockOSThread()
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-a.sendChan:
			frame := can.Frame{}
			frame.IsExtended = a.cfg.UseExtendedID
			frame.ID = f.Identifier
			frame.Length = uint8(f.DLC())
			data := can.Data{}
			copy(data[:], f.Data)
			frame.Data = data
			if err := a.tx.TransmitFrame(ctx, frame); err != nil {
				a.Error(fmt.Errorf("send error: %w", err))
			}
			//workaround, delay to prevent can flasher fail
			time.Sleep(time.Millisecond)
		}
	}
}

func FindDevices() (dev []string) {
	iFaces, _ := net.Interfaces()
	for _, i := range iFaces {
		if strings.Contains(i.Name, "can") {
			dev = append(dev, i.Name)
		}
	}
	return
}
