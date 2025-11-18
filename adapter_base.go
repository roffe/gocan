package gocan

import (
	"context"
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

	closeOnce sync.Once
	closeChan chan struct{}
}

func NewBaseAdapter(name string, cfg *AdapterConfig) *BaseAdapter {
	return &BaseAdapter{
		name:      name,
		cfg:       cfg,
		sendChan:  make(chan *CANFrame, 40),
		recvChan:  make(chan *CANFrame, 1024),
		errChan:   make(chan error, 1),
		evtChan:   make(chan Event, 100),
		closeChan: make(chan struct{}),
	}
}

// Name returns the adapter name.
func (base *BaseAdapter) Name() string {
	return base.name
}

// Return the send channel for the adapter
func (base *BaseAdapter) Send() chan<- *CANFrame {
	return base.sendChan
}

// Return the receive channel for the adapter
func (base *BaseAdapter) Recv() <-chan *CANFrame {
	return base.recvChan
}

// Return the error channel for the adapter
func (base *BaseAdapter) Err() <-chan error {
	return base.errChan
}

func (base *BaseAdapter) Event() <-chan Event {
	return base.evtChan
}

func (base *BaseAdapter) Close() {
	base.closeOnce.Do(func() {
		//log.Println("close baseAdapter")
		close(base.closeChan)
		select {
		case base.errChan <- nil:
		default:
			log.Println("failed to send <nil> to errchan")
		}
	})
}

// Set a fatal adapter error, meaning communication is broken and cannot continue.
func (base *BaseAdapter) Fatal(err error) {
	base.errOnce.Do(func() {
		select {
		case base.errChan <- err:
		default:
			_, file, no, ok := runtime.Caller(1)
			if ok {
				log.Printf("%s:%d error channel full: %v\n", filepath.Base(file), no, err)
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

// Send an error event
func (base *BaseAdapter) Error(err error) {
	base.sendEvent(EventTypeError, err.Error())
}

// Send a warning event
func (base *BaseAdapter) Warn(warn string) {
	base.sendEvent(EventTypeWarning, warn)
}

// Send an info event
func (base *BaseAdapter) Info(info string) {
	base.sendEvent(EventTypeInfo, info)
}

// Send a debug event
func (base *BaseAdapter) Debug(debug string) {
	base.sendEvent(EventTypeDebug, debug)
}
