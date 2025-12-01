//go:build ftdi

package gocan

import (
	"bytes"
	"context"
	"fmt"
	"time"

	ftdi "github.com/roffe/gocan/pkg/ftdi"
)

type CanusbFTDI struct {
	*BaseAdapter
	port         *ftdi.Device
	canRate      string
	filter, mask string
	buff         *bytes.Buffer
	sendSem      chan struct{}
	devIndex     uint64
	closed       bool
}

func NewCanusbFTDI(name string, index uint64) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		cu := &CanusbFTDI{
			BaseAdapter: NewBaseAdapter(name, cfg),
			buff:        bytes.NewBuffer(nil),
			sendSem:     make(chan struct{}, 1),
			devIndex:    index,
		}
		rate, err := canusbCANrate(cfg.CANRate)
		if err != nil {
			return nil, err
		}
		cu.canRate = rate
		cu.filter, cu.mask = canusbAcceptanceFilters(cfg.CANFilter)
		return cu, nil
	}
}

func (cu *CanusbFTDI) SetFilter(filters []uint32) error {
	return canusbSetFilter(cu.BaseAdapter, filters)
}

func (cu *CanusbFTDI) Open(ctx context.Context) error {
	p, err := ftdi.Open(ftdi.DeviceInfo{
		Index: cu.devIndex,
	})
	if err != nil {
		return fmt.Errorf("failed to open ftdi device: %w", err)
	}
	cu.port = p
	if err := p.SetLineProperty(ftdi.LineProperties{Bits: 8, StopBits: 0, Parity: ftdi.NONE}); err != nil {
		p.Close()
		return err
	}

	if err := p.SetBaudRate(3000000); err != nil {
		p.Close()
		return err
	}

	if err := p.SetLatency(1); err != nil {
		p.Close()
		return err
	}

	if err := p.SetTimeout(4, 4); err != nil {
		p.Close()
		return err
	}

	p.Write([]byte{'C', '\r'})
	time.Sleep(100 * time.Millisecond)
	p.Purge(ftdi.FT_PURGE_RX)

	parseFn := canusbCreateParser(cu.buff, cu.BaseAdapter, cu.sendSem)
	go cu.recvManager(ctx, parseFn)

	if err := canusbInit(cu.port, cu.canRate, cu.filter, cu.mask); err != nil {
		cu.port.Close()
		return err
	}

	cu.port.Purge(ftdi.FT_PURGE_RX)

	go canusbSendManager(ctx, cu.BaseAdapter, cu.sendSem, cu.port)

	// Open the CAN channel
	cu.sendChan <- &CANFrame{
		Identifier: SystemMsg,
		Data:       []byte{'O'},
		FrameType:  Outgoing,
	}
	return nil
}

func (cu *CanusbFTDI) Close() error {
	cu.closed = true
	cu.BaseAdapter.Close()
	if cu.port != nil {
		if err := canusbClose(cu.port); err != nil {
			return fmt.Errorf("canusb ftdi close error: %w", err)
		}
		cu.port.Purge(ftdi.FT_PURGE_BOTH)
		if err := cu.port.Close(); err != nil {
			return fmt.Errorf("failed to close com port: %w", err)
		}
		cu.port = nil
	}
	return nil
}
