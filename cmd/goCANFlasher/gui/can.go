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

	dev, err := adapter.New(
		state.adapter,
		&gocan.AdapterConfig{
			Port:         state.port,
			PortBaudrate: state.portBaudrate,
			CANRate:      state.canRate,
			CANFilter:    ecu.CANFilters(state.ecuType),
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
