//go:build canlib

package adapter

import (
	"context"
	"fmt"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/canlib"
)

const (
	defaultReadTimeoutMs  = 20
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

func (k *Kvaser) Close() error {
	//	log.Println("Kvaser.Close()")
	if err := k.handle.BusOff(); err != nil {
		log.Println("Kvaser.Close() off error:", err)
	}
	if err := k.handle.FlushReceiveQueue(); err != nil {
		log.Println("Kvaser.Close() flush error:", err)
	}
	if err := k.handle.FlushTransmitQueue(); err != nil {
		log.Println("Kvaser.Close() flush error:", err)
	}
	k.BaseAdapter.Close()
	return k.handle.Close()
}

func (k *Kvaser) Connect(ctx context.Context) error {
	//if k.cfg.PrintVersion {
	//	log.Printf("Canlib v" + canlib.GetVersion())
	//}

	if err := k.openChannel(); err != nil {
		return err
	}

	if err := k.setSpeed(k.cfg.CANRate); err != nil {
		closeErr := k.handle.Close()
		return fmt.Errorf("setSpeed: %v, Close: %v", err, closeErr)
	}

	if err := canlib.SetBusOutputControl(k.handle, canlib.DRIVER_NORMAL); err != nil {
		return fmt.Errorf("setBusOutputControl: %v", err)
	}

	go k.sendManager(ctx)
	go k.recvManager(ctx)

	return k.handle.BusOn()
}

func (k *Kvaser) openChannel() (err error) {
	k.handle, err = canlib.OpenChannel(k.channel, canlib.OPEN_EXCLUSIVE)
	if err != nil {
		return fmt.Errorf("OpenChannel error: %v", err)
	}
	return
}

func (k *Kvaser) setSpeed(CANRate float64) error {
	switch CANRate {
	case 1000:
		return k.handle.SetBusParams(canlib.BITRATE_1M, 0, 0, 0, 0, 0)
	/*
		case 615.384: // Trionic 5 is special ;)
		//return k.handle.SetBusParamsC200(0x40, 0x37)
		//return k.handle.SetBitrate(615384)
	*/
	case 500:
		return k.handle.SetBusParams(canlib.BITRATE_500K, 0, 0, 0, 0, 0)
	case 250:
		return k.handle.SetBusParams(canlib.BITRATE_250K, 0, 0, 0, 0, 0)
	case 125:
		return k.handle.SetBusParams(canlib.BITRATE_125K, 0, 0, 0, 0, 0)
	case 100:
		return k.handle.SetBusParams(canlib.BITRATE_100K, 0, 0, 0, 0, 0)
	case 83:
		return k.handle.SetBusParams(canlib.BITRATE_83K, 0, 0, 0, 0, 0)
	case 62:
		return k.handle.SetBusParams(canlib.BITRATE_62K, 0, 0, 0, 0, 0)
	case 50:
		return k.handle.SetBusParams(canlib.BITRATE_50K, 0, 0, 0, 0, 0)
	case 10:
		return k.handle.SetBusParams(canlib.BITRATE_10K, 0, 0, 0, 0, 0)
	default:
		return k.handle.SetBitrate(int(k.cfg.CANRate * 1000))
	}
}

func (k *Kvaser) sendManager(ctx context.Context) {
	if k.cfg.Debug {
		defer log.Println("kvaser.sendManager() done")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.closeChan:
			return
		case frame := <-k.sendChan:
			if frame.Identifier() >= gocan.SystemMsg {
				continue
			}
			k.sendMessage(frame)
		}
	}
}

func (k *Kvaser) sendMessage(frame gocan.CANFrame) {
	if err := k.handle.Write(frame.Identifier(), frame.Data(), canlib.MSG_STD); err != nil {
		k.SetError(fmt.Errorf("kvaser.sendManager error: %v", err))
	}
}

func (k *Kvaser) recvManager(ctx context.Context) {
	if k.cfg.Debug {
		defer log.Println("kvaser.recvManager() done")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.closeChan:
			return
		default:
			msg, err := k.handle.ReadWait(k.timeoutRead)
			if err != nil && err != canlib.ErrNoMsg {
				k.SetError(fmt.Errorf("kvaser.recvManager() error: %v", err))
				return
			}
			if msg == nil {
				continue
			}
			k.recvMessage(msg.Identifier, msg.Data, msg.Dlc)
		}
	}
}

func (k *Kvaser) recvMessage(identifier uint32, data []byte, dlc uint32) {
	if len(data) < int(dlc) {
		log.Println("kvaser.recvManager() invalid data length")
		return
	}
	frame := gocan.NewFrame(identifier, data[:dlc], gocan.Incoming)
	select {
	case k.recvChan <- frame:
	default:
		log.Println("kvaser.recvManager() dropped frame")
	}
}
