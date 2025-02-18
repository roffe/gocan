package adapter

import (
	"log"
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
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 1024),
		err:   make(chan error, 10),
		close: make(chan struct{}),
	}
}

func (base *BaseAdapter) Name() string {
	return base.name
}

func (base *BaseAdapter) Send() chan<- gocan.CANFrame {
	return base.send
}

func (base *BaseAdapter) Recv() <-chan gocan.CANFrame {
	return base.recv
}

func (base *BaseAdapter) Err() <-chan error {
	return base.err
}

func (base *BaseAdapter) Close() {
	base.once.Do(func() {
		close(base.close)
	})
}

func (base *BaseAdapter) SetError(err error) {
	select {
	case base.err <- err:
	default:
		log.Println("adapter error channel full")
	}
}
