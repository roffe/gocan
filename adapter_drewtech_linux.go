package gocan

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "Drewtech Mongoose",
		Description:        "Drewtech Mongoose Linux Driver",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: true,
		},
		New: NewDrewtech,
	}); err != nil {
		panic(err)
	}
}

type Drewtech struct {
	*BaseAdapter
	dev *Device
}

func NewDrewtech(cfg *AdapterConfig) (Adapter, error) {
	return &Drewtech{
		BaseAdapter: NewBaseAdapter("Drewtech Mongoose", cfg),
	}, nil
}

func (a *Drewtech) Open(ctx context.Context) error {
	// Create device with options
	dev := NewDrewtechDevice(
		WithCANChannel(a.recvChan),
		WithErrorHandler(a.Error),
	)
	a.dev = dev

	if err := a.dev.Open(a.cfg.Port); err != nil {
		return fmt.Errorf("open device: %w", err)
	}

	// Initialize device
	log.Println("Opening device...")
	if err := dev.PassThruOpen(); err != nil {
		return fmt.Errorf("PassThruOpen failed: %w", err)
	}

	// Get serial number
	serial, _ := dev.GetSerial()
	log.Printf("Device serial: %s", serial)

	// Connect to CAN at 500kbps
	log.Println("Connecting to CAN at 500kbps...")
	if err := dev.PassThruConnect(500000); err != nil {
		return fmt.Errorf("PassThruConnect failed: %w", err)
	}

	// Start pass-all filter
	log.Println("Starting filter...")
	if err := dev.PassThruStartMsgFilter(); err != nil {
		return fmt.Errorf("PassThruStartMsgFilter failed: %w", err)
	}

	go a.sendManager(ctx)

	return nil
}

func (a *Drewtech) SetFilter(filters []uint32) error {
	return nil
}

func (a *Drewtech) Close() error {
	log.Println("Close")
	a.BaseAdapter.Close()
	if a.dev != nil {
		// Stop filter
		if err := a.dev.PassThruStopMsgFilter(); err != nil {
			log.Printf("StopMsgFilter: %v", err)
		}

		// Small delay to process remaining frames
		time.Sleep(100 * time.Millisecond)

		// Disconnect
		if err := a.dev.PassThruDisconnect(); err != nil {
			log.Printf("Disconnect: %v", err)
		}

		// Close device
		if err := a.dev.PassThruClose(); err != nil {
			log.Printf("Close: %v", err)
		}

		a.dev.Close()
	}
	return nil
}

func (a *Drewtech) sendManager(ctx context.Context) {
	defer log.Println("exit sendmanager")
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.closeChan:
			return
		case f := <-a.sendChan:
			err := a.dev.PassThruWriteMsgs(f.Identifier, f.Data)
			if err != nil {
				a.Error(fmt.Errorf("send error: %w", err))
			}
		}
	}
}

// =========================

const (
	MagicByte   uint8 = 0x51
	ChecksumXOR uint8 = 0xE6

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
	SubCmdSetConfig      uint8 = 0x0B
	SubCmdGetConfig      uint8 = 0x0C
	SubCmdStartMsgFilter uint8 = 0x0D
	SubCmdStopMsgFilter  uint8 = 0x10
	SubCmdGetStatus      uint8 = 0x11
	SubCmdSetProtocol    uint8 = 0x12
	SubCmdGetSerial      uint8 = 0x13

	ProtocolCAN uint32 = 6

	ParamDataRate       uint32 = 0x04
	ParamBitSamplePoint uint32 = 0x14
	ParamSyncJumpWidth  uint32 = 0x15
)

/*
// CANFrame represents a CAN bus frame
type CANFrame struct {
	ID        uint32
	Data      []byte
	DLC       int
	Timestamp uint32
}

func (f *CANFrame) String() string {
	return fmt.Sprintf("ID=%03X [%d] %X ts=%d", f.ID, f.DLC, f.Data[:f.DLC], f.Timestamp)
}
*/

// Packet represents a protocol packet
type Packet struct {
	Length    uint16
	Direction uint8
	Channel   uint8
	Payload   []byte
}

func (p *Packet) ToBytes() []byte {
	wireLen := int(p.Length) + 4
	buf := make([]byte, wireLen)
	binary.LittleEndian.PutUint16(buf[0:2], p.Length)
	buf[2] = buf[0] ^ ChecksumXOR
	buf[3] = MagicByte
	buf[4] = p.Direction
	buf[5] = p.Channel
	copy(buf[8:], p.Payload)
	return buf
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

func (p *Packet) IsCANRx() bool {
	return p.Length == 0x24 && len(p.Payload) >= 1 && p.Payload[0] == SubCmdCANRx
}

func (p *Packet) ParseCANFrame() (*CANFrame, error) {
	if len(p.Payload) < 28 {
		return nil, fmt.Errorf("payload too short")
	}
	dataLen := int(binary.LittleEndian.Uint16(p.Payload[18:20]))
	dlc := dataLen - 4
	if dlc < 0 {
		dlc = 0
	}
	if dlc > 8 {
		dlc = 8
	}
	data := make([]byte, 8)
	copy(data, p.Payload[24:32])
	return &CANFrame{
		//Timestamp:  binary.LittleEndian.Uint32(p.Payload[12:16]),
		Identifier: binary.BigEndian.Uint32(p.Payload[20:24]),
		Data:       data,
		//DLC:        dlc,
	}, nil
}

// Parser handles streaming packet parsing
type Parser struct {
	buf []byte
}

func NewParser() *Parser {
	return &Parser{buf: make([]byte, 0, 4096)}
}

func (p *Parser) Feed(data []byte) ([]*Packet, error) {
	p.buf = append(p.buf, data...)
	var packets []*Packet

	for {
		if len(p.buf) < 4 {
			break
		}

		length := binary.LittleEndian.Uint16(p.buf[0:2])
		checksum := p.buf[2]
		magic := p.buf[3]

		if magic != MagicByte {
			// Try to resync by finding next magic byte
			for i := 1; i < len(p.buf); i++ {
				if p.buf[i] == MagicByte && i >= 3 {
					p.buf = p.buf[i-3:]
					break
				}
			}
			return packets, fmt.Errorf("invalid magic: %02X", magic)
		}

		expectedChecksum := uint8(length) ^ ChecksumXOR
		if checksum != expectedChecksum {
			p.buf = p.buf[1:]
			continue
		}

		wireLen := int(length) + 4
		if length < 8 || len(p.buf) < wireLen {
			break
		}

		var direction, channel uint8
		if p.buf[4] == DirectionOut {
			direction = DirectionOut
			channel = p.buf[5]
		} else {
			direction = DirectionIn
			channel = p.buf[7]
		}

		pkt := &Packet{
			Length:    length,
			Direction: direction,
			Channel:   channel,
			Payload:   make([]byte, wireLen-8),
		}
		copy(pkt.Payload, p.buf[8:wireLen])
		packets = append(packets, pkt)
		p.buf = p.buf[wireLen:]
	}

	return packets, nil
}

// pendingRequest tracks a request waiting for response
type pendingRequest struct {
	seq      uint16
	response chan *Packet
}

// Device represents a DrewTech J2534 device
type Device struct {
	port   serial.Port
	parser *Parser

	seq      uint32
	channel  uint8
	filterID uint32

	// Async handling
	running atomic.Bool
	wg      sync.WaitGroup
	mu      sync.Mutex
	pending map[uint16]*pendingRequest
	canRxCh chan *CANFrame
	closeCh chan struct{}
	errCh   chan error

	// Callbacks
	onCANFrame func(*CANFrame)
	onError    func(error)
}

// Option configures the device
type Option func(*Device)

// WithCANFrameHandler sets the CAN frame callback
func WithCANFrameHandler(handler func(*CANFrame)) Option {
	return func(d *Device) {
		d.onCANFrame = handler
	}
}

// WithErrorHandler sets the error callback
func WithErrorHandler(handler func(error)) Option {
	return func(d *Device) {
		d.onError = handler
	}
}

// WithCANChannel sets a channel for receiving CAN frames
func WithCANChannel(ch chan *CANFrame) Option {
	return func(d *Device) {
		d.canRxCh = ch
	}
}

// NewDrewtechDevice creates a new Device
func NewDrewtechDevice(opts ...Option) *Device {
	d := &Device{
		parser:  NewParser(),
		pending: make(map[uint16]*pendingRequest),
		closeCh: make(chan struct{}),
		errCh:   make(chan error, 10),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Open opens the serial port and starts the read loop
func (d *Device) Open(portName string) error {
	mode := &serial.Mode{
		BaudRate: 115200,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return fmt.Errorf("open serial: %w", err)
	}

	d.port = port
	d.port.SetReadTimeout(50 * time.Millisecond)

	d.running.Store(true)
	d.wg.Add(1)
	go d.readLoop()

	return nil
}

// Close stops the read loop and closes the port
func (d *Device) Close() error {
	if !d.running.Load() {
		return nil
	}

	d.running.Store(false)
	close(d.closeCh)
	d.wg.Wait()

	if d.port != nil {
		return d.port.Close()
	}
	return nil
}

func (d *Device) readLoop() {
	defer d.wg.Done()

	buf := make([]byte, 1024)
	for d.running.Load() {
		n, err := d.port.Read(buf)
		if err != nil {
			if d.running.Load() {
				d.handleError(fmt.Errorf("read error: %w", err))
			}
			continue
		}
		if n == 0 {
			continue
		}

		packets, err := d.parser.Feed(buf[:n])
		if err != nil {
			d.handleError(err)
		}

		for _, pkt := range packets {
			d.handlePacket(pkt)
		}
	}
}

func (d *Device) handlePacket(pkt *Packet) {
	if pkt.Direction != DirectionIn {
		return
	}

	// Check if it's a CAN frame
	if pkt.IsCANRx() {
		frame, err := pkt.ParseCANFrame()
		if err != nil {
			d.handleError(err)
			return
		}

		if d.onCANFrame != nil {
			d.onCANFrame(frame)
		}
		if d.canRxCh != nil {
			select {
			case d.canRxCh <- frame:
			default:
				// Channel full, drop frame
			}
		}
		return
	}

	// Check if it's a response to a pending request
	seq := pkt.Sequence()
	d.mu.Lock()
	req, ok := d.pending[seq]
	if ok {
		delete(d.pending, seq)
	}
	d.mu.Unlock()

	if ok {
		select {
		case req.response <- pkt:
		default:
		}
	}
}

func (d *Device) handleError(err error) {
	if d.onError != nil {
		d.onError(err)
	}
	select {
	case d.errCh <- err:
	default:
	}
}

func (d *Device) nextSeq() uint16 {
	return uint16(atomic.AddUint32(&d.seq, 1))
}

func (d *Device) send(pkt *Packet) error {
	data := pkt.ToBytes()
	_, err := d.port.Write(data)
	return err
}

func (d *Device) sendAndWait(pkt *Packet, timeout time.Duration) (*Packet, error) {
	seq := binary.LittleEndian.Uint16(pkt.Payload[2:4])

	req := &pendingRequest{
		seq:      seq,
		response: make(chan *Packet, 1),
	}

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

// Protocol commands

func (d *Device) Init() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdInit
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], 0x0B48)

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: 0, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

func (d *Device) GetDeviceInfo(param uint16) error {
	payload := make([]byte, 8)
	payload[0] = SubCmdCANRx
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], param)

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: 0, Payload: payload}
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	log.Printf("% 02X", resp.Payload)

	info := ExtractVersions(resp.Payload)
	log.Printf("FW: %s", info.FW)
	log.Printf("BL: %s", info.BL)

	return err
}

func (d *Device) GetConfig(param uint32) (uint32, error) {
	payload := make([]byte, 12)
	payload[0] = SubCmdGetConfig
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], param)

	pkt := &Packet{Length: 0x10, Direction: DirectionOut, Channel: 0, Payload: payload}
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	if err != nil {
		return 0, err
	}

	if len(resp.Payload) >= 20 {
		return binary.LittleEndian.Uint32(resp.Payload[16:20]), nil
	}
	return 0, nil
}

func (d *Device) GetStatus() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdGetStatus
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: 0, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

func (d *Device) GetFirmwareVersion() (string, error) {
	payload := make([]byte, 8)
	payload[0] = SubCmdGetVersion
	payload[1] = FlagRequest
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: 0, Payload: payload}
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	if err != nil {
		return "", err
	}

	if len(resp.Payload) > 12 {
		// Find null terminator
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

func (d *Device) sendMagic() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdGetVersion
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], 0x6ADB)

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: 0, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

func (d *Device) GetSerial() (string, error) {
	payload := make([]byte, 12)
	payload[0] = SubCmdGetSerial
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())

	pkt := &Packet{Length: 0x10, Direction: DirectionOut, Channel: 0, Payload: payload}
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	if err != nil {
		return "", err
	}
	if len(resp.Payload) > 13 {
		return string(resp.Payload[resp.Payload[0]:len(resp.Payload)]), nil
	}
	return "", nil
}

// PassThruOpen performs the full device initialization sequence
func (d *Device) PassThruOpen() error {
	if err := d.Init(); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if err := d.GetDeviceInfo(0x003C); err != nil {
		return fmt.Errorf("get device info: %w", err)
	}
	if _, err := d.GetConfig(0x2F); err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if err := d.GetStatus(); err != nil {
		return fmt.Errorf("get status: %w", err)
	}
	if _, err := d.GetFirmwareVersion(); err != nil {
		return fmt.Errorf("get firmware: %w", err)
	}
	if err := d.sendMagic(); err != nil {
		return fmt.Errorf("send magic: %w", err)
	}
	if err := d.GetDeviceInfo(0xFFFF); err != nil {
		return fmt.Errorf("get device info ffff: %w", err)
	}
	if _, err := d.GetSerial(); err != nil {
		return fmt.Errorf("get serial: %w", err)
	}
	return nil
}

// PassThruConnect connects to CAN bus
func (d *Device) PassThruConnect(baudRate uint32) error {
	payload := make([]byte, 16)
	payload[0] = SubCmdConnect
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[12:16], baudRate)

	pkt := &Packet{Length: 0x14, Direction: DirectionOut, Channel: 5, Payload: payload}
	d.channel = 5

	if _, err := d.sendAndWait(pkt, 2*time.Second); err != nil {
		return err
	}

	// Set protocol
	payload2 := make([]byte, 20)
	payload2[0] = SubCmdSetProtocol
	payload2[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload2[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload2[8:12], 0x01)
	binary.LittleEndian.PutUint32(payload2[12:16], ProtocolCAN)
	binary.LittleEndian.PutUint32(payload2[16:20], 0x0E)

	pkt2 := &Packet{Length: 0x18, Direction: DirectionOut, Channel: d.channel, Payload: payload2}
	_, err := d.sendAndWait(pkt2, 2*time.Second)
	return err
}

// PassThruStartMsgFilter starts a pass-all filter
func (d *Device) PassThruStartMsgFilter() error {
	payload := make([]byte, 24)
	payload[0] = SubCmdStartMsgFilter
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	payload[14] = 0x01 // filter type
	payload[15] = 0x04 // flags

	pkt := &Packet{Length: 0x1C, Direction: DirectionOut, Channel: d.channel, Payload: payload}
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	if err != nil {
		return err
	}

	if len(resp.Payload) >= 16 {
		d.filterID = binary.LittleEndian.Uint32(resp.Payload[12:16])
	}
	return nil
}

// PassThruStopMsgFilter stops the current filter
func (d *Device) PassThruStopMsgFilter() error {
	payload := make([]byte, 12)
	payload[0] = SubCmdStopMsgFilter
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], uint16(d.filterID&0xFFFF))

	pkt := &Packet{Length: 0x10, Direction: DirectionOut, Channel: d.channel, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

// PassThruWriteMsgs transmits a CAN frame
func (d *Device) PassThruWriteMsgs(canID uint32, data []byte) error {
	if len(data) > 8 {
		data = data[:8]
	}

	payload := make([]byte, 24+len(data))
	payload[0] = SubCmdCANTx
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[4:6], 0x0001)
	binary.LittleEndian.PutUint16(payload[16:18], 4+uint16(len(data))) // data length
	binary.BigEndian.PutUint32(payload[20:24], canID)
	copy(payload[24:], data)

	pkt := &Packet{Length: 0x24, Direction: DirectionOut, Channel: d.channel, Payload: payload}
	return d.send(pkt)
}

// PassThruDisconnect disconnects from CAN bus
func (d *Device) PassThruDisconnect() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdDisconnect
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: d.channel, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

// PassThruClose closes the device connection
func (d *Device) PassThruClose() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdClose
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], 0x4C49)

	pkt := &Packet{Length: 0x0C, Direction: DirectionOut, Channel: 0, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

// GetChannelConfig gets a config parameter from the current channel
func (d *Device) GetChannelConfig(param uint32) (uint32, error) {
	payload := make([]byte, 12)
	payload[0] = SubCmdGetConfig
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], param)

	pkt := &Packet{Length: 0x10, Direction: DirectionOut, Channel: d.channel, Payload: payload}
	resp, err := d.sendAndWait(pkt, 2*time.Second)
	if err != nil {
		return 0, err
	}

	if len(resp.Payload) >= 20 {
		return binary.LittleEndian.Uint32(resp.Payload[16:20]), nil
	}
	return 0, nil
}

// SetChannelConfig sets a config parameter on the current channel
func (d *Device) SetChannelConfig(param, value uint32) error {
	payload := make([]byte, 16)
	payload[0] = SubCmdSetConfig
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], param)
	binary.LittleEndian.PutUint32(payload[12:16], value)

	pkt := &Packet{Length: 0x14, Direction: DirectionOut, Channel: d.channel, Payload: payload}
	_, err := d.sendAndWait(pkt, 2*time.Second)
	return err
}

// =========================

type Version struct {
	Major, Minor, Patch, Build byte
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.Major, v.Minor, v.Patch, v.Build)
}

type FirmwareInfo struct {
	FW Version
	BL Version
}

func ExtractVersions(data []byte) FirmwareInfo {
	const fwOffset = 28
	const blOffset = 32

	return FirmwareInfo{
		FW: Version{
			Major: data[fwOffset+3],
			Minor: data[fwOffset+2],
			Patch: data[fwOffset+1],
			Build: data[fwOffset],
		},
		BL: Version{
			Major: data[blOffset+3],
			Minor: data[blOffset+2],
			Patch: data[blOffset+1],
			Build: data[blOffset],
		},
	}
}
