package t7kwp

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

func newClient(t *testing.T, node *gocantest.Node) *Client {
	t.Helper()
	c := New(gocantest.Open(t, node))
	c.SetResponseID(0x258)
	return c
}

// acks lists the rows the client acked on RESP_CHUNK_CONF_ID.
func acks(node *gocantest.Node) []byte {
	var out []byte
	for _, f := range node.Sent() {
		if f.ID == RESP_CHUNK_CONF_ID {
			out = append(out, f.Data[3])
		}
	}
	return out
}

// f0Node answers the F0 read that starts recvData with reply, and the first
// ack with next if it is set.
func f0Node(reply, next gocan.Frame) *gocantest.Node {
	first := true
	return &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		switch {
		case f.ID == REQ_MSG_ID && f.Data[3] == READ_DATA_BY_IDENTIFIER && f.Data[4] == 0xF0:
			return []gocan.Frame{reply}
		case f.ID == RESP_CHUNK_CONF_ID && first && next.ID != 0:
			first = false
			return []gocan.Frame{next}
		}
		return nil
	}}
}

func recvDataFrom(t *testing.T, reply gocan.Frame, length int) ([]byte, *gocantest.Node, error) {
	t.Helper()
	node := f0Node(reply, gocan.Frame{})
	b, err := newClient(t, node).recvData(t.Context(), length)
	return b, node, err
}

// A positive F0 reply hands back the data bytes after the SID and identifier
// and acks the last row.
func TestRecvData(t *testing.T) {
	b, node, err := recvDataFrom(t, frame(5, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0xAA, 0xBB, 0xCC), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{0xAA, 0xBB, 0xCC}) {
		t.Fatalf("data % 02X, want AA BB CC", b)
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", got)
	}
}

// A negative reply to the F0 read is an error, not one byte of data. The ECU
// still gets its ack.
func TestRecvDataNegativeResponse(t *testing.T) {
	b, node, err := recvDataFrom(t, frame(3, 0x7F, READ_DATA_BY_IDENTIFIER, SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT), 0x200)
	if !errors.Is(err, ErrSubFunctionNotSupportedOrInvalidFormat) {
		t.Fatalf("err = %v, want %v", err, ErrSubFunctionNotSupportedOrInvalidFormat)
	}
	if b != nil {
		t.Fatalf("data % 02X, want none", b)
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", got)
	}
}

// checkErr reads a negative response only from a first frame. In a later frame
// byte 3 is payload, so 0x7F there is data.
func TestCheckErrFirstFrameOnly(t *testing.T) {
	neg := frame(3, 0x7F, READ_DATA_BY_IDENTIFIER, REQUEST_OUT_OF_RANGE)
	if err := checkErr(neg); !errors.Is(err, ErrRequestOutOfRange) {
		t.Fatalf("first frame: err = %v, want %v", err, ErrRequestOutOfRange)
	}
	cont := gocan.NewFrame(0x258, []byte{0x81, 0xA1, 0x12, 0x7F, 0x21, 0x31, 0x00, 0x00})
	if err := checkErr(cont); err != nil {
		t.Fatalf("continuation frame with 0x7F in byte 3: err = %v, want nil", err)
	}
}

// A multi-frame F0 reply whose payload has 0x7F where a first frame keeps its
// SID comes back whole.
func TestRecvDataPayload7F(t *testing.T) {
	node := f0Node(
		gocan.NewFrame(0x258, []byte{0xC1, 0xA1, 0x08, 0x61, 0xF0, 0x01, 0x02, 0x03}),
		gocan.NewFrame(0x258, []byte{0x80, 0xA1, 0x04, 0x7F, 0x05, 0x00, 0x00, 0x00}),
	)
	b, err := newClient(t, node).recvData(t.Context(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x01, 0x02, 0x03, 0x04, 0x7F, 0x05}; !bytes.Equal(b, want) {
		t.Fatalf("data % 02X, want % 02X", b, want)
	}
}

// readFlashFrom reads 3 bytes from an ECU that answers ReadMemoryByAddressF0:
// 6C F0 to the 2C F0 03 jump, then the next of replies to each 21 F0. It
// returns how many F0 reads there were.
func readFlashFrom(t *testing.T, replies ...gocan.Frame) ([]byte, int, error) {
	t.Helper()
	reads := 0
	node := &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		switch {
		case f.ID != REQ_MSG_ID:
		case f.Data[0] == 0x00: // second frame of 2C F0 03 <len> <addr>
			return []gocan.Frame{frame(2, DYNAMICALLY_DEFINE_IDENTIFIER|0x40, 0xF0)}
		case f.Data[3] == READ_DATA_BY_IDENTIFIER && f.Data[4] == 0xF0:
			reads++
			return []gocan.Frame{replies[min(reads, len(replies))-1]}
		}
		return nil
	}}
	b, err := newClient(t, node).ReadFlash(t.Context(), 0x1000, 3)
	return b, reads, err
}

// A reply with fewer bytes than asked for is retried, not written short.
func TestReadFlashRetriesShortRead(t *testing.T) {
	short := frame(4, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0xAA, 0xBB)
	full := frame(5, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0x01, 0x02, 0x03)
	b, reads, err := readFlashFrom(t, short, full)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{0x01, 0x02, 0x03}) || reads != 2 {
		t.Fatalf("got % 02X after %d reads, want 01 02 03 after 2", b, reads)
	}
}

// Short replies on every attempt fail the read instead of returning a dump
// with a hole in it.
func TestReadFlashShortReadFails(t *testing.T) {
	b, reads, err := readFlashFrom(t, frame(4, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0xAA, 0xBB))
	if err == nil {
		t.Fatalf("ReadFlash = % 02X, want an error", b)
	}
	if reads != 3 {
		t.Fatalf("%d reads, want 3 attempts", reads)
	}
}

// Late replies to other reads are skipped, not returned as this read's data.
func TestReadMemoryByAddressSkipsOtherReplies(t *testing.T) {
	c := newClient(t, &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		if f.ID == REQ_MSG_ID && f.Data[3] == READ_MEMORY_BY_ADDRESS {
			return []gocan.Frame{
				gocan.NewFrame(0x258, []byte{0xC0, 0xA1, 0x05, 0x61, 0xF0, 0x11, 0x22, 0x33}), // a logger row
				gocan.NewFrame(0x258, []byte{0xC0, 0xA1, 0x05, 0x63, 0xF0, 0x20, 0x00, 0xBB}), // another address
				gocan.NewFrame(0x258, []byte{0xC0, 0xA1, 0x05, 0x63, 0xF0, 0x10, 0x00, 0xAA}),
			}
		}
		return nil
	}})
	got, err := c.ReadMemoryByAddress(t.Context(), 0xF01000, 1)
	if err != nil || !bytes.Equal(got, []byte{0xAA}) {
		t.Fatalf("got % 02X, %v, want AA", got, err)
	}
}

// A row counter that does not count down ends the read instead of acking forever.
func TestRecvChunkedRowCountdown(t *testing.T) {
	node := &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		switch {
		case f.ID == REQ_MSG_ID && f.Data[3] == READ_DATA_BY_IDENTIFIER:
			return []gocan.Frame{gocan.NewFrame(0x258, []byte{0xC1, 0xA1, 0x08, 0x61, 0x10, 1, 2, 3})}
		case f.ID == RESP_CHUNK_CONF_ID:
			return []gocan.Frame{gocan.NewFrame(0x258, []byte{0x81, 0xA1, 4, 5, 6, 7, 8, 9})}
		}
		return nil
	}}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := newClient(t, node).ReadDataByIdentifier(ctx, 0x10); err == nil || len(acks(node)) != 1 {
		t.Fatalf("err = %v after %d acks, want an error after 1", err, len(acks(node)))
	}
}

// The ECU's verdict on a definition is read: a rejected entry is an error.
func TestSendDDLVerdict(t *testing.T) {
	for _, tt := range []struct {
		reply gocan.Frame
		ok    bool
	}{
		{frame(2, DYNAMICALLY_DEFINE_IDENTIFIER|0x40, 0xF0), true},
		{frame(3, 0x7F, DYNAMICALLY_DEFINE_IDENTIFIER, REQUEST_OUT_OF_RANGE), false},
	} {
		c := newClient(t, &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
			switch {
			case f.ID != REQ_MSG_ID:
			case f.Data[0]&0x80 != 0:
				return []gocan.Frame{gocan.NewFrame(REQ_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, 0x81, 0, 0, 0, 0})}
			default:
				return []gocan.Frame{tt.reply}
			}
			return nil
		}})
		if err := c.DynamicallyDefineLocalIdBySymbolNumber(t.Context(), 0, 0x1234); (err == nil) != tt.ok {
			t.Errorf("reply %s: err %v", tt.reply, err)
		}
	}
}
