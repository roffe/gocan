package t7kwp

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
)

// f0ECU answers the F0 read that starts recvData with reply, sends next (if
// set) on the first ack, and records the rows the client acks on
// RESP_CHUNK_CONF_ID.
type f0ECU struct {
	bus   *gocan.Bus
	reply gocan.Frame
	next  gocan.Frame
	acks  []byte
}

func (e *f0ECU) Open(_ context.Context, b *gocan.Bus) error { e.bus = b; return nil }
func (e *f0ECU) Close() error                               { return nil }

func (e *f0ECU) Send(_ context.Context, f gocan.Frame) error {
	switch {
	case f.ID == REQ_MSG_ID && f.Data[3] == READ_DATA_BY_IDENTIFIER && f.Data[4] == 0xF0:
		e.bus.Deliver(e.reply)
	case f.ID == RESP_CHUNK_CONF_ID:
		e.acks = append(e.acks, f.Data[3])
		if len(e.acks) == 1 && e.next.ID != 0 {
			e.bus.Deliver(e.next)
		}
	}
	return nil
}

func recvDataFrom(t *testing.T, reply gocan.Frame, length int) ([]byte, *f0ECU, error) {
	t.Helper()
	ecu := &f0ECU{reply: reply}
	bus, err := gocan.OpenAdapter(t.Context(), ecu)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c := New(bus)
	c.SetResponseID(0x258)
	b, err := c.recvData(t.Context(), length)
	return b, ecu, err
}

// A positive F0 reply hands back the data bytes after the SID and identifier
// and acks the last row.
func TestRecvData(t *testing.T) {
	b, ecu, err := recvDataFrom(t, frame(5, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0xAA, 0xBB, 0xCC), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{0xAA, 0xBB, 0xCC}) {
		t.Fatalf("data % 02X, want AA BB CC", b)
	}
	if !bytes.Equal(ecu.acks, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", ecu.acks)
	}
}

// A negative reply to the F0 read is an error, not one byte of data. The ECU
// still gets its ack.
func TestRecvDataNegativeResponse(t *testing.T) {
	b, ecu, err := recvDataFrom(t, frame(3, 0x7F, READ_DATA_BY_IDENTIFIER, SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT), 0x200)
	if !errors.Is(err, ErrSubFunctionNotSupportedOrInvalidFormat) {
		t.Fatalf("err = %v, want %v", err, ErrSubFunctionNotSupportedOrInvalidFormat)
	}
	if b != nil {
		t.Fatalf("data % 02X, want none", b)
	}
	if !bytes.Equal(ecu.acks, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", ecu.acks)
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
	ecu := &f0ECU{}
	bus, err := gocan.OpenAdapter(t.Context(), ecu)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	ecu.reply = gocan.NewFrame(0x258, []byte{0xC1, 0xA1, 0x08, 0x61, 0xF0, 0x01, 0x02, 0x03})
	ecu.next = gocan.NewFrame(0x258, []byte{0x80, 0xA1, 0x04, 0x7F, 0x05, 0x00, 0x00, 0x00})
	c := New(bus)
	c.SetResponseID(0x258)
	b, err := c.recvData(t.Context(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x01, 0x02, 0x03, 0x04, 0x7F, 0x05}; !bytes.Equal(b, want) {
		t.Fatalf("data % 02X, want % 02X", b, want)
	}
}

// flashReadECU answers ReadMemoryByAddressF0: 6C F0 to the 2C F0 03 jump, then
// the next of replies to each 21 F0.
type flashReadECU struct {
	bus     *gocan.Bus
	replies []gocan.Frame
	reads   int
}

func (e *flashReadECU) Open(_ context.Context, b *gocan.Bus) error { e.bus = b; return nil }
func (e *flashReadECU) Close() error                               { return nil }

func (e *flashReadECU) Send(_ context.Context, f gocan.Frame) error {
	switch {
	case f.ID != REQ_MSG_ID:
	case f.Data[0] == 0x00: // second frame of 2C F0 03 <len> <addr>
		e.bus.Deliver(frame(2, DYNAMICALLY_DEFINE_IDENTIFIER|0x40, 0xF0))
	case f.Data[3] == READ_DATA_BY_IDENTIFIER && f.Data[4] == 0xF0:
		e.bus.Deliver(e.replies[min(e.reads, len(e.replies)-1)])
		e.reads++
	}
	return nil
}

func readFlashFrom(t *testing.T, replies ...gocan.Frame) ([]byte, *flashReadECU, error) {
	t.Helper()
	ecu := &flashReadECU{replies: replies}
	bus, err := gocan.OpenAdapter(t.Context(), ecu)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c := New(bus)
	c.SetResponseID(0x258)
	b, err := c.ReadFlash(t.Context(), 0x1000, 3)
	return b, ecu, err
}

// A reply with fewer bytes than asked for is retried, not written short.
func TestReadFlashRetriesShortRead(t *testing.T) {
	short := frame(4, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0xAA, 0xBB)
	full := frame(5, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0x01, 0x02, 0x03)
	b, ecu, err := readFlashFrom(t, short, full)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{0x01, 0x02, 0x03}) || ecu.reads != 2 {
		t.Fatalf("got % 02X after %d reads, want 01 02 03 after 2", b, ecu.reads)
	}
}

// Short replies on every attempt fail the read instead of returning a dump
// with a hole in it.
func TestReadFlashShortReadFails(t *testing.T) {
	b, ecu, err := readFlashFrom(t, frame(4, READ_DATA_BY_IDENTIFIER|0x40, 0xF0, 0xAA, 0xBB))
	if err == nil {
		t.Fatalf("ReadFlash = % 02X, want an error", b)
	}
	if ecu.reads != 3 {
		t.Fatalf("%d reads, want 3 attempts", ecu.reads)
	}
}

// scriptECU answers each frame the client sends with onSend.
type scriptECU struct {
	bus    *gocan.Bus
	onSend func(e *scriptECU, f gocan.Frame)
}

func (e *scriptECU) Open(_ context.Context, b *gocan.Bus) error { e.bus = b; return nil }
func (e *scriptECU) Close() error                               { return nil }
func (e *scriptECU) Send(_ context.Context, f gocan.Frame) error {
	e.onSend(e, f)
	return nil
}

func newScriptClient(t *testing.T, onSend func(e *scriptECU, f gocan.Frame)) *Client {
	t.Helper()
	bus, err := gocan.OpenAdapter(t.Context(), &scriptECU{onSend: onSend})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	c := New(bus)
	c.SetResponseID(0x258)
	return c
}

// A row received twice (a CAN double receive) is data once.
func TestRecvDataRepeatedRow(t *testing.T) {
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	c := newScriptClient(t, func(e *scriptECU, f gocan.Frame) {
		switch {
		case f.ID == REQ_MSG_ID && f.Data[3] == READ_DATA_BY_IDENTIFIER:
			e.bus.Deliver(gocan.NewFrame(0x258, append([]byte{0xC2, 0xA1, 17, 0x61, 0xF0}, data[0:3]...)))
		case f.ID == RESP_CHUNK_CONF_ID && f.Data[3] == 0x82:
			row := gocan.NewFrame(0x258, append([]byte{0x81, 0xA1}, data[3:9]...))
			e.bus.Deliver(row)
			e.bus.Deliver(row)
		case f.ID == RESP_CHUNK_CONF_ID && f.Data[3] == 0x81:
			e.bus.Deliver(gocan.NewFrame(0x258, append([]byte{0x80, 0xA1}, data[9:15]...)))
		}
	})
	got, err := c.recvData(t.Context(), len(data))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("got % 02X, %v, want % 02X", got, err, data)
	}
}

// Late replies to other reads are skipped, not returned as this read's data.
func TestReadMemoryByAddressSkipsOtherReplies(t *testing.T) {
	c := newScriptClient(t, func(e *scriptECU, f gocan.Frame) {
		if f.ID == REQ_MSG_ID && f.Data[3] == READ_MEMORY_BY_ADDRESS {
			e.bus.Deliver(gocan.NewFrame(0x258, []byte{0xC0, 0xA1, 0x05, 0x61, 0xF0, 0x11, 0x22, 0x33})) // a logger row
			e.bus.Deliver(gocan.NewFrame(0x258, []byte{0xC0, 0xA1, 0x05, 0x63, 0xF0, 0x20, 0x00, 0xBB})) // another address
			e.bus.Deliver(gocan.NewFrame(0x258, []byte{0xC0, 0xA1, 0x05, 0x63, 0xF0, 0x10, 0x00, 0xAA}))
		}
	})
	got, err := c.ReadMemoryByAddress(t.Context(), 0xF01000, 1)
	if err != nil || !bytes.Equal(got, []byte{0xAA}) {
		t.Fatalf("got % 02X, %v, want AA", got, err)
	}
}

// A row counter that does not count down ends the read instead of acking forever.
func TestRecvChunkedRowCountdown(t *testing.T) {
	acks := 0
	c := newScriptClient(t, func(e *scriptECU, f gocan.Frame) {
		switch {
		case f.ID == REQ_MSG_ID && f.Data[3] == READ_DATA_BY_IDENTIFIER:
			e.bus.Deliver(gocan.NewFrame(0x258, []byte{0xC1, 0xA1, 0x08, 0x61, 0x10, 1, 2, 3}))
		case f.ID == RESP_CHUNK_CONF_ID:
			acks++
			e.bus.Deliver(gocan.NewFrame(0x258, []byte{0x81, 0xA1, 4, 5, 6, 7, 8, 9}))
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := c.ReadDataByIdentifier(ctx, 0x10); err == nil || acks != 1 {
		t.Fatalf("err = %v after %d acks, want an error after 1", err, acks)
	}
}
