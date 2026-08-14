package gocan

import "context"

func init() {
	Register(AdapterInfo{
		Name:        "loopback",
		Description: "virtual adapter that echoes sent frames back, for testing",
		New:         func(Config) (Adapter, error) { return &Loopback{}, nil },
	})
}

// Loopback is a virtual adapter that delivers every sent frame back to the
// bus, for tests and examples.
type Loopback struct {
	bus *Bus
}

func (l *Loopback) Open(_ context.Context, bus *Bus) error {
	l.bus = bus
	return nil
}

func (l *Loopback) Send(_ context.Context, f Frame) error {
	l.bus.Deliver(f)
	return nil
}

func (l *Loopback) Close() error { return nil }
