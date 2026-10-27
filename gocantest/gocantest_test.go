package gocantest

import (
	"bytes"
	"errors"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

func TestNode(t *testing.T) {
	node := &Node{Reply: func(req gocan.Frame) []gocan.Frame {
		return []gocan.Frame{gocan.NewFrame(0x7E8, req.Bytes()), gocan.NewFrame(0x7E8, []byte{0xFF})}
	}}
	bus := Open(t, node)
	rx := bus.SubscribeN(t.Context(), 16, 0x7E8)
	exchange := func(want ...string) {
		t.Helper()
		if err := bus.Send(t.Context(), gocan.NewFrame(0x7E0, []byte{1, 2, 3})); err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			select {
			case f := <-rx:
				if got := string(f.Bytes()); got != w {
					t.Fatalf("got % X, want % X", got, w)
				}
			case <-time.After(time.Second):
				t.Fatalf("no reply, want % X", w)
			}
		}
		if len(rx) != 0 {
			t.Fatalf("%d extra replies", len(rx))
		}
	}

	exchange("\x01\x02\x03", "\xFF") // replies 0 and 1
	node.Fault = Drop(2)
	exchange("\xFF")
	node.Fault = Duplicate(4)
	exchange("\x01\x02\x03", "\x01\x02\x03", "\xFF")
	node.Fault = Truncate(6, 1)
	exchange("\x01", "\xFF")
	node.Fault, node.Delay = nil, 20*time.Millisecond
	exchange("\x01\x02\x03", "\xFF")

	if sent := node.Sent(); len(sent) != 5 || !bytes.Equal(sent[4].Bytes(), []byte{1, 2, 3}) {
		t.Fatalf("Sent() = %v", sent)
	}
	node.Unplug(errors.New("unplugged"))
	if bus.Err() == nil {
		t.Fatal("bus still up after Unplug")
	}
}
