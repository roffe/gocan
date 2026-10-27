package drewtech

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"go.bug.st/serial"
)

// TestParseCANFrame checks the documented RX offsets: dataLen(LE)@[18:20],
// id(BE)@[20:24], data@[24:]. dlc = dataLen - 4 (dataLen counts the 4-byte id).
func TestParseCANFrame(t *testing.T) {
	payload := make([]byte, 26)
	payload[0] = SubCmdCANRx
	binary.LittleEndian.PutUint32(payload[12:16], 0x12345678) // timestamp
	binary.LittleEndian.PutUint16(payload[18:20], 6)          // dataLen = id(4)+2
	binary.BigEndian.PutUint32(payload[20:24], 0x7E8)         // id
	payload[24], payload[25] = 0x01, 0x50                     // data

	f, err := (&Packet{Payload: payload}).ParseCANFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != 0x7E8 {
		t.Errorf("id = %X, want 7E8", f.ID)
	}
	if f.DLC != 2 || !bytes.Equal(f.Data, []byte{0x01, 0x50}) {
		t.Errorf("dlc=%d data=%X, want 2 [01 50]", f.DLC, f.Data)
	}
	if f.Timestamp != 0x12345678 {
		t.Errorf("ts = %X", f.Timestamp)
	}
}

func TestIsCANRx(t *testing.T) {
	for _, c := range []struct {
		cmd  []byte
		want bool
	}{
		{[]byte{SubCmdCANRx, 0}, true},                  // RX push, 0x0009
		{[]byte{SubCmdCANRx, FlagResponse}, false},      // board-info reply, 0x8109
		{[]byte{SubCmdCANTx, FlagResponseFinal}, false}, // transmit reply, 0x8008
	} {
		p := &Packet{Payload: c.cmd}
		if got := p.IsCANRx(); got != c.want {
			t.Errorf("command % X: IsCANRx=%v want %v", c.cmd, got, c.want)
		}
	}
}

// TestFrameRoundTrip verifies ToBytes and Parser agree on framing, and that the
// check word is length^0x51E6 (not a constant 0x51 magic) even for len>=256.
func TestFrameRoundTrip(t *testing.T) {
	payload := []byte{SubCmdConnect, FlagRequestFinal, 0x34, 0x12, 0, 0, 0, 0}
	wire := cmd(0, payload).ToBytes()

	pkts, err := NewParser().Feed(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 1 {
		t.Fatalf("got %d packets, want 1", len(pkts))
	}
	if pkts[0].SubCommand() != SubCmdConnect || pkts[0].Sequence() != 0x1234 {
		t.Errorf("op=%02X seq=%04X", pkts[0].SubCommand(), pkts[0].Sequence())
	}

	// Large frame: high check byte must not be 0x51, and the parser must
	// still accept it (full check-word validation, not a constant magic).
	big := &Packet{Length: 0x0120, Payload: make([]byte, 0x011C)}
	b := big.ToBytes()
	if got := binary.LittleEndian.Uint16(b[2:4]); got != 0x0120^ChecksumWord {
		t.Errorf("check word = %04X, want %04X", got, 0x0120^ChecksumWord)
	}
	if b[3] == MagicByte {
		t.Error("high check byte is 0x51 for a >=256B frame (latent flashing bug)")
	}
	pkts, err = NewParser().Feed(b)
	if err != nil || len(pkts) != 1 || pkts[0].Length != 0x0120 {
		t.Fatalf(">=256B frame rejected: err=%v pkts=%d", err, len(pkts))
	}
}

// TestParseCANFrameCapture runs the real 0x09 RX push captured from hardware
// (txlogger issue #32) end to end through the parser: id 0x7E8, data {01,50},
// and pins the capture's check bytes (len 0x1E -> F8 51).
func TestParseCANFrameCapture(t *testing.T) {
	wire := []byte{
		0x1e, 0x00, 0xf8, 0x51, 0x00, 0x00, 0x01, 0x05,
		0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x48, 0x0b,
		0x00, 0x00, 0x00, 0x00, 0xfc, 0x37, 0x0d, 0x00,
		0x06, 0x00, 0x06, 0x00, 0x00, 0x00, 0x07, 0xe8,
		0x01, 0x50,
	}
	pkts, err := NewParser().Feed(wire)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(pkts) != 1 {
		t.Fatalf("got %d packets, want 1", len(pkts))
	}
	if !pkts[0].IsCANRx() {
		t.Fatalf("not recognized as CAN RX (op=0x%02X len=0x%X)", pkts[0].SubCommand(), pkts[0].Length)
	}
	f, err := pkts[0].ParseCANFrame()
	if err != nil {
		t.Fatalf("ParseCANFrame: %v", err)
	}
	if f.ID != 0x7E8 || f.DLC != 2 || !bytes.Equal(f.Data, []byte{0x01, 0x50}) {
		t.Errorf("id=0x%X dlc=%d data=% 02X, want 0x7E8 2 [01 50]", f.ID, f.DLC, f.Data)
	}
	if f.Timestamp != 0x000D37FC {
		t.Errorf("ts = 0x%X, want 0xD37FC", f.Timestamp)
	}
}

// TestParserResync feeds garbage — including a valid-check header with
// length<8, which used to wedge the parser forever — and verifies the
// following valid frame is still recovered and the parser stays healthy.
func TestParserResync(t *testing.T) {
	valid := cmd(0, []byte{SubCmdConnect, FlagRequestFinal, 0x34, 0x12, 0, 0, 0, 0}).ToBytes()
	tiny := make([]byte, 4) // length=4 < 8 with a valid check word
	binary.LittleEndian.PutUint16(tiny[0:2], 4)
	binary.LittleEndian.PutUint16(tiny[2:4], 4^ChecksumWord)
	stream := append([]byte{0xDE, 0xAD}, tiny...)
	stream = append(stream, valid...)

	p := NewParser()
	pkts, _ := p.Feed(stream) // resync error expected, frame must survive
	if len(pkts) != 1 || pkts[0].SubCommand() != SubCmdConnect {
		t.Fatalf("frame not recovered after garbage: %d packets", len(pkts))
	}
	pkts, err := p.Feed(valid)
	if err != nil || len(pkts) != 1 {
		t.Fatalf("parser wedged after resync: err=%v pkts=%d", err, len(pkts))
	}
}

// TestReadMsgsZeroTimeout: per J2534, a zero timeout returns the messages
// already received instead of racing the deadline.
func TestReadMsgsZeroTimeout(t *testing.T) {
	d := New()
	d.rxQueue <- &CANFrame{ID: 0x7E8}
	out, err := d.PassThruReadMsgs(10, 0)
	if err != nil || len(out) != 1 || out[0].ID != 0x7E8 {
		t.Fatalf("out=%v err=%v, want the queued frame", out, err)
	}
}

func TestWriteMsgsRejectsOversized(t *testing.T) {
	if err := New().PassThruWriteMsgs(0x7E0, make([]byte, 9)); err == nil {
		t.Fatal("expected error for >8 data bytes")
	}
}

func TestExtractVersionsShortInput(t *testing.T) {
	if fi := ExtractVersions(make([]byte, 10)); fi != (FirmwareInfo{}) {
		t.Fatalf("want zero value for short input, got %+v", fi)
	}
}

// lastWrite records the most recent write and answers it with the device's
// 0x08 reply: the next of codes, then txQueued. The embedded nil Port panics
// on anything else.
type lastWrite struct {
	serial.Port
	d       *Device
	b       []byte
	codes   []uint32
	reply   Packet
	payload [12]byte
}

func (w *lastWrite) Write(p []byte) (int, error) {
	w.b = append(w.b[:0], p...)
	code := txQueued
	if len(w.codes) > 0 {
		code, w.codes = w.codes[0], w.codes[1:]
	}
	w.payload[0] = SubCmdCANTx
	w.payload[1] = FlagResponseFinal
	copy(w.payload[2:4], p[10:12]) // the Tx seq
	binary.LittleEndian.PutUint32(w.payload[8:12], code)
	w.reply = Packet{Direction: DirectionIn, Payload: w.payload[:]}
	w.d.handlePacket(&w.reply)
	return len(p), nil
}

func newWriteDevice(codes ...uint32) (*Device, *lastWrite) {
	d := New()
	w := &lastWrite{d: d, codes: codes}
	d.port = w
	return d, w
}

// TestWriteMsgsInPlace checks the frame built in the reused buffer is the
// one cmd+ToBytes produced, and that sending it allocates nothing.
func TestWriteMsgsInPlace(t *testing.T) {
	d, w := newWriteDevice()
	data := []byte{0x40, 0xA1, 0x02, 0x1A, 0x90}
	if err := d.PassThruWriteMsgs(0x7E0, data); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 32)
	payload[0] = SubCmdCANTx
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], 1)
	binary.LittleEndian.PutUint16(payload[4:6], 0x0001)
	binary.LittleEndian.PutUint16(payload[16:18], uint16(4+len(data)))
	binary.BigEndian.PutUint32(payload[20:24], 0x7E0)
	copy(payload[24:], data)
	if want := cmd(canChannel, payload).ToBytes(); !bytes.Equal(w.b, want) {
		t.Fatalf("wire\n got % X\nwant % X", w.b, want)
	}
	if n := testing.AllocsPerRun(100, func() { d.PassThruWriteMsgs(0x7E0, data) }); n != 0 {
		t.Errorf("PassThruWriteMsgs allocates %v times per frame, want 0", n)
	}
}

// A full device queue is retried; a refused frame is an error.
func TestWriteMsgsWaitsForQueued(t *testing.T) {
	d, w := newWriteDevice(txQueueFull, txQueueFull)
	if err := d.PassThruWriteMsgs(0x7E0, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if seq := binary.LittleEndian.Uint16(w.b[10:12]); seq != 3 {
		t.Fatalf("queued by write %d, want 3", seq)
	}
	w.codes = []uint32{0x10C}
	if err := d.PassThruWriteMsgs(0x7E0, []byte{1}); err == nil {
		t.Fatal("a refused frame returned nil")
	}
}

// rxCapture is a real RX push (id 0x7E8, data 01 50) from a Windows USB capture.
var rxCapture = []byte{
	0x1e, 0x00, 0xf8, 0x51, 0x00, 0x00, 0x01, 0x05,
	0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x48, 0x0b,
	0x00, 0x00, 0x00, 0x00, 0xfc, 0x37, 0x0d, 0x00,
	0x06, 0x00, 0x06, 0x00, 0x00, 0x00, 0x07, 0xe8,
	0x01, 0x50,
}

// TestReadPathAllocs runs RX pushes through what readLoop does per read: the
// only allocations left are the CANFrame handed to the callback and queues.
func TestReadPathAllocs(t *testing.T) {
	var got CANFrame
	d := New(WithCANFrameHandler(func(f *CANFrame) { got = *f }))
	rx := rxCapture
	var packets []Packet
	n := testing.AllocsPerRun(1000, func() {
		var err error
		if packets, err = d.parser.parse(rx, packets[:0]); err != nil || len(packets) != 1 {
			t.Fatalf("parse: %d packets, %v", len(packets), err)
		}
		d.handlePacket(&packets[0])
	})
	if got.ID != 0x7E8 || !bytes.Equal(got.Data, []byte{0x01, 0x50}) {
		t.Fatalf("callback got %v", &got)
	}
	if n > 2 {
		t.Errorf("read path allocates %v times per frame, want <= 2 (frame + data)", n)
	}
	if q := len(d.rxQueue); q != 0 {
		t.Errorf("%d frames queued for PassThruReadMsgs behind a handler", q)
	}
}

// errPort fails every read, like an unplugged device.
type errPort struct{ serial.Port }

func (errPort) Read([]byte) (int, error) { return 0, errors.New("EIO") }

func TestReadLoopStopsWhenDeviceGone(t *testing.T) {
	var errs []error
	d := New(WithErrorHandler(func(err error) { errs = append(errs, err) }))
	d.port = errPort{}
	d.running.Store(true)
	d.wg.Add(1)
	d.readLoop()
	if len(errs) != 1 || !errors.Is(errs[0], ErrDeviceGone) {
		t.Fatalf("errors %v, want one ErrDeviceGone", errs)
	}
}

// handshakePort answers like a Mongoose whose firmware is already running: no
// reply to the ECHO, a success reply to everything else unless status names
// one, every reply carrying device handle 0x1234 at body[20:24].
type handshakePort struct {
	serial.Port
	rx     chan []byte
	status map[uint16]uint32 // reply status by command, body[4:6]
	wrote  [][]byte
}

func (p *handshakePort) Write(b []byte) (int, error) {
	p.wrote = append(p.wrote, bytes.Clone(b))
	c := binary.LittleEndian.Uint16(b[8:10])
	if c == 0x0100 { // ECHO
		return len(b), nil
	}
	r := make([]byte, 8+24)
	binary.LittleEndian.PutUint16(r[0:2], uint16(len(r)-4))
	binary.LittleEndian.PutUint16(r[2:4], uint16(len(r)-4)^ChecksumWord)
	r[6] = 0x01 // from board 1
	binary.LittleEndian.PutUint16(r[8:10], c|0x8000)
	copy(r[10:12], b[10:12])
	binary.LittleEndian.PutUint32(r[16:20], p.status[c])
	binary.LittleEndian.PutUint32(r[24:28], 0x1234)
	p.rx <- r
	return len(b), nil
}

func (p *handshakePort) Read(b []byte) (int, error) {
	select {
	case r := <-p.rx:
		return copy(b, r), nil
	case <-time.After(10 * time.Millisecond):
		return 0, nil
	}
}

func (p *handshakePort) Close() error { return nil }

func newHandshakeDevice(t *testing.T, status map[uint16]uint32) (*Device, *handshakePort) {
	p := &handshakePort{rx: make(chan []byte, 16), status: status}
	d := New()
	d.port = p
	d.running.Store(true)
	d.wg.Add(1)
	go d.readLoop()
	t.Cleanup(func() { d.Close() })
	return d, p
}

func TestOpenWithoutEchoReply(t *testing.T) {
	d, _ := newHandshakeDevice(t, nil)
	if err := d.PassThruOpen(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenWithoutBatteryVoltage(t *testing.T) {
	d, _ := newHandshakeDevice(t, map[uint16]uint32{0x0003: stVBattLoss})
	if err := d.PassThruOpen(); err == nil || !strings.Contains(err.Error(), "no voltage") {
		t.Fatalf("err = %v, want no voltage on the vehicle connector", err)
	}
}

// A filter is stopped with op 0x0E and the handle the device gave it.
func TestFilterHandle(t *testing.T) {
	d, p := newHandshakeDevice(t, nil)
	id, err := d.PassThruStartMsgFilter(PASS_FILTER, 0x258, 0xFFFFFFFF)
	if err != nil || id != 0x1234 {
		t.Fatalf("filter id %#x, %v; want the device handle 0x1234", id, err)
	}
	if err := d.PassThruStopMsgFilter(id); err != nil {
		t.Fatal(err)
	}
	w := p.wrote[len(p.wrote)-1]
	if w[8] != SubCmdDeleteObject || binary.LittleEndian.Uint32(w[16:20]) != objPass || binary.LittleEndian.Uint32(w[20:24]) != 0x1234 {
		t.Fatalf("stop sent % X", w)
	}
}

func TestSeqWrapsLikeTheDLL(t *testing.T) {
	d := New()
	for want := range 600 {
		if got := d.nextSeq(); got != uint16(want%255+1) {
			t.Fatalf("call %d: seq %d, want %d", want, got, want%255+1)
		}
	}
}

// Whatever the port delivers, parsing and handling it must not panic, and a
// frame never carries more than 8 bytes.
func FuzzReadPath(f *testing.F) {
	f.Add(rxCapture)
	f.Add(append(bytes.Clone(rxCapture[:20]), rxCapture...))
	f.Fuzz(func(t *testing.T, data []byte) {
		d := New(WithCANFrameHandler(func(fr *CANFrame) {
			if fr.DLC > 8 || len(fr.Data) != fr.DLC {
				t.Fatalf("frame %v from % X", fr, data)
			}
		}))
		packets, _ := d.parser.parse(data, nil)
		for i := range packets {
			d.handlePacket(&packets[i])
		}
	})
}
