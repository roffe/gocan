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

func FuzzParse(f *testing.F) {
	f.Add(New(CMD_SEND_TO_NETWORK_NORMAL, []byte{0, 0, 2, 0x58, 1, 2}).Bytes())
	f.Add([]byte{0x08, 0x0C, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		if cmd, err := Parse(data); err == nil && !bytes.Equal(cmd.Bytes(), data[:3+cmd.Length()]) {
			t.Fatalf("Parse(% X) re-encodes as % X", data, cmd.Bytes())
		}
		p := NewCommandParser(func(*Command) {})
		for rest := data; len(rest) > 0; {
			n := min(len(rest), 1+int(rest[0])%16)
			p.AddData(rest[:n])
			rest = rest[n:]
		}
	})
}
