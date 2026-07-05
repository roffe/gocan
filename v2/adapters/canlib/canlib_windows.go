//go:build canlib

package canlib

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/canlib"
)

const (
	defaultReadTimeoutMs  = 20
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

func init() {
	if err := canlib.Init(); err != nil {
		return // Kvaser driver not installed
	}
	channels, err := canlib.GetNumberOfChannels()
	if err != nil {
		return
	}
	for channel := range channels {
		devDescr, err := canlib.GetChannelDataString(channel, canlib.CHANNELDATA_DEVDESCR_ASCII)
		if err != nil {
			continue
		}
		if strings.HasPrefix(devDescr, "Kvaser Virtual") {
			continue
		}
		ch := channel
		gocan.Register(gocan.AdapterInfo{
			Name:         fmt.Sprintf("CANlib #%d %v", channel, devDescr),
			Description:  "Canlib driver for Kvaser devices",
			Capabilities: gocan.Capabilities{HSCAN: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				return New(ch, cfg)
			},
		})
	}
}

type CANlib struct {
	cfg          gocan.Config
	bus          *gocan.Bus
	channel      int
	readHandle   canlib.Handle
	writeHandle  canlib.Handle
	timeoutRead  uint32
	timeoutWrite uint32
	closeOnce    sync.Once
}

func New(channel int, cfg gocan.Config) (gocan.Adapter, error) {
	return &CANlib{
		cfg:          cfg,
		channel:      channel,
		timeoutRead:  defaultReadTimeoutMs,
		timeoutWrite: defaultWriteTimeoutMs,
	}, nil
}

func (k *CANlib) Open(ctx context.Context, bus *gocan.Bus) error {
	k.bus = bus
	bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "CANlib v" + canlib.GetVersion()})

	if err := k.openChannels(); err != nil {
		return err
	}
	if err := k.setSpeed(k.cfg.CANRate); err != nil {
		err1 := k.readHandle.Close()
		err2 := k.writeHandle.Close()
		return fmt.Errorf("setSpeed: %v, RH: %v WH: %v", err, err1, err2)
	}

	go k.readLoop(ctx)

	if err := k.readHandle.BusOn(); err != nil {
		return err
	}
	return k.writeHandle.BusOn()
}

func (k *CANlib) Close() error {
	k.closeOnce.Do(func() {
		k.readHandle.BusOff()
		k.writeHandle.BusOff()
		k.readHandle.FlushReceiveQueue()
		k.writeHandle.FlushReceiveQueue()
		k.readHandle.FlushTransmitQueue()
		k.writeHandle.FlushTransmitQueue()
		k.readHandle.Close()
		k.writeHandle.Close()
	})
	return nil
}

// Send writes one frame; WriteWait blocks until the frame is on the bus (or
// the write timeout hits), which is the v2 write confirmation.
func (k *CANlib) Send(ctx context.Context, f gocan.Frame) error {
	if err := k.writeHandle.WriteWait(f.ID, f.Bytes(), canlib.MSG_STD, k.timeoutWrite); err != nil {
		return fmt.Errorf("Send: %w", err)
	}
	return nil
}

func (k *CANlib) openChannels() (err error) {
	k.readHandle, err = canlib.OpenChannel(k.channel, canlib.OPEN_REQUIRE_INIT_ACCESS)
	if err != nil {
		return fmt.Errorf("OpenChannel error: %v", err)
	}
	k.writeHandle, err = canlib.OpenChannel(k.channel, canlib.OPEN_NO_INIT_ACCESS)
	if err != nil {
		k.readHandle.Close()
		return fmt.Errorf("OpenChannel error: %v", err)
	}
	return
}

// setSpeed maps the CAN rate to bus parameters. The 615.384 kbit/s
// SetBusParamsC200 path is not used on Windows (matches the v1 driver, where
// it is disabled); the default SetBitrate branch handles it approximately.
func (k *CANlib) setSpeed(canRate float64) error {
	var freq canlib.BusParamsFreq
	switch canRate {
	case 1000:
		freq = canlib.BITRATE_1M
	case 500:
		freq = canlib.BITRATE_500K
	case 250:
		freq = canlib.BITRATE_250K
	case 125:
		freq = canlib.BITRATE_125K
	case 100:
		freq = canlib.BITRATE_100K
	case 83:
		freq = canlib.BITRATE_83K
	case 62:
		freq = canlib.BITRATE_62K
	case 50:
		freq = canlib.BITRATE_50K
	case 10:
		freq = canlib.BITRATE_10K
	default:
		if err := k.readHandle.SetBitrate(int(canRate * 1000)); err != nil {
			return err
		}
		return k.writeHandle.SetBitrate(int(canRate * 1000))
	}
	if err := k.readHandle.SetBusParams(freq, 0, 0, 0, 0, 0); err != nil {
		return err
	}
	return k.writeHandle.SetBusParams(freq, 0, 0, 0, 0, 0)
}

// readLoop installs the RX notification callback and parks until shutdown;
// deliveries happen on the driver's callback thread.
func (k *CANlib) readLoop(ctx context.Context) {
	if err := k.readHandle.SetNotifyCallback(k.handleCallback, canlib.NOTIFY_RX); err != nil {
		k.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("set callback error: %v", err), Err: err})
	}
	defer k.readHandle.SetNotifyCallback(nil, canlib.NOTIFY_RX)
	<-ctx.Done()
}

func (k *CANlib) handleCallback(hhnd int32, cbctx uintptr, event canlib.NotifyFlag) uintptr {
	for {
		msg, err := k.readHandle.Read()
		if err != nil {
			if err == canlib.ErrNoMsg {
				break
			}
			k.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("recv error: %v", err), Err: err})
			return 0
		}
		if err := k.deliver(msg); err != nil {
			k.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
		}
	}
	return 0
}

func (k *CANlib) deliver(msg *canlib.CANMessage) error {
	if len(msg.Data) < int(msg.DLC) || msg.DLC > 8 {
		return errors.New("readLoop invalid data length")
	}
	f := gocan.Frame{
		ID:       uint32(msg.Identifier),
		Length:   uint8(msg.DLC),
		Extended: msg.Flags&uint32(canlib.MSG_EXT) != 0,
		Remote:   msg.Flags&uint32(canlib.MSG_RTR) != 0,
	}
	copy(f.Data[:], msg.Data[:msg.DLC])
	k.bus.Deliver(f)
	return nil
}
