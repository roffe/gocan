package serialcommand

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 253, 254, 255} {
		in := &SerialCommand{Command: 'R', Data: bytes.Repeat([]byte{0xFF}, n)}
		b, err := in.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		var out SerialCommand
		if err := out.UnmarshalBinary(b); err != nil {
			t.Fatalf("len %d: %v", n, err)
		}
		if out.Command != in.Command || !bytes.Equal(out.Data, in.Data) {
			t.Fatalf("len %d: got %v", n, &out)
		}
	}
	if err := new(SerialCommand).UnmarshalBinary([]byte{'t', 5}); err == nil {
		t.Fatal("short input accepted")
	}
}
