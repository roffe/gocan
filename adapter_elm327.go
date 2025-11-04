package gocan

import (
	"context"
	"fmt"
	"time"

	"go.bug.st/serial"
)

/*
THIS IS NOT A FUNCTIONAL ADAPTER YET. WORK IN PROGRESS.

I have yet not been able to find a working ELM327 clone to implement this..
*/

type ELM327 struct {
	BaseAdapter
	port serial.Port
}

func NewELM327(cfg *AdapterConfig) (Adapter, error) {
	el := &ELM327{
		BaseAdapter: NewBaseAdapter("ELM327", cfg),
	}
	return el, nil
}

func (el *ELM327) Open(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: el.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(el.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", el.cfg.Port, err)
	}
	p.SetReadTimeout(3 * time.Millisecond)
	el.port = p
	p.ResetOutputBuffer()
	p.ResetInputBuffer()
	return nil
}

func (el *ELM327) Close() error {
	el.BaseAdapter.Close()
	if el.port != nil {
		el.port.Close()
		el.port = nil
	}
	return nil
}
