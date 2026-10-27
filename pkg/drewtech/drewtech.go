// Package drewtech is a native (no Windows DLL) J2534 client for the DrewTech
// Mongoose Pro GM II. It speaks the Mongoose wire protocol directly over the
// device's virtual COM port at 115200 baud.
//
// The wire protocol is decoded from the Windows DLL in devel/drewtech
// (re/*.md; PROTOCOL.md there predates it and is wrong about 0x10). The
// libj2534_mongoose C driver implements the same protocol in full and is the
// reference when the two disagree. The notable facts:
//   - framing check word is length^0x51E6 (LE), not a constant magic byte
//   - replies carry the command with 0x8000 set and echo the request's
//     sequence, a 1..255 byte counter; status is a u32 at body[12:16]
//   - RX arrives as unsolicited 0x0009 pushes; there is no "read messages" command
//   - 0x08/0x0A are Tx confirmations carrying the Tx sequence, not RX
//   - a pass filter (0x0D) per CAN id is required or you receive nothing;
//     0x0E removes one by the handle the 0x0D reply gave, 0x10 all of a type
//   - 0x11 is a multiplexed control op (subtype at payload[8:12]):
//     GetStatus, clear-TX=2, clear-RX=3, become-master=4
//
// This file is the wire layer (framing + device transport + CAN data path).
// The J2534 API facade lives in j2534.go.
package drewtech

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

const (
	MagicByte    uint8  = 0x51
	ChecksumXOR  uint8  = 0xE6
	ChecksumWord uint16 = 0x51E6 // check = length ^ 0x51E6 (LE)

	DirectionOut uint8 = 0x01
	DirectionIn  uint8 = 0x00

	FlagRequest       uint8 = 0x01
	FlagRequestFinal  uint8 = 0x00
	FlagResponse      uint8 = 0x81
	FlagResponseFinal uint8 = 0x80

	SubCmdInit           uint8 = 0x00
	SubCmdGetVersion     uint8 = 0x03
	SubCmdClose          uint8 = 0x05
	SubCmdConnect        uint8 = 0x06
	SubCmdDisconnect     uint8 = 0x07
	SubCmdCANTx          uint8 = 0x08
	SubCmdCANRx          uint8 = 0x09
	SubCmdTxStatus       uint8 = 0x0A // device->host Tx completion status
	SubCmdDeleteObject   uint8 = 0x0E // remove one filter or periodic: type, device handle
	SubCmdClearFilters   uint8 = 0x10 // remove every object of one type
	SubCmdGetConfig      uint8 = 0x0C // get config (there is no runtime set)
	SubCmdStartMsgFilter uint8 = 0x0D
	SubCmdControl        uint8 = 0x11 // multiplexed: status / clear buffers / master
	SubCmdSetProtocol    uint8 = 0x12
	SubCmdGetSerial      uint8 = 0x13

	// SubCmdControl (0x11) subtypes, at payload[8:12].
	ctlClearTxBuffer uint32 = 2
	ctlClearRxBuffer uint32 = 3
	ctlBecomeMaster  uint32 = 4
	// Object types for SubCmdStartMsgFilter / SubCmdDeleteObject / SubCmdClearFilters.
	objPass  uint32 = 0
	objBlock uint32 = 1

	stVBattLoss uint32 = 0x20A // OPEN_DEVICE status: no battery voltage on pin 16

	ProtocolCAN uint32 = 6

	ParamDataRate       uint32 = 0x04
	ParamBitSamplePoint uint32 = 0x14
	ParamSyncJumpWidth  uint32 = 0x15

	canChannel uint8 = 5
)

// CANFrame represents a CAN bus frame.
type CANFrame struct {
	ID        uint32
	Data      []byte
	DLC       int
	Timestamp uint32
}

func (f *CANFrame) String() string {
	return fmt.Sprintf("ID=%03X [%d] %X ts=%d", f.ID, f.DLC, f.Data, f.Timestamp)
}

// Packet represents a protocol packet. Payload starts at the command opcode
// (wire offset 8): Payload[0]=opcode, Payload[1]=flag, Payload[2:4]=sequence.
type Packet struct {
	Length    uint16
	Direction uint8
	Channel   uint8
	Payload   []byte
}

func (p *Packet) ToBytes() []byte {
	buf := make([]byte, int(p.Length)+4)
	p.putHeader(buf)
	copy(buf[8:], p.Payload)
	return buf
}

// putHeader writes the 8-byte wire header into buf, leaving buf[6:8] alone.
func (p *Packet) putHeader(buf []byte) {
	binary.LittleEndian.PutUint16(buf[0:2], p.Length)
	// Check is length^0x51E6 as an LE u16. For len<256 the high byte is always
	// 0x51 so it looks like a constant magic; for len>=256 (flashing) it is not.
	binary.LittleEndian.PutUint16(buf[2:4], p.Length^ChecksumWord)
	buf[4] = p.Direction
	buf[5] = p.Channel
}

func (p *Packet) SubCommand() uint8 {
	if len(p.Payload) > 0 {
		return p.Payload[0]
	}
	return 0
}

func (p *Packet) Sequence() uint16 {
	if len(p.Payload) >= 4 {
		return binary.LittleEndian.Uint16(p.Payload[2:4])
	}
	return 0
}

// IsCANRx reports whether this is an unsolicited RX push, command 0x0009.
// Board-info replies share the opcode but carry the reply flag (0x8109).
func (p *Packet) IsCANRx() bool {
	return len(p.Payload) >= 2 && p.Payload[0] == SubCmdCANRx && p.Payload[1] == 0
}

func (p *Packet) ParseCANFrame() (*CANFrame, error) {
	if len(p.Payload) < 24 {
		return nil, fmt.Errorf("payload too short: %d", len(p.Payload))
	}
	dataLen := int(binary.LittleEndian.Uint16(p.Payload[18:20]))
	dlc := dataLen - 4 // dataLen counts the 4-byte id
	avail := len(p.Payload) - 24
	if dlc < 0 {
		dlc = 0
	}
	if dlc > avail {
		dlc = avail
	}
	if dlc > 8 {
		dlc = 8
	}
	data := make([]byte, dlc)
	copy(data, p.Payload[24:])
	return &CANFrame{
		Timestamp: binary.LittleEndian.Uint32(p.Payload[12:16]),
		ID:        binary.BigEndian.Uint32(p.Payload[20:24]),
		Data:      data,
		DLC:       dlc,
	}, nil
}

// Parser handles streaming packet parsing.
type Parser struct {
	buf []byte
	off int // start of the unparsed bytes in buf
}

func NewParser() *Parser {
	return &Parser{buf: make([]byte, 0, 4096)}
}

// Feed appends data and returns the complete packets now buffered, each with
// its own copy of the payload.
func (p *Parser) Feed(data []byte) ([]*Packet, error) {
	pkts, err := p.parse(data, nil)
	var packets []*Packet
	for _, pkt := range pkts {
		pkt.Payload = bytes.Clone(pkt.Payload)
		packets = append(packets, &pkt)
	}
	return packets, err
}

// parse appends data and then every complete packet buffered to dst. The
// packets' Payloads alias the parser's buffer, so they are only valid until
// the next call: the read loop handles them in place rather than paying a
// Packet and a payload copy per frame.
func (p *Parser) parse(data []byte, dst []Packet) ([]Packet, error) {
	// Slide the unparsed tail back to the front instead of letting the
	// window walk off the end of buf, which reallocated it on every read.
	if p.off > 0 && len(p.buf)+len(data) > cap(p.buf) {
		p.buf = p.buf[:copy(p.buf, p.buf[p.off:])]
		p.off = 0
	}
	p.buf = append(p.buf, data...)

	dropped := 0
	for len(p.buf)-p.off >= 4 {
		b := p.buf[p.off:]
		length := binary.LittleEndian.Uint16(b[0:2])
		check := binary.LittleEndian.Uint16(b[2:4])

		// Validate the full check word (length^0x51E6, PROTOCOL.md §2) so
		// >=256B frames pass too; the high byte only looks like a constant
		// 0x51 below 256B. Every real body is at least 12 bytes, so a shorter
		// one is garbage. On any mismatch drop one byte and rescan —
		// always makes progress, never wedges on a corrupt stream.
		if check != length^ChecksumWord || length < 12 {
			p.off++
			dropped++
			continue
		}

		wireLen := int(length) + 4
		if len(b) < wireLen {
			break
		}

		var direction, channel uint8
		if b[4] == DirectionOut {
			direction = DirectionOut
			channel = b[5]
		} else {
			direction = DirectionIn
			channel = b[7]
		}

		dst = append(dst, Packet{
			Length:    length,
			Direction: direction,
			Channel:   channel,
			Payload:   b[8:wireLen:wireLen],
		})
		p.off += wireLen
	}

	if dropped > 0 {
		return dst, fmt.Errorf("resync: dropped %d bytes", dropped)
	}
	return dst, nil
}

// pendingRequest tracks a request waiting for response.
type pendingRequest struct {
	seq      uint16
	response chan *Packet
}

// Device represents a DrewTech J2534 device.
type Device struct {
	port    serial.Port
	parser  *Parser
	seq     uint32
	channel uint8

	running  atomic.Bool
	wg       sync.WaitGroup
	mu       sync.Mutex
	pending  map[uint16]*pendingRequest
	canRxCh  chan *CANFrame
	rxQueue  chan *CANFrame // host-side FIFO drained by PassThruReadMsgs
	overflow atomic.Bool    // set when rxQueue dropped a frame
	closeCh  chan struct{}

	// Cached info populated by PassThruOpen, surfaced via PassThruReadVersion.
	info   FirmwareInfo
	serial string

	// Bookkeeping for CLEAR_MSG_FILTERS / CLEAR_PERIODIC_MSGS.
	filters     []uint32
	periodics   map[uint32]*periodicMsg
	periodicSeq uint32

	lastErr atomic.Value // error

	onCANFrame func(*CANFrame)
	onError    func(error)

	// PassThruWriteMsgs builds its wire frame here; txMu covers the periodic
	// senders, which write concurrently with the caller.
	txMu    sync.Mutex
	txBuf   [8 + 32]byte
	txReply chan txReply
	txTimer *time.Timer
}

// txReply is the device's answer to a Tx: the 0x08 reply code and the Tx seq.
type txReply struct {
	seq  uint16
	code uint32
}

// 0x08 reply codes, from the DLL's WriteMsgs state machine.
const (
	txQueued    uint32 = 0x100
	txQueueFull uint32 = 0x101
	txBusFault  uint32 = 0x105
)

// Option configures the device.
type Option func(*Device)

// WithCANFrameHandler sets the CAN frame callback. Frames pushed to a handler
// or channel are not also queued for PassThruReadMsgs.
func WithCANFrameHandler(handler func(*CANFrame)) Option {
	return func(d *Device) { d.onCANFrame = handler }
}

// WithErrorHandler sets the error callback.
func WithErrorHandler(handler func(error)) Option {
	return func(d *Device) { d.onError = handler }
}

// WithCANChannel sets a channel for receiving CAN frames.
func WithCANChannel(ch chan *CANFrame) Option {
	return func(d *Device) { d.canRxCh = ch }
}

// New creates a new Device.
func New(opts ...Option) *Device {
	d := &Device{
		parser:    NewParser(),
		pending:   make(map[uint16]*pendingRequest),
		periodics: make(map[uint32]*periodicMsg),
		rxQueue:   make(chan *CANFrame, 8192),
		closeCh:   make(chan struct{}),
		channel:   canChannel,
		txReply:   make(chan txReply, 8),
		txTimer:   time.NewTimer(0),
	}
	d.txTimer.Stop()
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Open opens the serial port and starts the read loop.
func (d *Device) Open(portName string) error {
	port, err := serial.Open(portName, &serial.Mode{BaudRate: 115200})
	if err != nil {
		return fmt.Errorf("open serial: %w", err)
	}
	d.port = port
	d.port.SetReadTimeout(50 * time.Millisecond)
	d.port.ResetInputBuffer() // what an earlier session left unread would start the parse out of sync

	d.running.Store(true)
	d.wg.Add(1)
	go d.readLoop()
	return nil
}

// Close stops periodic messages, the read loop and closes the port.
func (d *Device) Close() error {
	if !d.running.Load() {
		return nil
	}
	d.stopAllPeriodic()
	d.running.Store(false)
	close(d.closeCh)
	d.wg.Wait()
	if d.port != nil {
		return d.port.Close()
	}
	return nil
}

// ErrDeviceGone is reported to the error handler when reading the port fails,
// which ends the read loop: the device was unplugged or the port broke.
var ErrDeviceGone = errors.New("drewtech: device gone")

func (d *Device) readLoop() {
	defer d.wg.Done()
	buf := make([]byte, 1024)
	var packets []Packet // reused; payloads alias the parser's buffer
	for d.running.Load() {
		n, err := d.port.Read(buf)
		if err != nil {
			if d.running.Load() {
				d.handleError(fmt.Errorf("%w: %w", ErrDeviceGone, err))
			}
			return
		}
		if n == 0 {
			continue
		}
		packets, err = d.parser.parse(buf[:n], packets[:0])
		if err != nil {
			d.handleError(err)
		}
		for i := range packets {
			d.handlePacket(&packets[i])
		}
	}
}

// handlePacket must not keep pkt: its payload is the parser's buffer.
func (d *Device) handlePacket(pkt *Packet) {
	if pkt.Direction != DirectionIn {
		return
	}

	if pkt.IsCANRx() {
		frame, err := pkt.ParseCANFrame()
		if err != nil {
			d.handleError(err)
			return
		}
		d.mu.Lock()
		cb := d.onCANFrame
		d.mu.Unlock()
		if cb != nil {
			cb(frame)
		}
		if d.canRxCh != nil {
			select {
			case d.canRxCh <- frame:
			default: // channel full, drop
			}
		}
		if cb != nil || d.canRxCh != nil {
			return // pushed to the consumer; queueing too would fill rxQueue for good
		}
		// Host-side FIFO for PassThruReadMsgs; flag overflow like the DLL does.
		select {
		case d.rxQueue <- frame:
		default:
			d.overflow.Store(true)
		}
		return
	}

	// Every 0x08 Tx gets a 0x08 "queued" reply, which PassThruWriteMsgs waits
	// for, and a 0x0A "sent" status; both carry the Tx seq.
	switch pkt.SubCommand() {
	case SubCmdCANTx:
		if len(pkt.Payload) >= 12 {
			select {
			case d.txReply <- txReply{pkt.Sequence(), binary.LittleEndian.Uint32(pkt.Payload[8:12])}:
			default:
			}
		}
		return
	case SubCmdTxStatus:
		if len(pkt.Payload) >= 12 {
			if status := binary.LittleEndian.Uint32(pkt.Payload[8:12]); txFault(status) {
				d.handleError(fmt.Errorf("tx failed: status 0x%X", status))
			}
		}
		return
	}

	// Response to a pending request, matched by sequence.
	seq := pkt.Sequence()
	d.mu.Lock()
	req, ok := d.pending[seq]
	if ok {
		delete(d.pending, seq)
	}
	d.mu.Unlock()
	if ok {
		resp := *pkt
		resp.Payload = bytes.Clone(pkt.Payload)
		select {
		case req.response <- &resp:
		default:
		}
	}
}

// txFault reports whether a 0x0A Tx-status code means the frame failed to
// transmit (vs queued / sent OK). Codes from monpa432.dll.
func txFault(status uint32) bool {
	switch status {
	case 0x102, 0x103, 0x104, 0x105, 0x10C, 0x10D, 0x112, 0x113, 0x114, 0x118, 0x11E, 0x11F, 0x120, 0x121:
		return true
	}
	return false
}

func (d *Device) handleError(err error) {
	d.lastErr.Store(err)
	if d.onError != nil {
		d.onError(err)
	}
}

// nextSeq counts 1..255 like the DLL, which compares the sequence as a byte.
func (d *Device) nextSeq() uint16 {
	return uint16((atomic.AddUint32(&d.seq, 1)-1)%255 + 1)
}

// cmd builds an outbound command packet, deriving Length from the payload.
func cmd(channel uint8, payload []byte) *Packet {
	return &Packet{
		Length:    uint16(len(payload) + 4),
		Direction: DirectionOut,
		Channel:   channel,
		Payload:   payload,
	}
}

func (d *Device) send(pkt *Packet) error {
	_, err := d.port.Write(pkt.ToBytes())
	return err
}

// do sends pkt and returns its reply, failing on a non-zero device status.
func (d *Device) do(pkt *Packet) (*Packet, error) {
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return resp, statusErr(resp)
}

// statusErr reports the status a reply carries at body[12:16], with the
// firmware's error text after it.
func statusErr(resp *Packet) error {
	if len(resp.Payload) < 12 {
		return nil
	}
	st := binary.LittleEndian.Uint32(resp.Payload[8:12])
	if st == 0 {
		return nil
	}
	var text []byte
	if len(resp.Payload) > 16 {
		text = resp.Payload[16:]
		if i := bytes.IndexByte(text, 0); i >= 0 {
			text = text[:i]
		}
	}
	return fmt.Errorf("device: %s (0x%X)", text, st)
}

func (d *Device) sendAndWait(pkt *Packet, timeout time.Duration) (*Packet, error) {
	seq := pkt.Sequence()
	req := &pendingRequest{seq: seq, response: make(chan *Packet, 1)}

	d.mu.Lock()
	d.pending[seq] = req
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.pending, seq)
		d.mu.Unlock()
	}()

	if err := d.send(pkt); err != nil {
		return nil, err
	}
	select {
	case resp := <-req.response:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for seq %d", seq)
	case <-d.closeCh:
		return nil, fmt.Errorf("device closed")
	}
}

// --- Wire commands (low level) ---

// init is the ECHO the open sequence starts with. Like the DLL it waits 250 ms
// and ignores the result: a device whose firmware is already running, after
// an earlier session, doesn't answer it.
func (d *Device) init() {
	payload := make([]byte, 8)
	payload[0] = SubCmdInit
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	d.sendAndWait(cmd(0, payload), 250*time.Millisecond)
}

func (d *Device) getDeviceInfo() (FirmwareInfo, error) {
	payload := make([]byte, 8)
	payload[0] = SubCmdCANRx
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	resp, err := d.sendAndWait(cmd(0, payload), 2*time.Second)
	if err != nil {
		return FirmwareInfo{}, err
	}
	return ExtractVersions(resp.Payload), nil
}

// getConfig reads a device-level config param (channel 0) during the open
// handshake. Only the ack matters; the DLL discards the value too.
func (d *Device) getConfig(param uint32) error {
	payload := make([]byte, 12)
	payload[0] = SubCmdGetConfig
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], param)
	_, err := d.sendAndWait(cmd(0, payload), 2*time.Second)
	return err
}

func (d *Device) getStatus() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdControl // 0x11 with flag=1 = status query
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	_, err := d.sendAndWait(cmd(0, payload), 2*time.Second)
	return err
}

func (d *Device) getFirmwareVersion() (string, error) {
	payload := make([]byte, 8)
	payload[0] = SubCmdGetVersion
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	resp, err := d.sendAndWait(cmd(0, payload), 2*time.Second)
	if err != nil {
		return "", err
	}
	if len(resp.Payload) > 12 {
		end := len(resp.Payload)
		for i := 12; i < len(resp.Payload); i++ {
			if resp.Payload[i] == 0 {
				end = i
				break
			}
		}
		return string(resp.Payload[12:end]), nil
	}
	return "", nil
}

// openDevice is the DLL's OPEN_DEVICE, which fails without battery voltage on
// the vehicle connector.
func (d *Device) openDevice() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdGetVersion
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	resp, err := d.sendAndWait(cmd(0, payload), 2*time.Second)
	if err != nil {
		return err
	}
	if len(resp.Payload) >= 12 && binary.LittleEndian.Uint32(resp.Payload[8:12]) == stVBattLoss {
		return errors.New("no voltage on the vehicle connector")
	}
	return statusErr(resp)
}

func (d *Device) getSerial() (string, error) {
	payload := make([]byte, 12)
	payload[0] = SubCmdGetSerial
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	resp, err := d.sendAndWait(cmd(0, payload), 2*time.Second)
	if err != nil {
		return "", err
	}
	if len(resp.Payload) <= 20 {
		return "", nil
	}
	s := resp.Payload[20:] // NUL-terminated, after the status
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s), nil
}

// control issues an 0x11 control command with the given subtype and optional
// trailing bytes (become-master carries a poll-id byte).
func (d *Device) control(subtype uint32, extra ...byte) error {
	payload := make([]byte, 12+len(extra))
	payload[0] = SubCmdControl
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], subtype)
	copy(payload[12:], extra)
	_, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second)
	return err
}
