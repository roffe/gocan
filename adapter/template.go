package adapter

import (
	"context"
	"sync"

	"github.com/roffe/gocan"
)

type Template struct {
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
	closeOnce  sync.Once
}

func NewTemplate(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Template{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 20),
		close: make(chan struct{}),
	}, nil
}

func (a *Template) SetFilter(filters []uint32) error {
	return nil
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
	a.closeOnce.Do(func() {
		close(a.close)
	})
	return nil
}
