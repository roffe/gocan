package gocan

import (
	"bytes"
	"testing"

	"github.com/roffe/gocan/pkg/drewtech"
)

// TestDrewtechRecvFrame verifies drewtech RX pushes land on the adapter's
// receive channel as gocan frames with the right id and DLC. Wire-protocol
// tests live in pkg/drewtech.
func TestDrewtechRecvFrame(t *testing.T) {
	adapter, err := NewDrewtech(&AdapterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	a := adapter.(*Drewtech)
	a.recvFrame(&drewtech.CANFrame{ID: 0x7E8, Data: []byte{0x01, 0x50}, DLC: 2})
	select {
	case f := <-a.recvChan:
		if f.Identifier != 0x7E8 || !bytes.Equal(f.Data, []byte{0x01, 0x50}) {
			t.Fatalf("got id=0x%X data=% 02X, want 0x7E8 [01 50]", f.Identifier, f.Data)
		}
		if f.FrameType != Incoming {
			t.Errorf("frame type = %v, want Incoming", f.FrameType)
		}
	default:
		t.Fatal("no frame on recvChan")
	}
}
