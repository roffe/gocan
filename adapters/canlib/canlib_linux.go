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
	txDrainTimeoutMs      = 1000
)

func init() {
	gocan.RegisterScanner(scanDevices)
}

func scanDevices() []gocan.AdapterInfo {
	if err := canlib.Init(); err != nil {
		return nil // Kvaser driver not installed
	}
	// re-running canInitializeLibrary refreshes CANlib's channel list so
	// devices plugged in after startup show up on rescan
	canlib.InitializeLibrary()
	channels, err := canlib.GetNumberOfChannels()
	if err != nil {
		return nil
	}
	var out []gocan.AdapterInfo
	for channel := range channels {
		devDescr, err := canlib.GetChannelDataString(channel, canlib.CHANNELDATA_DEVDESCR_ASCII)
		if err != nil {
			continue
		}
		if strings.HasPrefix(devDescr, "Kvaser Virtual") {
			continue
		}
		ch := channel
		out = append(out, gocan.AdapterInfo{
			Name:         fmt.Sprintf("CANlib #%d %v", channel, devDescr),
			Description:  "Canlib driver for Kvaser devices",
			Capabilities: gocan.Capabilities{HSCAN: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				return New(ch, cfg)
			},
		})
	}
	return out
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
	readDone     chan struct{} // closed when readLoop has left the driver

	// Driver calls force their buffers onto the heap; these live there
	// already, so the per-frame path doesn't allocate. tx is Send-only (the
	// Bus serializes Send), rx is readLoop-only.
	tx gocan.Frame
	rx canlib.CANMessage
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
	if err := canlib.Init(); err != nil {
		return fmt.Errorf("canlib: %w", err)
	}
	bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "CANlib v" + canlib.GetVersion()})

	if err := k.openChannels(); err != nil {
		return err
	}
	if err := k.setSpeed(k.cfg.CANRate); err != nil {
		err1 := k.readHandle.Close()
		err2 := k.writeHandle.Close()
		return fmt.Errorf("setSpeed: %v, RH: %v WH: %v", err, err1, err2)
	}

	k.readDone = make(chan struct{})
	go k.readLoop(ctx)

	if err := k.readHandle.BusOn(); err != nil {
		return err
	}
	return k.writeHandle.BusOn()
}

func (k *CANlib) Close() error {
	k.closeOnce.Do(func() {
		// The Bus cancelled ctx first, so readLoop exits within one ReadWait.
		// Closing the handles under an in-flight canReadWait left init access
		// held and an immediate reopen failed with "Access denied".
		if k.readDone != nil {
			<-k.readDone
		}
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

// Send queues the frame in the driver (canWrite) and returns, like
// TrionicCANLib does. WriteWait confirmed every frame on the wire, costing a
// USB round trip each: a T7 flash took ~60 s instead of ~25 s. A full driver
// queue is drained once and the write retried.
func (k *CANlib) Send(ctx context.Context, f gocan.Frame) error {
	k.tx = f
	flags := msgFlags(f)
	err := k.writeHandle.Write(f.ID, k.tx.Bytes(), flags)
	if errors.Is(err, canlib.ErrTxBufOfl) {
		if err = k.writeHandle.WriteSync(txDrainTimeoutMs); err == nil {
			err = k.writeHandle.Write(f.ID, k.tx.Bytes(), flags)
		}
	}
	if err != nil {
		return fmt.Errorf("Send: %w", err)
	}
	return nil
}

func msgFlags(f gocan.Frame) canlib.MsgFlag {
	flags := canlib.MSG_STD
	if f.Extended {
		flags = canlib.MSG_EXT
	}
	if f.Remote {
		flags |= canlib.MSG_RTR
	}
	return flags
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

func (k *CANlib) setSpeed(canRate float64) error {
	var freq canlib.BusParamsFreq
	switch canRate {
	case 1000:
		freq = canlib.BITRATE_1M
	case 615.384:
		if err := k.readHandle.SetBusParamsC200(0x40, 0x37); err != nil {
			return err
		}
		return k.writeHandle.SetBusParamsC200(0x40, 0x37)
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

func (k *CANlib) readLoop(ctx context.Context) {
	defer close(k.readDone)
	for ctx.Err() == nil {
		err := k.readHandle.ReadWait(&k.rx, k.timeoutRead)
		if err != nil {
			if err == canlib.ErrNoMsg || err == canlib.ErrTimeout {
				continue
			}
			if ctx.Err() == nil {
				k.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("recv error: %v", err), Err: err})
			}
			continue
		}
		if err := k.deliver(&k.rx); err != nil {
			k.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
		}
	}
}

func (k *CANlib) deliver(msg *canlib.CANMessage) error {
	if msg.DLC > 8 {
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
