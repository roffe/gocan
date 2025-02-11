package adapter

import (
	"sync"

	"github.com/roffe/gocan"
)

type BaseAdapter struct {
	name       string
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	err        chan error
	close      chan struct{}
	once       sync.Once
}

func NewBaseAdapter(name string, cfg *gocan.AdapterConfig) BaseAdapter {
	return BaseAdapter{
		name:  name,
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 40),
		recv:  make(chan gocan.CANFrame, 40),
		err:   make(chan error, 5),
		close: make(chan struct{}),
	}
}

func (a *BaseAdapter) Name() string {
	return a.name
}

func (a *BaseAdapter) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *BaseAdapter) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *BaseAdapter) Err() <-chan error {
	return a.err
}

func (a *BaseAdapter) Close() {
	a.once.Do(func() {
		close(a.close)
	})
}
