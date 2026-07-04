package gocan

import (
	"context"
	"log"
	"path/filepath"
	"runtime"
	"sync"
)

// ADCCapable is implemented by adapters that expose analog inputs (e.g. the
// CombiAdapter's ADC channels).
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

	// syncCapable is set by adapters that call markSent after writing a frame to
	// hardware, enabling Client.SendSync. Adapters that don't set it fall back to
	// fire-and-forget sends.
	syncCapable bool
}

// SupportsSync reports whether this adapter confirms frame write-completion
// (i.e. calls markSent), which Client.SendSync relies on.
func (base *BaseAdapter) SupportsSync() bool {
	return base.syncCapable
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

// NewSyncBaseAdapter is NewBaseAdapter for adapters that call markSent after
// writing each frame to hardware, enabling Client.SendSync (see SupportsSync).
func NewSyncBaseAdapter(name string, cfg *AdapterConfig) *BaseAdapter {
	b := NewBaseAdapter(name, cfg)
	b.syncCapable = true
	return b
}

// Name returns the adapter name.
func (base *BaseAdapter) Name() string {
	return base.name
}

// Send returns the channel the client queues outgoing frames on.
func (base *BaseAdapter) Send() chan<- *CANFrame {
	return base.sendChan
}

// Recv returns the channel incoming frames are delivered on.
func (base *BaseAdapter) Recv() <-chan *CANFrame {
	return base.recvChan
}

// Err returns the channel a fatal adapter error is reported on.
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

// Fatal reports an unrecoverable adapter error: communication is broken and
// the client will terminate. Only the first call has any effect.
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

// emit delivers an event to the event channel without blocking. The channel is
// buffered; if it is full the event is dropped (and logged) rather than
// stalling the adapter. Fatal failures use Fatal/errChan and are never dropped.
func (base *BaseAdapter) emit(ev Event) {
	select {
	case base.evtChan <- ev:
	default:
		_, file, no, ok := runtime.Caller(2)
		if ok {
			log.Printf("%s#%d event channel full: %s\n", filepath.Base(file), no, ev.Details)
		} else {
			log.Printf("event channel full: %s", ev.Details)
		}
	}
}

func (base *BaseAdapter) sendEvent(eventType EventType, details string) {
	base.emit(Event{Type: eventType, Details: details})
}

// Error emits a recoverable error as an event.
func (base *BaseAdapter) Error(err error) {
	base.emit(Event{Type: EventTypeError, Details: err.Error(), Err: err})
}

// Warn emits a warning event.
func (base *BaseAdapter) Warn(warn string) {
	base.sendEvent(EventTypeWarning, warn)
}

// Info emits an informational event.
func (base *BaseAdapter) Info(info string) {
	base.sendEvent(EventTypeInfo, info)
}

// Debug emits a debug event.
func (base *BaseAdapter) Debug(debug string) {
	base.sendEvent(EventTypeDebug, debug)
}
