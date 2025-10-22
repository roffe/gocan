//go:build ftdi

package gocan

import (
	"bytes"
	"context"
	"fmt"
	"time"

	ftdi "github.com/roffe/goftdi"
)

type CanusbFTDI struct {
	BaseAdapter
	port         *ftdi.Device
	canRate      string
	filter, mask string
	buff         *bytes.Buffer
	sendSem      chan struct{}
	devIndex     uint64
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
	return canusbSetFilter(&cu.BaseAdapter, filters)
}

func (cu *CanusbFTDI) Open(ctx context.Context) error {
	if p, err := ftdi.Open(ftdi.DeviceInfo{
		Index: cu.devIndex,
	}); err != nil {
		return fmt.Errorf("failed to open ftdi device: %w", err)
	} else {
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
	}

	if err := canusbInit(cu.port, cu.canRate, cu.filter, cu.mask); err != nil {
		cu.port.Close()
		return err
	}

	cu.port.Purge(ftdi.FT_PURGE_RX)

	go cu.recvManager(ctx)
	go canusbSendManager(ctx, cu.closeChan, cu.sendSem, cu.port, cu.sendChan, cu.SetError, cu.cfg.OnMessage, cu.cfg.Debug)

	// Open the CAN channel
	cu.sendChan <- &CANFrame{
		Identifier: SystemMsg,
		Data:       []byte{'O'},
		FrameType:  Outgoing,
	}
	return nil
}

func (cu *CanusbFTDI) Close() error {
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

func (cu *CanusbFTDI) recvManager(ctx context.Context) {
	parseFn := canusbCreateParser(
		cu.cfg.Debug,
		cu.cfg.PrintVersion,
		cu.buff,
		cu.sendSem,
		cu.recvChan,
		cu.SetError,
		cu.cfg.OnMessage,
	)
	var rx_cnt int32
	var err error
	buf := make([]byte, 1024) // large enough for worst case
	for {
		select {
		case <-ctx.Done():
			return
		case <-cu.closeChan:
			return
		default:
			rx_cnt, err = cu.port.GetQueueStatus()
			if err != nil {
				cu.SetError(Unrecoverable(fmt.Errorf("failed to get queue status: %w", err)))
				return
			}
			if rx_cnt == 0 {
				time.Sleep(400 * time.Microsecond)
				continue
			}
			// Adjust slice length without reallocation
			readBuffer := buf[:rx_cnt]
			n, err := cu.port.Read(readBuffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				cu.SetError(fmt.Errorf("failed to read com port: %w", err))
				return
			}
			if n == 0 {
				continue
			}
			parseFn(readBuffer[:n])
		}
	}
}
