package socketcan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/roffe/gocan/v2"
	"go.einride.tech/can/pkg/candevice"
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
	cfg       gocan.Config
	bus       *gocan.Bus
	dev       *candevice.Device
	broughtUp bool // we took the interface up, so we take it down again
	f         *os.File
	deadline  bool // f has a write deadline; Send is serialized by the Bus
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
	// vcan interfaces have no bittiming and are shared between processes;
	// leave their device state alone.
	if !strings.HasPrefix(a.cfg.Port, "vcan") {
		if err := a.bringUp(); err != nil {
			return err
		}
	}
	if a.f, err = a.dial(); err != nil {
		return fmt.Errorf("socketcan %s: %w", a.cfg.Port, err)
	}
	go a.readLoop(ctx)
	return nil
}

// dial opens a non-blocking CAN_RAW socket bound to the interface. As an
// os.File it sits on the runtime poller, so Close unblocks the read loop.
func (a *SocketCAN) dial() (*os.File, error) {
	ifi, err := net.InterfaceByName(a.cfg.Port)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.CAN_RAW)
	if err != nil {
		return nil, err
	}
	if len(a.cfg.CANFilter) > 0 {
		filters := make([]unix.CanFilter, len(a.cfg.CANFilter))
		for i, id := range a.cfg.CANFilter {
			filters[i] = unix.CanFilter{Id: id, Mask: unix.CAN_SFF_MASK}
		}
		err = unix.SetsockoptCanRawFilter(fd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, filters)
	}
	if err == nil {
		err = unix.Bind(fd, &unix.SockaddrCAN{Ifindex: ifi.Index})
	}
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), a.cfg.Port), nil
}

// bringUp configures the interface only if it is down. An interface that is
// already up was configured by the system (networkd, ip link, ...) and is left
// alone: reconfiguring it needs CAP_NET_ADMIN and would disturb other users.
func (a *SocketCAN) bringUp() error {
	up, err := a.dev.IsUp()
	if err != nil {
		return err
	}
	if up {
		if rate, err := a.dev.Bitrate(); err == nil && rate != uint32(a.cfg.CANRate*1000) {
			a.bus.Emit(gocan.Event{
				Type:    gocan.EventTypeWarning,
				Details: fmt.Sprintf("%s is already up at %d kbit/s, requested %.0f kbit/s", a.cfg.Port, rate/1000, a.cfg.CANRate),
			})
		}
		return nil
	}
	if err := a.dev.SetBitrate(uint32(a.cfg.CANRate * 1000)); err != nil {
		return a.configErr(err)
	}
	if err := a.dev.SetUp(); err != nil {
		return a.configErr(err)
	}
	a.broughtUp = true
	return nil
}

func (a *SocketCAN) configErr(err error) error {
	return fmt.Errorf("%s is down and could not be configured: %w (bring it up first: sudo ip link set %s up type can bitrate %.0f)",
		a.cfg.Port, err, a.cfg.Port, a.cfg.CANRate*1000)
}

func (a *SocketCAN) Close() error {
	if a.f != nil {
		a.f.Close() // unblocks the read loop
	}
	if a.dev != nil && a.broughtUp {
		return a.dev.SetDown()
	}
	return nil
}

// struct can_frame: canid_t (host order) | len | pad, res0, len8_dlc | data[8]
const frameSize = 16

// Send transmits one frame; the write blocks until the kernel accepts it.
func (a *SocketCAN) Send(ctx context.Context, f gocan.Frame) error {
	// Set a deadline when ctx has one, clear a previous one when it does not:
	// an expired deadline left on the fd fails every later write.
	if d, ok := ctx.Deadline(); ok || a.deadline {
		if err := a.f.SetWriteDeadline(d); err != nil {
			return fmt.Errorf("send error: %w", err)
		}
		a.deadline = ok
	}
	var buf [frameSize]byte // stays on the stack: os.File.Write does not retain it
	id := f.ID
	if f.Extended || a.cfg.UseExtendedID {
		id = id&unix.CAN_EFF_MASK | unix.CAN_EFF_FLAG
	} else {
		id &= unix.CAN_SFF_MASK
	}
	if f.Remote {
		id |= unix.CAN_RTR_FLAG
	}
	binary.NativeEndian.PutUint32(buf[:], id)
	buf[4] = min(f.Length, 8)
	copy(buf[8:], f.Data[:buf[4]])
	if _, err := a.f.Write(buf[:]); err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	return nil
}

func (a *SocketCAN) readLoop(ctx context.Context) {
	var buf [frameSize]byte
	for {
		n, err := a.f.Read(buf[:])
		if err != nil {
			if ctx.Err() == nil {
				a.bus.Fatal(fmt.Errorf("socketcan receive: %w", err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		id := binary.NativeEndian.Uint32(buf[:])
		if n != frameSize || id&unix.CAN_ERR_FLAG != 0 {
			continue
		}
		frame := gocan.Frame{
			Extended: id&unix.CAN_EFF_FLAG != 0,
			Remote:   id&unix.CAN_RTR_FLAG != 0,
			Length:   min(buf[4], 8),
		}
		if frame.Extended {
			frame.ID = id & unix.CAN_EFF_MASK
		} else {
			frame.ID = id & unix.CAN_SFF_MASK
		}
		copy(frame.Data[:], buf[8:8+frame.Length])
		a.bus.Deliver(frame)
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
