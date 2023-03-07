package adapter

import (
	"context"

	"github.com/roffe/gocan"
)

type Template struct {
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
}

func NewTemplate(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Template{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 100),
		recv:  make(chan gocan.CANFrame, 100),
		close: make(chan struct{}, 1),
	}, nil
}

func (a *Template) Name() string {
	return "Template"
}

func (a *Template) Init(ctx context.Context) error {
	return nil
}

func (a *Template) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *Template) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *Template) Close() error {
	return nil
}
