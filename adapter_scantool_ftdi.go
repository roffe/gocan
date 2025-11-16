//go:build ftdi

package gocan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/roffe/gocan/pkg/ftdi"
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

	closed bool
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
	go scantoolSendManager(ctx, stn.port, &stn.BaseAdapter, stn.sendSem)
	//go scantoolSendManagerOld(ctx, stn.cfg.Debug, stn.port, stn.sendChan, stn.sendSem, stn.closeChan, stn.setError, stn.cfg.OnMessage)
	return nil
}

func (stn *ScantoolFTDI) Close() error {
	stn.closed = true
	stn.BaseAdapter.Close()
	scantoolReset(stn.port)
	stn.port.Purge(ftdi.FT_PURGE_BOTH)
	return stn.port.Close()
}
