package adapter

import (
	"context"
	"fmt"
	"github.com/roffe/gocan"
	"go.einride.tech/can"
	"go.einride.tech/can/pkg/candevice"
	"go.einride.tech/can/pkg/socketcan"
	"net"
	"runtime"
	"strings"
	"time"
)

func init() {
	for _, dev := range FindDevices() {
		name := "SocketCAN " + dev
		if err := Register(&AdapterInfo{
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
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
	d          *candevice.Device
	tx         *socketcan.Transmitter
	rx         *socketcan.Receiver
}

func NewSocketCANFromDevName(dev string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		cfg.Port = dev
		return NewSocketCAN(cfg)
	}
}

func NewSocketCAN(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &SocketCAN{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 10),
		close: make(chan struct{}, 1),
	}, nil
}

func (a *SocketCAN) Name() string {
	return "SocketCAN"
}

func (a *SocketCAN) Init(ctx context.Context) error {
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

	conn, err := socketcan.DialContext(ctx, "can", a.cfg.Port)

	a.tx = socketcan.NewTransmitter(conn)
	a.rx = socketcan.NewReceiver(conn)

	go a.recvManager(ctx)
	go a.sendManager(ctx)
	return err
}

func (a *SocketCAN) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *SocketCAN) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *SocketCAN) Close() error {
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
		case <-a.close:
			return
		default:
			if a.rx.Receive() {
				f := a.rx.Frame()
				for i := 0; i < len(a.cfg.CANFilter); i++ {
					if f.ID == a.cfg.CANFilter[i] {
						frame := gocan.NewFrame(
							f.ID,
							f.Data[0:f.Length],
							gocan.Incoming,
						)
						a.recv <- frame
					}
				}
			}
		}
	}
}

func (a *SocketCAN) sendManager(ctx context.Context) {
	runtime.LockOSThread()
	for {
		select {
		case <-a.close:
			return
		case f := <-a.send:
			frame := can.Frame{}
			frame.ID = f.Identifier()
			frame.Length = uint8(f.Length())
			data := can.Data{}
			copy(data[:], f.Data())
			frame.Data = data
			if err := a.tx.TransmitFrame(ctx, frame); err != nil {
				a.cfg.OnError(fmt.Errorf("send error: %w", err))
			}
			delay := 20 * time.Millisecond

			time.Sleep(delay)
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
