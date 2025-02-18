package adapter

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/roffe/gocan"
)

type BaseAdapter struct {
	name               string
	cfg                *gocan.AdapterConfig
	sendChan, recvChan chan gocan.CANFrame
	errChan            chan error
	closeChan          chan struct{}
	closeOnce          sync.Once
}

func NewBaseAdapter(name string, cfg *gocan.AdapterConfig) BaseAdapter {
	return BaseAdapter{
		name:      name,
		cfg:       cfg,
		sendChan:  make(chan gocan.CANFrame, 10),
		recvChan:  make(chan gocan.CANFrame, 1024),
		errChan:   make(chan error, 10),
		closeChan: make(chan struct{}),
	}
}

func (base *BaseAdapter) Name() string {
	return base.name
}

func (base *BaseAdapter) Send() chan<- gocan.CANFrame {
	return base.sendChan
}

func (base *BaseAdapter) Recv() <-chan gocan.CANFrame {
	return base.recvChan
}

func (base *BaseAdapter) Err() <-chan error {
	return base.errChan
}

func (base *BaseAdapter) Close() {
	base.closeOnce.Do(func() {
		close(base.closeChan)
	})
}

func (base *BaseAdapter) SetError(err error) {
	select {
	case base.errChan <- err:
	default:
		_, file, no, ok := runtime.Caller(1)
		if ok {
			fmt.Printf("%s#%d error channel full: %v\n", filepath.Base(file), no, err)
		} else {
			log.Printf("error channel full: %v", err)
		}
	}
}
