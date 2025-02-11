//go:build kvaser

package adapter

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/canlib"
)

const (
	defaultReadTimeoutMs  = 50
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

func init() {
	//	log.Println("Kvaser adapter init")
	canlib.InitializeLibrary()
	channels, err := canlib.GetNumberOfChannels()
	if err == nil {
		for channel := range channels {
			devDescr, err := canlib.GetChannelDataString(channel, canlib.CHANNELDATA_DEVDESCR_ASCII)
			if err != nil {
				panic(err)
			}
			name := fmt.Sprintf("CANlib #%d %v", channel, devDescr)
			if err := Register(&AdapterInfo{
				Name:               name,
				Description:        "Canlib driver for Kvaser devices",
				RequiresSerialPort: false,
				Capabilities: AdapterCapabilities{
					HSCAN: true,
					KLine: false,
					SWCAN: false,
				},
				New: NewKvaser(channel, name),
			}); err != nil {
				panic(err)
			}
		}
	}
}

var _ gocan.Adapter = (*Kvaser)(nil)

type Kvaser struct {
	BaseAdapter
	channel      int
	handle       canlib.Handle
	timeoutRead  int
	timeoutWrite int
}

func NewKvaser(channel int, name string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		return &Kvaser{
			channel:      channel,
			BaseAdapter:  NewBaseAdapter(name, cfg),
			timeoutRead:  defaultReadTimeoutMs,
			timeoutWrite: defaultWriteTimeoutMs,
		}, nil
	}
}

func (k *Kvaser) SetFilter(filters []uint32) error {
	return nil
}

func (k *Kvaser) Connect(ctx context.Context) error {
	if k.cfg.PrintVersion {
		log.Printf("Canlib v" + canlib.GetVersion())
	}

	flags := canlib.OPEN_EXCLUSIVE

	handle, err := canlib.OpenChannel(k.channel, flags)
	if err != nil {
		return fmt.Errorf("OpenChannel error: %v", err)
	}

	k.handle = handle

	switch k.cfg.CANRate {
	case 10:
		err = k.handle.SetBusParams(canlib.BITRATE_10K, 0, 0, 0, 0, 0)
	case 50:
		err = k.handle.SetBusParams(canlib.BITRATE_50K, 0, 0, 0, 0, 0)
	case 62:
		err = k.handle.SetBusParams(canlib.BITRATE_62K, 0, 0, 0, 0, 0)
	case 83:
		err = k.handle.SetBusParams(canlib.BITRATE_83K, 0, 0, 0, 0, 0)
	case 100:
		err = k.handle.SetBusParams(canlib.BITRATE_100K, 0, 0, 0, 0, 0)
	case 125:
		err = k.handle.SetBusParams(canlib.BITRATE_125K, 0, 0, 0, 0, 0)
	case 250:
		err = k.handle.SetBusParams(canlib.BITRATE_250K, 0, 0, 0, 0, 0)
	case 500:
		err = k.handle.SetBusParams(canlib.BITRATE_500K, 0, 0, 0, 0, 0)
	case 615.384:
		err = k.handle.SetBusParamsC200(0x40, 0x37)
	case 1000:
		err = k.handle.SetBusParams(canlib.BITRATE_1M, 0, 0, 0, 0, 0)
	default:
		return errors.New("unsupported CAN rate")
	}
	if err != nil {
		return fmt.Errorf("SetBusParams error: %v", err)
	}
	if err := canlib.SetBusOutputControl(k.handle, canlib.DRIVER_NORMAL); err != nil {
		return fmt.Errorf("SetBusOutputControl error: %v", err)
	}

	go k.recvManager(ctx)
	go k.sendManager(ctx)

	return k.handle.BusOn()
}

func (k *Kvaser) recvManager(ctx context.Context) {
	defer log.Println("Kvaser.recvManager() done")
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.close:
			return
		default:
			msg, err := k.handle.ReadWait(k.timeoutRead)
			if err != nil && err.Error() != canlib.ErrNoMsg.Error() {
				k.err <- fmt.Errorf("Kvaser.recvManager() error: %v", err)
				return
			}
			if msg == nil {
				continue
			}
			frame := gocan.NewFrame(msg.Identifier, msg.Data[:msg.DLC], gocan.Incoming)
			select {
			case k.recv <- frame:
			default:
				log.Println("Kvaser.recvManager() dropped frame")
			}
		}
	}
}

func (k *Kvaser) sendManager(ctx context.Context) {
	defer log.Println("Kvaser.sendManager() done")
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.close:
			return
		case frame := <-k.send:
			if err := k.handle.Write(frame.Identifier(), frame.Data(), canlib.MSG_STD); err != nil {
				k.err <- fmt.Errorf("Kvaser.sendManager() error: %v", err)
				return
			}
		}
	}
}

func (k *Kvaser) Close() error {
	log.Println("Kvaser.Close()")
	k.BaseAdapter.Close()
	if err := k.handle.BusOff(); err != nil {
		log.Println("Kvaser.Close() off error:", err)
	}
	return k.handle.Close()
}
