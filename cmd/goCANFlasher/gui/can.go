package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/pkg/ecu"
)

func (m *mainWindow) initCAN(ctx context.Context) (*gocan.Client, error) {
	startTime := time.Now()
	m.output("Init adapter")

	var filters []uint32
	switch state.ecuType {
	case ecu.Trionic5:
		filters = []uint32{0x000, 0x005, 0x006, 0x00C}
	case ecu.Trionic7:
		filters = []uint32{0x220, 0x238, 0x240, 0x258, 0x266}
	case ecu.Trionic8:
		filters = []uint32{0x011, 0x311, 0x7E0, 0x7E8, 0x5E8}
	}

	dev, err := adapter.New(
		state.adapter,
		&gocan.AdapterConfig{
			Port:         state.port,
			PortBaudrate: state.portBaudrate,
			CANRate:      state.canRate,
			CANFilter:    filters,
		})
	if err != nil {
		return nil, err
	}

	client, err := gocan.New(ctx, dev)
	if err != nil {
		return nil, err
	}

	m.output(fmt.Sprintf("Done, took: %s\n", time.Since(startTime).Round(time.Millisecond).String()))

	return client, nil
}
