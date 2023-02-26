package template

import (
	"context"

	"github.com/roffe/gocan"
)

type Adapter struct {
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
}

func New(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Adapter{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 100),
		recv:  make(chan gocan.CANFrame, 100),
		close: make(chan struct{}, 1),
	}, nil
}

func (a *Adapter) Init(ctx context.Context) error {
	return nil
}

func (a *Adapter) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *Adapter) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *Adapter) Close() error {
	return nil
}
