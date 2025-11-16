package gocan

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sync"
)

type ADCCapable interface {
	GetADCValue(ctx context.Context, channel int) (float64, error)
}

type BaseAdapter struct {
	name               string
	cfg                *AdapterConfig
	sendChan, recvChan chan *CANFrame

	errOnce sync.Once
	errChan chan error

	evtChan chan Event

	closeChan chan struct{}
	closeOnce sync.Once
}

func NewBaseAdapter(name string, cfg *AdapterConfig) BaseAdapter {
	return BaseAdapter{
		name:      name,
		cfg:       cfg,
		sendChan:  make(chan *CANFrame, 40),
		recvChan:  make(chan *CANFrame, 1024),
		errChan:   make(chan error, 1),
		evtChan:   make(chan Event, 100),
		closeChan: make(chan struct{}),
	}
}

func (base *BaseAdapter) Name() string {
	return base.name
}

func (base *BaseAdapter) Send() chan<- *CANFrame {
	return base.sendChan
}

func (base *BaseAdapter) Recv() <-chan *CANFrame {
	return base.recvChan
}

func (base *BaseAdapter) Err() <-chan error {
	return base.errChan
}

func (base *BaseAdapter) Event() <-chan Event {
	return base.evtChan
}

func (base *BaseAdapter) Close() {
	base.closeOnce.Do(func() {
		close(base.closeChan)
	})
}

// setError set a fatal adapter error only once and non-blocking.
func (base *BaseAdapter) setError(err error) {
	base.errOnce.Do(func() {
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
	})
}

func (base *BaseAdapter) sendEvent(eventType EventType, details string) {
	select {
	case base.evtChan <- Event{Type: eventType, Details: details}:
	default:
		_, file, no, ok := runtime.Caller(1)
		if ok {
			log.Printf("%s#%d event channel full: %s\n", filepath.Base(file), no, details)
		} else {
			log.Printf("event channel full: %s", details)
		}
	}

}

func (base *BaseAdapter) sendErrorEvent(err error) {
	base.sendEvent(EventTypeError, err.Error())
}

func (base *BaseAdapter) sendWarningEvent(warn string) {
	base.sendEvent(EventTypeWarning, warn)
}

func (base *BaseAdapter) sendInfoEvent(info string) {
	base.sendEvent(EventTypeInfo, info)
}
