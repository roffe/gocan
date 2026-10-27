// Package gocantest provides a fake CAN node for testing code that talks to
// ECUs over a gocan.Bus, with faults injected on demand: dropped, duplicated,
// truncated or late replies, and an adapter that is unplugged mid-session.
package gocantest

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

// Node is a scripted CAN node and the gocan.Adapter that connects a Bus to
// it. Every frame sent on the bus is recorded and handed to Reply; the frames
// Reply returns are delivered back through Fault, after Delay. Set the fields
// before the calls they should affect.
type Node struct {
	// Reply answers one frame the tester sent; nil answers nothing.
	Reply func(req gocan.Frame) []gocan.Frame
	// Fault rewrites reply frame n, counted from 0 over the whole session,
	// before delivery: nothing drops it, two frames duplicate it, a changed
	// frame corrupts it.
	Fault func(n int, f gocan.Frame) []gocan.Frame
	// Delay holds replies back and delivers them from another goroutine;
	// longer than the tester's timeout makes them late replies.
	Delay time.Duration

	bus     *gocan.Bus
	mu      sync.Mutex
	sent    []gocan.Frame
	replies int
}

// Open connects a Bus to node and closes it when the test ends.
func Open(tb testing.TB, node *Node) *gocan.Bus {
	tb.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), node)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { bus.Close() })
	return bus
}

func (n *Node) Open(_ context.Context, bus *gocan.Bus) error {
	n.bus = bus
	return nil
}

func (n *Node) Close() error { return nil }

func (n *Node) Send(_ context.Context, f gocan.Frame) error {
	n.mu.Lock()
	n.sent = append(n.sent, f)
	n.mu.Unlock()
	if n.Reply == nil {
		return nil
	}
	var out []gocan.Frame
	for _, r := range n.Reply(f) {
		n.mu.Lock()
		i := n.replies
		n.replies++
		n.mu.Unlock()
		if n.Fault == nil {
			out = append(out, r)
		} else {
			out = append(out, n.Fault(i, r)...)
		}
	}
	if n.Delay > 0 {
		time.AfterFunc(n.Delay, func() { n.Deliver(out...) })
		return nil
	}
	n.Deliver(out...)
	return nil
}

// Deliver puts frames on the bus as if the node sent them unprompted.
func (n *Node) Deliver(frames ...gocan.Frame) {
	for _, f := range frames {
		n.bus.Deliver(f)
	}
}

// Sent returns the frames the tester has sent so far.
func (n *Node) Sent() []gocan.Frame {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.sent)
}

// Unplug ends the bus the way a removed adapter does.
func (n *Node) Unplug(err error) { n.bus.Fatal(err) }

// Drop is a Fault that loses reply frame k.
func Drop(k int) func(int, gocan.Frame) []gocan.Frame {
	return func(n int, f gocan.Frame) []gocan.Frame {
		if n == k {
			return nil
		}
		return []gocan.Frame{f}
	}
}

// Duplicate is a Fault that delivers reply frame k twice.
func Duplicate(k int) func(int, gocan.Frame) []gocan.Frame {
	return func(n int, f gocan.Frame) []gocan.Frame {
		if n == k {
			return []gocan.Frame{f, f}
		}
		return []gocan.Frame{f}
	}
}

// Truncate is a Fault that cuts reply frame k to length data bytes.
func Truncate(k int, length uint8) func(int, gocan.Frame) []gocan.Frame {
	return func(n int, f gocan.Frame) []gocan.Frame {
		if n == k && f.Length > length {
			f.Length = length
			clear(f.Data[length:])
		}
		return []gocan.Frame{f}
	}
}
