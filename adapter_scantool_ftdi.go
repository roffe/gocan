//go:build ftdi

package gocan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	ftdi "github.com/roffe/goftdi"
)

type ScantoolFTDI struct {
	BaseAdapter

	baseName     string
	canrateCMD   string
	protocolCMD  string
	filter, mask string
	sendSem      chan struct{}

	port     *ftdi.Device
	devIndex uint64
}

func NewScantoolFTDI(name string, idx uint64) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		stn := &ScantoolFTDI{
			BaseAdapter: NewBaseAdapter(name, cfg),
			devIndex:    idx,
			sendSem:     make(chan struct{}, 1),
			baseName:    strings.TrimPrefix(name, "d2xx "),
		}
		var err error
		stn.protocolCMD, stn.canrateCMD, err = scantoolCalculateCANrate(stn.baseName, cfg.CANRate)
		if err != nil {
			return nil, err
		}
		stn.filter, stn.mask = scantoolCANfilter(cfg.CANFilter)
		return stn, nil
	}
}

func (stn *ScantoolFTDI) SetFilter(filters []uint32) error {
	stn.filter, stn.mask = scantoolCANfilter(stn.cfg.CANFilter)
	return scantoolSetFilter(&stn.BaseAdapter, stn.filter, stn.mask)
}

func (stn *ScantoolFTDI) Open(ctx context.Context) error {
	var err error
	stn.port, err = ftdi.Open(ftdi.DeviceInfo{Index: stn.devIndex})
	if err != nil {
		return fmt.Errorf("failed to open ftdi device: %w", err)
	}

	if err := stn.port.SetLineProperty(ftdi.LineProperties{Bits: 8, StopBits: 0, Parity: ftdi.NONE}); err != nil {
		stn.port.Close()
		return err
	}

	if err := stn.port.SetLatency(1); err != nil {
		stn.port.Close()
		return err
	}

	if err := stn.port.SetTimeout(10, 10); err != nil {
		stn.port.Close()
		return err
	}

	to := uint(2000000)
	found := false
	resetInputBuffer := func() error {
		return stn.port.Purge(ftdi.FT_PURGE_RX)
	}
	speedSetter := func(baud int) error {
		return stn.port.SetBaudRate(uint(baud))
	}

	for _, from := range scantoolBaudrates {
		log.Println("trying to change baudrate from", from, "to", to, "bps")
		if err := scantoolTrySpeed(stn.port, from, to, speedSetter, resetInputBuffer, stn.cfg.OnMessage); err == nil {
			found = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !found {
		stn.port.Close()
		return errors.New("failed to switch adapter baudrate")
	}

	scantoolInit(stn.cfg.Debug, stn.port, stn.protocolCMD, stn.canrateCMD, stn.filter, stn.mask, stn.cfg.OnMessage)
	if err := stn.port.Purge(ftdi.FT_PURGE_RX); err != nil {
		stn.port.Close()
		return err
	}

	go stn.recvManager(ctx)
	go scantoolSendManager(ctx, stn.cfg.Debug, stn.port, stn.sendChan, stn.sendSem, stn.closeChan, stn.setError, stn.cfg.OnMessage)

	return nil
}

func (stn *ScantoolFTDI) Close() error {
	stn.BaseAdapter.Close()
	scantoolReset(stn.port)
	stn.port.Purge(ftdi.FT_PURGE_BOTH)
	return stn.port.Close()
}

func (stn *ScantoolFTDI) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	buf := make([]byte, 256)
	var rx_cnt int32
	var err error
	for {
		//select {
		//case <-ctx.Done():
		//	return
		//default:
		//}
		rx_cnt, err = stn.port.GetQueueStatus()
		if err != nil {
			stn.setError(fmt.Errorf("failed to get queue status: %w", err))
			return
		}
		if rx_cnt == 0 {
			time.Sleep(400 * time.Microsecond)
			continue
		}

		readBuffer := buf[:rx_cnt]
		n, err := stn.port.Read(readBuffer)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			stn.setError(fmt.Errorf("failed to read: %w", err))
			return
		}
		if n == 0 {
			continue
		}
		for _, b := range readBuffer[:n] {
			if b == '>' {
				select {
				case <-stn.sendSem:
				default:
				}
				continue
			}
			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if stn.cfg.Debug {
					stn.cfg.OnMessage("<i> " + buff.String())
				}
				switch buff.String() {
				case "CAN ERROR":
					//stn.cfg.OnMessage("CAN ERROR")
					stn.sendEvent(EventTypeError, "CAN ERROR")
					buff.Reset()
				case "STOPPED":
					//stn.cfg.OnMessage("STOPPED")
					stn.sendEvent(EventTypeInfo, "STOPPED")
					buff.Reset()
				case "?":
					//stn.cfg.OnMessage("UNKNOWN COMMAND")
					stn.sendEvent(EventTypeWarning, "UNKNOWN COMMAND")
					buff.Reset()
				case "NO DATA", "OK":
					buff.Reset()
				default:
					f, err := scantoolDecodeFrame(buff.Bytes())
					if err != nil {
						stn.cfg.OnMessage(fmt.Sprintf("failed to decode frame: %s %v", buff.String(), err))
						buff.Reset()
						continue
					}
					select {
					case stn.recvChan <- f:
					default:
						stn.sendErrorEvent(ErrDroppedFrame)
					}
					buff.Reset()
				}
				continue
			}
			buff.WriteByte(b)
		}
	}
}
