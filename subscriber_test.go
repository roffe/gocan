package gocan

import (
	"context"
	"testing"
	"time"
)

// passiveAdapter does nothing on its own; tests inject incoming frames by
// writing to recvChan directly.
type passiveAdapter struct{ *BaseAdapter }

func newPassiveAdapter() *passiveAdapter {
	return &passiveAdapter{NewBaseAdapter("passive", &AdapterConfig{})}
}

func (a *passiveAdapter) Open(ctx context.Context) error { return nil }
func (a *passiveAdapter) Close() error                   { a.BaseAdapter.Close(); return nil }

func recvOne(t *testing.T, ch <-chan *CANFrame) *CANFrame {
	t.Helper()
	select {
	case f := <-ch:
		return f
	case <-time.After(time.Second):
		t.Fatal("no frame delivered")
		return nil
	}
}

func requireClosed(t *testing.T, ch <-chan *CANFrame, what string) {
	t.Helper()
	select {
	case f, ok := <-ch:
		if ok {
			t.Fatalf("%s: unexpected frame %v", what, f)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s: channel not closed", what)
	}
}

// A global (no identifiers) subscription must close its channel on Close so a
// range over Chan terminates, just like a filtered one.
func TestGlobalSubscriberCloseClosesChannel(t *testing.T) {
	a := newPassiveAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	sub := cl.Subscribe(context.Background())
	a.recvChan <- NewFrame(0x123, []byte{1}, Incoming)
	if f := recvOne(t, sub.Chan()); f.Identifier != 0x123 {
		t.Fatalf("got frame 0x%X, want 0x123", f.Identifier)
	}

	sub.Close()
	requireClosed(t, sub.Chan(), "global sub after Close")
}

// Cancelling the context passed to Subscribe must release the subscription
// and close its channel.
func TestSubscribeCtxCancelClosesChannel(t *testing.T) {
	a := newPassiveAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sub := cl.Subscribe(ctx, 0x100)
	cancel()
	requireClosed(t, sub.Chan(), "filtered sub after ctx cancel")
}

// SubscribeChan must deliver to the caller's channel but never close it.
func TestSubscribeChanCallerOwnsChannel(t *testing.T) {
	a := newPassiveAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	ch := make(chan *CANFrame, 1)
	sub := cl.SubscribeChan(context.Background(), ch, 0x100)
	a.recvChan <- NewFrame(0x100, []byte{2}, Incoming)
	if f := recvOne(t, ch); f.Identifier != 0x100 {
		t.Fatalf("got frame 0x%X, want 0x100", f.Identifier)
	}

	sub.Close()
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("library closed a caller-owned channel")
		}
		t.Fatal("unexpected frame after Close")
	case <-time.After(100 * time.Millisecond):
		// still open and quiet: correct
	}
}
