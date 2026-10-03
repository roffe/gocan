package socketcan

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
)

// Needs a vcan0 interface. Without root:
//
//	unshare -rn sh -c 'ip link add vcan0 type vcan && ip link set vcan0 up && go test ./adapters/socketcan/'
func openPair(t testing.TB) (tx, rx *gocan.Bus) {
	if _, err := net.InterfaceByName("vcan0"); err != nil {
		t.Skip("vcan0 not available")
	}
	open := func() *gocan.Bus {
		a, _ := New(gocan.Config{Port: "vcan0"})
		b, err := gocan.OpenAdapter(t.Context(), a)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { b.Close() })
		return b
	}
	return open(), open()
}

func TestLoopback(t *testing.T) {
	tx, rx := openPair(t)
	ext := gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3})
	rtr := gocan.Frame{ID: 0x7E0, Remote: true, Length: 2}
	frames := []gocan.Frame{gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0, 0x11, 0x0C, 0, 0, 0}), ext, rtr}
	ch := rx.Subscribe(t.Context(), 0x220, ext.ID, rtr.ID)
	for _, want := range frames {
		if err := tx.Send(t.Context(), want); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-ch:
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for %+v", want)
		}
	}
}

// A deadline from one Send must not outlive it and fail a later Send.
func TestStaleWriteDeadline(t *testing.T) {
	tx, _ := openPair(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := tx.Send(ctx, gocan.NewFrame(0x123, nil)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := tx.Send(t.Context(), gocan.NewFrame(0x123, nil)); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkSend(b *testing.B) {
	tx, _ := openPair(b)
	f := gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0, 0x11, 0x0C, 0, 0, 0})
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if err := tx.Send(ctx, f); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	tx, rx := openPair(b)
	f := gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0, 0x11, 0x0C, 0, 0, 0})
	ctx := b.Context()
	ch := rx.Subscribe(ctx, f.ID)
	b.ReportAllocs()
	for b.Loop() {
		if err := tx.Send(ctx, f); err != nil {
			b.Fatal(err)
		}
		<-ch
	}
}
