package drewtech

import (
	"bytes"
	"encoding/binary"
	"testing"
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
	cases := []struct {
		length uint16
		op     uint8
		want   bool
	}{
		{0x1e, SubCmdCANRx, true},  // real 2-byte-data frame
		{0x24, SubCmdCANRx, true},  // max
		{0x2c, SubCmdCANRx, false}, // device-info reply also uses op 0x09
		{0x24, SubCmdCANTx, false}, // tx echo
	}
	for _, c := range cases {
		p := &Packet{Length: c.length, Payload: []byte{c.op}}
		if got := p.IsCANRx(); got != c.want {
			t.Errorf("len=%02X op=%02X: IsCANRx=%v want %v", c.length, c.op, got, c.want)
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
