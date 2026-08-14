package gocan

import (
	"context"
	"errors"
	"testing"
	"time"
)

func openLoopback(t *testing.T) *Bus {
	t.Helper()
	bus, err := Open(context.Background(), "loopback", Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	return bus
}

func TestSubscribeFilters(t *testing.T) {
	bus := openLoopback(t)
	ctx := context.Background()

	ch := bus.Subscribe(ctx, 0x123)
	all := bus.Subscribe(ctx)

	if err := bus.Send(ctx, NewFrame(0x456, []byte{4})); err != nil {
		t.Fatal(err)
	}
	if err := bus.Send(ctx, NewFrame(0x123, []byte{1, 2, 3})); err != nil {
		t.Fatal(err)
	}

	f := <-ch
	if f.ID != 0x123 || f.Length != 3 || f.Data != [8]byte{1, 2, 3} {
		t.Fatalf("filtered sub got wrong frame: %s", f)
	}
	if f := <-all; f.ID != 0x456 {
		t.Fatalf("global sub expected 0x456 first, got %s", f)
	}
	if f := <-all; f.ID != 0x123 {
		t.Fatalf("global sub expected 0x123 second, got %s", f)
	}
}

func TestRequest(t *testing.T) {
	bus := openLoopback(t)
	reply, err := bus.Request(context.Background(), NewFrame(0x240, []byte{0x3E}), 0x240)
	if err != nil {
		t.Fatal(err)
	}
	if reply.ID != 0x240 || reply.Bytes()[0] != 0x3E {
		t.Fatalf("unexpected reply: %s", reply)
	}
}

func TestRecvTimeout(t *testing.T) {
	bus := openLoopback(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := bus.Recv(ctx, 0x999)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

func TestFramesIteratorBreak(t *testing.T) {
	bus := openLoopback(t)
	ctx := context.Background()

	go func() {
		for i := range 10 {
			bus.Send(ctx, NewFrame(0x100, []byte{byte(i)}))
			time.Sleep(time.Millisecond)
		}
	}()

	var got int
	for range bus.Frames(ctx, 0x100) {
		got++
		if got == 3 {
			break
		}
	}
	if got != 3 {
		t.Fatalf("want 3 frames, got %d", got)
	}
}

func TestSubscriptionEndsOnContextCancel(t *testing.T) {
	bus := openLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())
	ch := bus.Subscribe(ctx, 0x100)
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel, got frame")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription channel not closed after ctx cancel")
	}
}

func TestCloseReleasesSubscribers(t *testing.T) {
	bus := openLoopback(t)
	ch := bus.Subscribe(context.Background(), 0x100)
	bus.Close()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel, got frame")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription channel not closed after bus close")
	}
	if err := bus.Err(); err != nil {
		t.Fatalf("clean close should report nil, got %v", err)
	}
	if err := bus.Send(context.Background(), NewFrame(1, nil)); !errors.Is(err, ErrClosed) {
		t.Fatalf("send after close: want ErrClosed, got %v", err)
	}
}

func TestFatal(t *testing.T) {
	bus := openLoopback(t)
	var events []Event
	bus.OnEvent(func(e Event) { events = append(events, e) })

	boom := errors.New("boom")
	bus.Fatal(boom)

	if err := bus.Wait(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Wait: want boom, got %v", err)
	}
	if err := bus.Err(); !errors.Is(err, boom) {
		t.Fatalf("Err: want boom, got %v", err)
	}
	if len(events) != 1 || !events[0].IsFatal() {
		t.Fatalf("want one fatal event, got %v", events)
	}
	// Subscribing on a dead bus yields an already-closed channel.
	if _, ok := <-bus.Subscribe(context.Background()); ok {
		t.Fatal("subscribe on dead bus should be closed")
	}
}

// TestNewAdapterDeferredOpen covers the construct-early, open-later pattern:
// a settings dialog builds the adapter, the session code opens the bus.
func TestNewAdapterDeferredOpen(t *testing.T) {
	adapter, err := NewAdapter("loopback", Config{})
	if err != nil {
		t.Fatal(err)
	}
	bus, err := OpenAdapter(context.Background(), adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if _, err := bus.Request(context.Background(), NewFrame(0x123, []byte{1}), 0x123); err != nil {
		t.Fatal(err)
	}

	if _, err := NewAdapter("no such adapter", Config{}); err == nil {
		t.Fatal("unknown adapter should error at construction")
	}
}

func TestOnEventUnregister(t *testing.T) {
	bus := openLoopback(t)
	var n int
	cancel := bus.OnEvent(func(Event) { n++ })
	bus.Emit(Event{Type: EventTypeInfo, Details: "one"})
	cancel()
	bus.Emit(Event{Type: EventTypeInfo, Details: "two"})
	if n != 1 {
		t.Fatalf("want 1 event after unregister, got %d", n)
	}
}
