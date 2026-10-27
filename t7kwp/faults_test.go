package t7kwp

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

// Every reply frame of a multi-row read is dropped, duplicated and truncated
// in turn: the read returns the right data or an error, never wrong data with
// a nil error.
func TestReadsUnderFaults(t *testing.T) {
	saved := DefaultTimeout
	DefaultTimeout = 30 * time.Millisecond
	t.Cleanup(func() { DefaultTimeout = saved })

	data := []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E}
	row := func(b ...byte) []gocan.Frame { return []gocan.Frame{gocan.NewFrame(0x258, b)} }
	// rows answers the acks of a reply whose first frame counts two more rows.
	rows := func(f gocan.Frame, r1, r2 []byte) []gocan.Frame {
		switch {
		case f.ID == RESP_CHUNK_CONF_ID && f.Data[3] == 0x82:
			return row(append([]byte{0x81, 0xA1}, r1...)...)
		case f.ID == RESP_CHUNK_CONF_ID && f.Data[3] == 0x81:
			return row(append([]byte{0x80, 0xA1}, r2...)...)
		}
		return nil
	}
	for _, tc := range []struct {
		name  string
		reply func(f gocan.Frame) []gocan.Frame
		read  func(c *Client) ([]byte, error)
		want  []byte
	}{
		{
			name: "ReadMemoryByAddress",
			reply: func(f gocan.Frame) []gocan.Frame {
				if f.ID == REQ_MSG_ID && f.Data[3] == READ_MEMORY_BY_ADDRESS {
					return row(0xC2, 0xA1, 4+13, 0x63, 0xF0, 0x10, 0x00, data[0])
				}
				return rows(f, data[1:7], data[7:13])
			},
			read: func(c *Client) ([]byte, error) { return c.ReadMemoryByAddress(t.Context(), 0xF01000, 13) },
			want: data[:13],
		},
		{
			name: "recvData",
			reply: func(f gocan.Frame) []gocan.Frame {
				if f.ID == REQ_MSG_ID && f.Data[3] == READ_DATA_BY_IDENTIFIER {
					return row(append([]byte{0xC2, 0xA1, 2 + 15, 0x61, 0xF0}, data[:3]...)...)
				}
				return rows(f, data[3:9], data[9:15])
			},
			read: func(c *Client) ([]byte, error) { return c.recvData(t.Context(), 15) },
			want: data,
		},
	} {
		run := func(name string, fault func(int, gocan.Frame) []gocan.Frame, clean bool) {
			t.Run(name, func(t *testing.T) {
				got, err := tc.read(newClient(t, &gocantest.Node{Reply: tc.reply, Fault: fault}))
				switch {
				case clean && (err != nil || !bytes.Equal(got, tc.want)):
					t.Fatalf("clean read: % 02X, %v; want % 02X", got, err, tc.want)
				case err == nil && !bytes.Equal(got, tc.want):
					t.Fatalf("got % 02X with no error, want % 02X", got, tc.want)
				}
			})
		}
		run(tc.name+"/clean", nil, true)
		for k := range 3 {
			for fname, fault := range map[string]func(int, gocan.Frame) []gocan.Frame{
				"drop":      gocantest.Drop(k),
				"duplicate": gocantest.Duplicate(k),
				"truncate":  gocantest.Truncate(k, 3),
			} {
				run(fmt.Sprintf("%s/%s %d", tc.name, fname, k), fault, false)
			}
		}
	}
}
