package gmlan

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

// faults loses, repeats or cuts reply frame k.
func faults(k int) map[string]func(int, gocan.Frame) []gocan.Frame {
	return map[string]func(int, gocan.Frame) []gocan.Frame{
		"drop":      gocantest.Drop(k),
		"duplicate": gocantest.Duplicate(k),
		"truncate":  gocantest.Truncate(k, 3),
	}
}

// Every reply frame of a multi-frame read is dropped, duplicated and
// truncated in turn: the read returns the right data or an error, never
// wrong data with a nil error.
func TestReadsUnderFaults(t *testing.T) {
	mem := []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F}
	for _, tc := range []struct {
		name   string
		frames int // reply frames in a clean read
		reply  func(req gocan.Frame) []gocan.Frame
		read   func(cl *Client) ([]byte, error)
		want   []byte
	}{
		{
			name:   "ReadDataByIdentifier",
			frames: 2,
			reply: func(req gocan.Frame) []gocan.Frame {
				switch {
				case isRDBI(req):
					return rx(firstFrame)
				case isFlowCtl(req):
					return rx(consecFrame)
				}
				return nil
			},
			read: func(cl *Client) ([]byte, error) { return cl.ReadDataByIdentifier(context.Background(), 0x18) },
			want: wantPayload,
		},
		{
			name:   "ReadMemoryByAddress",
			frames: 3,
			reply: func(req gocan.Frame) []gocan.Frame {
				switch b := req.Bytes(); {
				case len(b) > 1 && b[1] == READ_MEMORY_BY_ADDRESS:
					return rx(append([]byte{0x10, 4 + byte(len(mem)), 0x63, 0x00, 0x10, 0x00}, mem[:2]...))
				case isFlowCtl(req):
					return rx(append([]byte{0x21}, mem[2:9]...), append([]byte{0x22}, mem[9:]...))
				}
				return nil
			},
			read: func(cl *Client) ([]byte, error) {
				return cl.ReadMemoryByAddress(context.Background(), 0x001000, uint32(len(mem)))
			},
			want: mem,
		},
	} {
		run := func(name string, fault func(int, gocan.Frame) []gocan.Frame, clean bool) {
			t.Run(name, func(t *testing.T) {
				got, err := tc.read(newTestClient(t, &gocantest.Node{Reply: tc.reply, Fault: fault}))
				switch {
				case clean && (err != nil || !bytes.Equal(got, tc.want)):
					t.Fatalf("clean read: % 02X, %v; want % 02X", got, err, tc.want)
				case err == nil && !bytes.Equal(got, tc.want):
					t.Fatalf("got % 02X with no error, want % 02X", got, tc.want)
				}
			})
		}
		run(tc.name+"/clean", nil, true)
		for k := range tc.frames {
			for fname, fault := range faults(k) {
				run(fmt.Sprintf("%s/%s %d", tc.name, fname, k), fault, false)
			}
		}
	}
}
