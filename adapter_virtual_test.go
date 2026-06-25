package gocan

import (
	"context"
	"testing"
	"time"
)

// newMockClient wires a Client to the echo Mock adapter.
func newMockClient(t *testing.T) *Client {
	t.Helper()
	a, err := NewMock("mock", &AdapterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cl, err := NewWithOpts(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

// NewMock returns a working Mock (not a Template) whose Send/Recv echo loop runs.
func TestNewMockType(t *testing.T) {
	a, err := NewMock("mock", &AdapterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(*Mock); !ok {
		t.Fatalf("NewMock returned %T, want *Mock", a)
	}
}

// A frame sent through the Mock is echoed back and matched by SendAndWait,
// exercising the full Client -> adapter -> handler -> subscriber path.
func TestMockSendAndWaitRoundTrip(t *testing.T) {
	cl := newMockClient(t)

	resp, err := cl.SendAndWait(context.Background(),
		NewFrame(0x7E0, []byte{0x01, 0x02, 0x03}, ResponseRequired),
		time.Second, 0x7E0)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Identifier != 0x7E0 {
		t.Fatalf("id = 0x%X, want 0x7E0", resp.Identifier)
	}
	if resp.FrameType.Type != ResponseTypeIncoming {
		t.Fatalf("echoed frame type = %v, want Incoming", resp.FrameType.Type)
	}
	if string(resp.Data) != string([]byte{0x01, 0x02, 0x03}) {
		t.Fatalf("data = % X, want 01 02 03", resp.Data)
	}
}

// A subscriber only receives frames matching its identifier filter.
func TestMockSubscribeFilter(t *testing.T) {
	cl := newMockClient(t)

	sub := cl.Subscribe(context.Background(), 0x100)
	defer sub.Close()

	if err := cl.Send(0x200, []byte{0xAA}, Outgoing); err != nil {
		t.Fatal(err)
	}
	if err := cl.Send(0x100, []byte{0xBB}, Outgoing); err != nil {
		t.Fatal(err)
	}

	select {
	case fr := <-sub.Chan():
		if fr.Identifier != 0x100 {
			t.Fatalf("got 0x%X, want only 0x100", fr.Identifier)
		}
	case <-time.After(time.Second):
		t.Fatal("no frame delivered to subscriber")
	}

	// The unmatched 0x200 frame must not arrive.
	select {
	case fr := <-sub.Chan():
		t.Fatalf("unexpected frame 0x%X delivered to filtered sub", fr.Identifier)
	case <-time.After(50 * time.Millisecond):
	}
}

// Recv waits for a single frame with a matching identifier.
func TestMockRecv(t *testing.T) {
	cl := newMockClient(t)

	// Send after Recv is listening: the Mock echoes asynchronously and
	// deliver() drops frames with no matching subscriber, so sending first races.
	go func() {
		time.Sleep(50 * time.Millisecond)
		if err := cl.Send(0x42, []byte{0x99}, Outgoing); err != nil {
			t.Error(err)
		}
	}()
	fr, err := cl.Recv(context.Background(), time.Second, 0x42)
	if err != nil {
		t.Fatal(err)
	}
	if fr.Identifier != 0x42 || len(fr.Data) != 1 || fr.Data[0] != 0x99 {
		t.Fatalf("unexpected frame: %+v", fr)
	}
}

// Recv returns a timeout error when no matching frame arrives.
func TestMockRecvTimeout(t *testing.T) {
	cl := newMockClient(t)

	start := time.Now()
	_, err := cl.Recv(context.Background(), 50*time.Millisecond, 0xDEAD)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Recv took %v, expected ~50ms timeout", elapsed)
	}
}
