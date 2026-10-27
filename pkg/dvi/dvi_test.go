package dvi

import (
	"bytes"
	"testing"
)

func TestParseLengthBounds(t *testing.T) {
	for _, n := range []int{0, 1, 253, 254, 255} {
		in := New(CMD_SEND_TO_NETWORK_NORMAL, bytes.Repeat([]byte{0xFF}, n))
		out, err := Parse(in.Bytes())
		if err != nil {
			t.Fatalf("len %d: %v", n, err)
		}
		if !bytes.Equal(out.Data(), in.Data()) {
			t.Fatalf("len %d: got %X", n, out.Data())
		}
	}
	if _, err := Parse([]byte{0x08, 0x0C, 0x00}); err == nil {
		t.Fatal("short input accepted")
	}
}
