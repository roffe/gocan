//go:build canlib

package adapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/canlib"
)

const (
	defaultReadTimeoutMs  = 100
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

func init() {
	channels, err := canlib.GetNumberOfChannels()
	if err == nil {
		for channel := range channels {
			devDescr, err := canlib.GetChannelDataString(channel, canlib.CHANNELDATA_DEVDESCR_ASCII)
			if err != nil {
				panic(err)
			}
			if strings.HasPrefix(devDescr, "Kvaser Virtual") {
				continue
			}
			name := fmt.Sprintf("CANlib #%d %v", channel, devDescr)
			if err := gocan.RegisterAdapter(&gocan.AdapterInfo{
				Name:               name,
				Description:        "Canlib driver for Kvaser devices",
				RequiresSerialPort: false,
				Capabilities: gocan.AdapterCapabilities{
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
	readHandle   canlib.Handle
	writeHandle  canlib.Handle
	timeoutRead  uint32
	timeoutWrite uint32
	closeOnce    sync.Once
	// notifyChannel chan canlib.NotifyFlag
}

func NewCANlib(channel int, name string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		return &CANlib{
			channel:      channel,
			BaseAdapter:  NewBaseAdapter(name, cfg),
			timeoutRead:  defaultReadTimeoutMs,
			timeoutWrite: defaultWriteTimeoutMs,
			// notifyChannel: make(chan canlib.NotifyFlag, 100),
		}, nil
	}
}

func (k *CANlib) SetFilter(filters []uint32) error {
	return nil
}

func (k *CANlib) Close() error {
	k.BaseAdapter.Close()
	//if err := k.readHandle.SetNotifyCallback(nil, canlib.NOTIFY_RX); err != nil {
	//	log.Println("Kvaser.Close() set callback error:", err)
	//}
	k.closeOnce.Do(func() {
		if err := k.readHandle.BusOff(); err != nil {
			log.Println("CANlib.BusOff() off error:", err)
		}
		if err := k.writeHandle.BusOff(); err != nil {
			log.Println("CANlib.BusOff() off error:", err)
		}
		if err := k.readHandle.FlushReceiveQueue(); err != nil {
			log.Println("CANlib.FlushReceiveQueue() flush error:", err)
		}
		if err := k.writeHandle.FlushReceiveQueue(); err != nil {
			log.Println("CANlib.FlushReceiveQueue() flush error:", err)
		}
		if err := k.readHandle.FlushTransmitQueue(); err != nil {
			log.Println("CANlib.FlushTransmitQueue() flush error:", err)
		}
		if err := k.writeHandle.FlushTransmitQueue(); err != nil {
			log.Println("CANlib.FlushTransmitQueue() flush error:", err)
		}
		if err := k.readHandle.Close(); err != nil {
			log.Println("CANlib.Close() close error:", err)
		}
		if err := k.writeHandle.Close(); err != nil {
			log.Println("CANlib.Close() close error:", err)
		}
	})
	return nil
}

func (k *CANlib) Open(ctx context.Context) error {
	if k.cfg.PrintVersion {
		k.cfg.OnMessage("CANlib v" + canlib.GetVersion())
	}

	if err := k.openChannels(); err != nil {
		return err
	}

	if err := k.setSpeed(k.cfg.CANRate); err != nil {
		err1 := k.readHandle.Close()
		err2 := k.writeHandle.Close()
		return fmt.Errorf("setSpeed: %v, RH: %v WH: %v", err, err1, err2)
	}

	// if err := canlib.SetBusOutputControl(k.readHandle, canlib.DRIVER_NORMAL); err != nil {
	// 	return fmt.Errorf("setBusOutputControl: %v", err)
	// }
	// if err := canlib.SetBusOutputControl(k.writeHandle, canlib.DRIVER_NORMAL); err != nil {
	// 	return fmt.Errorf("setBusOutputControl: %v", err)
	// }

	go k.sendManager(ctx)
	go k.recvManager(ctx)

	if err := k.readHandle.BusOn(); err != nil {
		return err
	}

	return k.writeHandle.BusOn()
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

func (k *CANlib) setSpeed(CANRate float64) error {
	var freq canlib.BusParamsFreq

	switch CANRate {
	case 1000:
		freq = canlib.BITRATE_1M
	/*
		case 615.384: // Trionic 5 is special ;)
		//return k.handle.SetBusParamsC200(0x40, 0x37)
		//return k.handle.SetBitrate(615384)
	*/
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
			if msg.Identifier >= gocan.SystemMsg {
				continue
			}
			if err := k.writeHandle.WriteWait(msg.Identifier, msg.Data, canlib.MSG_STD, k.timeoutWrite); err != nil {
				k.SetError(fmt.Errorf("kvaser sendMessage error: %w", err))
			}
		}
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
				if err == canlib.ErrNoMsg {
					continue
				}
				k.SetError(gocan.Unrecoverable(fmt.Errorf("kvaser recvManager error: %v", err)))
				return
			}
			if err := k.recvMessage(msg); err != nil {
				k.SetError(err)
			}
		}
	}
}

func (k *CANlib) recvMessage(msg *canlib.CANMessage) error {
	if len(msg.Data) < int(msg.DLC) {
		return errors.New("kvaser recvManager invalid data length")
	}
	frame := gocan.NewFrame(uint32(msg.Identifier), msg.Data[:msg.DLC], gocan.Incoming)
	if msg.Flags&uint32(canlib.MSG_EXT) != 0 {
		frame.Extended = true
	}
	if msg.Flags&uint32(canlib.MSG_RTR) != 0 {
		frame.RTR = true
	}
	select {
	case k.recvChan <- frame:
		return nil
	default:
		return errors.New("kvaser recvManager dropped frame")
	}
}
