package gocan

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// testAdapter is a minimal Adapter built on BaseAdapter that does nothing on its
// own, letting tests drive events and fatals via the BaseAdapter helpers.
type testAdapter struct {
	*BaseAdapter
}

func newTestAdapter() *testAdapter {
	return &testAdapter{BaseAdapter: NewBaseAdapter("test", &AdapterConfig{})}
}

func (a *testAdapter) Open(context.Context) error { return nil }

func (a *testAdapter) Close() error {
	a.BaseAdapter.Close()
	return nil
}

func waitDone(t *testing.T, cl *Client) {
	t.Helper()
	select {
	case <-cl.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("client did not terminate")
	}
}

// A fatal adapter error cancels the client context with that error as the cause,
// surfaces it as a final EventTypeFatal event, and is reported by Err.
func TestFatalCancelsContextWithCause(t *testing.T) {
	a := newTestAdapter()

	var mu sync.Mutex
	var fatalSeen *Event
	cl, err := NewWithOpts(context.Background(), a, WithEventFunc(func(e Event) {
		if e.IsFatal() {
			mu.Lock()
			ev := e
			fatalSeen = &ev
			mu.Unlock()
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	boom := errors.New("boom")
	a.Fatal(boom)

	waitDone(t, cl)

	if got := cl.Err(); !errors.Is(got, boom) {
		t.Fatalf("Err() = %v, want %v", got, boom)
	}
	if cause := context.Cause(cl.Context()); !errors.Is(cause, boom) {
		t.Fatalf("context.Cause = %v, want %v", cause, boom)
	}
	mu.Lock()
	defer mu.Unlock()
	if fatalSeen == nil {
		t.Fatal("fatal event was not delivered to the event sink")
	}
	if !errors.Is(fatalSeen.Err, boom) {
		t.Fatalf("fatal event Err = %v, want %v", fatalSeen.Err, boom)
	}
}

// Close is a clean shutdown: the cause is ErrClosed and Err reports nil.
func TestCleanCloseIsNotFatal(t *testing.T) {
	a := newTestAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}

	if err := cl.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	waitDone(t, cl)

	if err := cl.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil after clean close", err)
	}
	if cause := context.Cause(cl.Context()); !errors.Is(cause, ErrClosed) {
		t.Fatalf("context.Cause = %v, want ErrClosed", cause)
	}
}

// Events are delivered to a sink in registration/emit order from a single
// goroutine (no per-event goroutine reordering).
func TestEventsDeliveredInOrder(t *testing.T) {
	a := newTestAdapter()

	got := make(chan string, 8)
	cl, err := NewWithOpts(context.Background(), a, WithEventFunc(func(e Event) {
		got <- e.Details
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	a.Info("one")
	a.Info("two")
	a.Warn("three")

	want := []string{"one", "two", "three"}
	for _, w := range want {
		select {
		case g := <-got:
			if g != w {
				t.Fatalf("event = %q, want %q", g, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for event %q", w)
		}
	}
}

// An in-flight SendAndWait aborts with the fatal error instead of timing out
// when the adapter dies mid-request.
func TestSendAndWaitAbortsOnFatal(t *testing.T) {
	a := newTestAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	boom := errors.New("link lost")
	go func() {
		time.Sleep(25 * time.Millisecond)
		a.Fatal(boom)
	}()

	// No response is ever produced; without the fatal this would block until the
	// 1s timeout. We expect it to return the fatal error well before that.
	start := time.Now()
	_, err = cl.SendAndWait(context.Background(), NewFrame(0x100, []byte{1}, ResponseRequired), time.Second, 0x200)
	if !errors.Is(err, boom) {
		t.Fatalf("SendAndWait err = %v, want %v", err, boom)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("SendAndWait took %v, expected fast abort", elapsed)
	}
}

// OnEvent supports multiple listeners and unregistration.
func TestOnEventMultipleAndUnregister(t *testing.T) {
	a := newTestAdapter()
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	var mu sync.Mutex
	var aCount, bCount int
	cancelA := cl.OnEvent(func(Event) { mu.Lock(); aCount++; mu.Unlock() })
	cl.OnEvent(func(Event) { mu.Lock(); bCount++; mu.Unlock() })

	a.Info("first")
	waitForCount(t, &mu, &bCount, 1)

	cancelA()
	a.Info("second")
	waitForCount(t, &mu, &bCount, 2)

	mu.Lock()
	defer mu.Unlock()
	if aCount != 1 {
		t.Fatalf("aCount = %d, want 1 (unregistered listener kept receiving)", aCount)
	}
}

func waitForCount(t *testing.T, mu *sync.Mutex, n *int, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		got := *n
		mu.Unlock()
		if got >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for count %d (got %d)", want, got)
		case <-time.After(2 * time.Millisecond):
		}
	}
}
