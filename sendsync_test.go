package gocan

import (
	"context"
	"testing"
	"time"
)

// gatedAdapter is a sync-capable test adapter whose drain loop calls markSent
// only after the test releases it, so we can observe SendSync actually blocking.
type gatedAdapter struct {
	*BaseAdapter
	release chan struct{}
}

func newGatedAdapter() *gatedAdapter {
	a := &gatedAdapter{
		BaseAdapter: NewBaseAdapter("gated", &AdapterConfig{}),
		release:     make(chan struct{}),
	}
	a.syncCapable = true
	return a
}

func (a *gatedAdapter) Open(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-a.closeChan:
				return
			case f := <-a.sendChan:
				<-a.release
				f.markSent()
			}
		}
	}()
	return nil
}

func (a *gatedAdapter) Close() error { a.BaseAdapter.Close(); return nil }

func TestMarkSent(t *testing.T) {
	(&CANFrame{}).markSent() // nil sent channel: must not panic
	f := &CANFrame{sent: make(chan struct{}, 1)}
	f.markSent()
	f.markSent() // idempotent: must not panic or block
	select {
	case <-f.sent:
	default:
		t.Fatal("markSent did not signal the waiter")
	}
}

func TestSendSyncBlocksUntilMarkSent(t *testing.T) {
	a := newGatedAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	done := make(chan error, 1)
	go func() {
		done <- cl.SendSync(context.Background(), NewFrame(0x240, []byte{1}, Outgoing), 5*time.Second)
	}()

	select {
	case <-done:
		t.Fatal("SendSync returned before the adapter confirmed the write")
	case <-time.After(50 * time.Millisecond):
	}

	close(a.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendSync: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendSync did not return after the adapter released the frame")
	}
}

// A non-sync-capable adapter (Mock) must make SendSync fall back to a plain
// fire-and-forget send instead of blocking until a timeout.
func TestSendSyncFallbackNonCapable(t *testing.T) {
	a, err := NewMock("mock", &AdapterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	done := make(chan error, 1)
	go func() {
		done <- cl.SendSync(context.Background(), NewFrame(0x240, []byte{1}, Outgoing), 5*time.Second)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendSync fallback: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SendSync did not fall back promptly on a non-capable adapter")
	}
}
