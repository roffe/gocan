//go:build canlib

package adapter

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocanlib"
)

const (
	defaultReadTimeoutMs  = 100
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

func init() {
	if err := gocanlib.InitializeLibrary(); err != nil {
		log.Println("canlib InitializeLibrary() error:", err)
		return
	}
	channels, err := gocanlib.GetNumberOfChannels()
	if err == nil {
		for channel := range channels {
			devDescr, err := gocanlib.GetChannelDataString(channel, gocanlib.CHANNELDATA_DEVDESCR_ASCII)
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
				New: NewCANlib(channel, name),
			}); err != nil {
				panic(err)
			}
		}
	}
}

var _ gocan.Adapter = (*CANlib)(nil)

type CANlib struct {
	BaseAdapter
	channel      int
	readHandle   gocanlib.Handle
	writeHandle  gocanlib.Handle
	timeoutRead  uint32
	timeoutWrite uint32

	// notifyChannel chan gocanlib.NotifyFlag
}

func NewCANlib(channel int, name string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		return &CANlib{
			channel:      channel,
			BaseAdapter:  NewBaseAdapter(name, cfg),
			timeoutRead:  defaultReadTimeoutMs,
			timeoutWrite: defaultWriteTimeoutMs,
			// notifyChannel: make(chan gocanlib.NotifyFlag, 100),
		}, nil
	}
}

func (k *CANlib) SetFilter(filters []uint32) error {
	return nil
}

func (k *CANlib) Close() error {
	k.BaseAdapter.Close()
	//if err := k.readHandle.SetNotifyCallback(nil, gocanlib.NOTIFY_RX); err != nil {
	//	log.Println("Kvaser.Close() set callback error:", err)
	//}
	if err := k.readHandle.BusOff(); err != nil {
		log.Println("Kvaser.Close() off error:", err)
	}
	if err := k.readHandle.FlushReceiveQueue(); err != nil {
		log.Println("Kvaser.Close() flush error:", err)
	}
	if err := k.readHandle.FlushTransmitQueue(); err != nil {
		log.Println("Kvaser.Close() flush error:", err)
	}
	if err := k.readHandle.Close(); err != nil {
		log.Println("Kvaser.Close() close error:", err)
	}
	if err := k.writeHandle.BusOff(); err != nil {
		log.Println("Kvaser.Close() off error:", err)
	}
	if err := k.writeHandle.FlushReceiveQueue(); err != nil {
		log.Println("Kvaser.Close() flush error:", err)
	}
	if err := k.writeHandle.FlushTransmitQueue(); err != nil {
		log.Println("Kvaser.Close() flush error:", err)
	}
	if err := k.writeHandle.Close(); err != nil {
		log.Println("Kvaser.Close() close error:", err)
	}
	return nil
}

func (k *CANlib) Connect(ctx context.Context) error {
	//if k.cfg.PrintVersion {
	//	log.Printf("Canlib v" + gocanlib.GetVersion())
	//}

	if err := k.openChannels(); err != nil {
		return err
	}

	if err := k.setSpeed(k.cfg.CANRate); err != nil {
		err1 := k.readHandle.Close()
		err2 := k.writeHandle.Close()
		return fmt.Errorf("setSpeed: %v, RH: %v WH: %v", err, err1, err2)
	}

	// if err := gocanlib.SetBusOutputControl(k.readHandle, gocanlib.DRIVER_NORMAL); err != nil {
	// 	return fmt.Errorf("setBusOutputControl: %v", err)
	// }
	// if err := gocanlib.SetBusOutputControl(k.writeHandle, gocanlib.DRIVER_NORMAL); err != nil {
	// 	return fmt.Errorf("setBusOutputControl: %v", err)
	// }

	//if err := k.readHandle.SetNotifyCallback(k.handleCallback, gocanlib.NOTIFY_RX); err != nil {
	//	k.readHandle.Close()
	//	k.writeHandle.Close()
	//	return fmt.Errorf("SetNotifyCallback: %w", err)
	//}

	//go k.manager(ctx)
	go k.sendManager(ctx)
	go k.recvManager(ctx)

	if err := k.readHandle.BusOn(); err != nil {
		return err
	}

	return k.writeHandle.BusOn()
}

/*
func (k *CANlib) handleCallback(hnd int32, ctx uintptr, event gocanlib.NotifyFlag) uintptr {
	select {
	case k.notifyChannel <- event:
	default:
		k.SetError(fmt.Errorf("Callback: hnd=%d, ctx=%d, event=%d dropped", hnd, ctx, event))
	}
	return 0
}
*/

func (k *CANlib) openChannels() (err error) {
	k.readHandle, err = gocanlib.OpenChannel(k.channel, gocanlib.OPEN_REQUIRE_INIT_ACCESS)
	if err != nil {
		return fmt.Errorf("OpenChannel error: %v", err)
	}
	k.writeHandle, err = gocanlib.OpenChannel(k.channel, gocanlib.OPEN_NO_INIT_ACCESS)
	if err != nil {
		k.readHandle.Close()
		return fmt.Errorf("OpenChannel error: %v", err)
	}
	return
}

func (k *CANlib) setSpeed(CANRate float64) error {
	var freq gocanlib.BusParamsFreq

	switch CANRate {
	case 1000:
		freq = gocanlib.BITRATE_1M
	/*
		case 615.384: // Trionic 5 is special ;)
		//return k.handle.SetBusParamsC200(0x40, 0x37)
		//return k.handle.SetBitrate(615384)
	*/
	case 500:
		freq = gocanlib.BITRATE_500K
	case 250:
		freq = gocanlib.BITRATE_250K
	case 125:
		freq = gocanlib.BITRATE_125K
	case 100:
		freq = gocanlib.BITRATE_100K
	case 83:
		freq = gocanlib.BITRATE_83K
	case 62:
		freq = gocanlib.BITRATE_62K
	case 50:
		freq = gocanlib.BITRATE_50K
	case 10:
		freq = gocanlib.BITRATE_10K
	default:
		if err := k.readHandle.SetBitrate(int(k.cfg.CANRate * 1000)); err != nil {
			return err
		}
		return k.writeHandle.SetBitrate(int(k.cfg.CANRate * 1000))
	}
	if err := k.readHandle.SetBusParams(freq, 0, 0, 0, 0, 0); err != nil {
		return err
	}
	return k.writeHandle.SetBusParams(freq, 0, 0, 0, 0, 0)
}

/*
func (k *Kvaser) manager(ctx context.Context) {
	if k.cfg.Debug {
		defer log.Println("kvaser sendManager exited")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.closeChan:
			return
		case evt := <-k.notifyChannel:
			switch evt {
			case gocanlib.NOTIFY_RX:
				var msg *gocanlib.CANMessage
				var err error
				for err == nil {
					msg, err = k.readHandle.Read()
					if err != nil {
						if err == gocanlib.ErrNoMsg {
							break
						}
						k.SetError(fmt.Errorf("gocanlib.Read error: %w", err))
						break
					}
					k.recvMessage(msg)
				}

			}
		case frame := <-k.sendChan:
			k.sendMessage(frame)
		}
	}
}
*/

func (k *CANlib) sendManager(ctx context.Context) {
	if k.cfg.Debug {
		defer log.Println("kvaser sendManager exited")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.closeChan:
			return
		case msg := <-k.sendChan:
			k.sendMessage(msg)
		}
	}
}

func (k *CANlib) sendMessage(msg *gocan.CANFrame) {
	if msg.Identifier >= gocan.SystemMsg {
		return
	}
	if err := k.writeHandle.WriteWait(msg.Identifier, msg.Data, gocanlib.MSG_STD, k.timeoutWrite); err != nil {
		k.SetError(fmt.Errorf("kvaser sendManager error: %v", err))
	}
}

func (k *CANlib) recvManager(ctx context.Context) {
	if k.cfg.Debug {
		defer log.Println("kvaser recvManager exited")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.closeChan:
			return
		default:
			msg, err := k.readHandle.ReadWait(k.timeoutRead)
			if err != nil {
				if err != gocanlib.ErrNoMsg {
					continue
				}
				k.SetError(fmt.Errorf("kvaser.recvManager() error: %v", err))
				return
			}
			k.recvMessage(msg)
		}
	}
}

func (k *CANlib) recvMessage(msg *gocanlib.CANMessage) {
	if len(msg.Data) < int(msg.DLC) {
		k.SetError(errors.New("kvaser recvManager invalid data length"))
		return
	}
	frame := gocan.NewFrame(uint32(msg.Identifier), msg.Data[:msg.DLC], gocan.Incoming)
	if msg.Flags&uint32(gocanlib.MSG_EXT) != 0 {
		frame.Extended = true
	}
	select {
	case k.recvChan <- frame:
	default:
		k.SetError(errors.New("kvaser recvManager dropped frame"))
	}
}
