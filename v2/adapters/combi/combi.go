// Package combi drives the CombiAdapter over USB via gotmc/libusb (cgo;
// requires libusb-1.0). Importing the package registers the "CombiAdapter"
// adapter. It is kept out of adapters/all so that package stays pure Go —
// import this package directly.
//
// Wire protocol: packets are cmd, size (u16 BE), payload, terminator
// (0x00 ack / 0xFF nak) in both directions. The firmware acks every TX frame
// (cmdCanTxFrame back) and is single-threaded, so Send waits for the ack
// before returning; the Bus serializes Send callers.
//
// Firmware 1.2 added a CAN acceptance filter (cmdCanFilter): up to 32
// little-endian uint32 IDs, dropped on the adapter before they cross USB.
// Open reads the firmware version and only installs the filter on >= 1.2;
// older firmware relays the whole bus and the Bus's subscription dispatch
// does the filtering host-side.
package combi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gotmc/libusb/v2"
	gocan "github.com/roffe/gocan/v2"
)

const (
	combiVid        = 0xFFFF
	combiPid        = 0x0005
	usbInterfaceNum = 1

	// Endpoint addresses (untyped constants: gotmc's transfer methods take an
	// unexported endpointAddress type that a byte var can't satisfy).
	combiInEP  = 0x82 // IN  endpoint 2
	combiOutEP = 0x05 // OUT endpoint 5
	combiOutE2 = 0x02 // OUT endpoint 2 (STM32 clone fallback)

	termAck = 0x00
	termNak = 0xFF

	cmdBrdFWVersion = 0x20 // firmware version
	cmdBrdADCFilter = 0x21 // ADC filter settings
	cmdBrdADC       = 0x22 // ADC value (float32 LE)
	cmdBrdEGT       = 0x23 // EGT value (float32 LE)
	cmdCanOpen      = 0x80 // open/close CAN channel (size=1, 0/1)
	cmdCanBitrate   = 0x81 // set bitrate (size=4, u32)
	cmdCanFrame     = 0x82 // incoming frame (15 bytes payload)
	cmdCanTxFrame   = 0x83 // outgoing frame (15 bytes payload)
	cmdCanFilter    = 0x84 // acceptance filter (firmware >= 1.2)

	maxHWFilterIDs = 32
	maxCommandSize = 1024
)

const libusbErrTimeout = libusb.ErrorCode(-7) // LIBUSB_ERROR_TIMEOUT

var validCommands = map[byte]struct{}{
	cmdBrdFWVersion: {},
	cmdBrdADCFilter: {},
	cmdBrdADC:       {},
	cmdBrdEGT:       {},
	cmdCanOpen:      {},
	cmdCanBitrate:   {},
	cmdCanFrame:     {},
	cmdCanTxFrame:   {},
	cmdCanFilter:    {},
}

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:         "CombiAdapter",
		Description:  "CombiAdapter (gotmc/libusb)",
		Capabilities: gocan.Capabilities{HSCAN: true},
		New:          New,
	})
}

type Combi struct {
	cfg gocan.Config
	bus *gocan.Bus

	usbCtx *libusb.Context
	dev    *libusb.Device
	handle *libusb.DeviceHandle
	useEP2 bool // device exposes OUT on EP2 instead of EP5

	closeOnce sync.Once

	txAck     chan struct{} // firmware per-frame cmdCanTxFrame ack
	filterAck chan struct{}
	adcValue  chan float32 // firmware cmdCanFilter ack
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &Combi{
		cfg:       cfg,
		txAck:     make(chan struct{}, 1),
		filterAck: make(chan struct{}, 1),
		adcValue:  make(chan float32, 1),
	}, nil
}

func (ca *Combi) Open(ctx context.Context, bus *gocan.Bus) error {
	ca.bus = bus
	usbCtx, err := libusb.NewContext()
	if err != nil {
		return fmt.Errorf("libusb init: %w", err)
	}
	dev, handle, err := usbCtx.OpenDeviceWithVendorProduct(combiVid, combiPid)
	if err != nil {
		usbCtx.Close()
		return fmt.Errorf("CombiAdapter not found: %w", err)
	}
	_ = handle.SetAutoDetachKernelDriver(true) // no-op on Windows, helpful on Linux
	if err := handle.ClaimInterface(usbInterfaceNum); err != nil {
		handle.Close()
		dev.Close()
		usbCtx.Close()
		return fmt.Errorf("claim interface %d: %w", usbInterfaceNum, err)
	}
	ca.usbCtx, ca.dev, ca.handle = usbCtx, dev, handle
	ca.useEP2 = !ca.hasOutEP5()
	if ca.useEP2 {
		ca.emit(gocan.EventTypeInfo, "using EP2 (stm32 clone)")
	}

	// Known state & drain stale bytes.
	if err := ca.canCtrl(0); err != nil {
		ca.closeUSB()
		return fmt.Errorf("failed to close canbus: %w", err)
	}
	ca.drainInput(150 * time.Millisecond)

	// Read the firmware version before the read loop starts (it would race
	// for the reply). It gates the hardware filter below.
	major, minor, verErr := ca.readVersion()
	if verErr != nil {
		ca.emit(gocan.EventTypeWarning, fmt.Sprintf("failed to read firmware version: %v", verErr))
	} else {
		ca.emit(gocan.EventTypeInfo, fmt.Sprintf("CombiAdapter v%d.%d", major, minor))
	}

	if err := ca.setBitrate(); err != nil {
		ca.closeUSB()
		return err
	}
	if err := ca.canCtrl(1); err != nil {
		ca.closeUSB()
		return fmt.Errorf("failed to open canbus: %w", err)
	}

	go ca.readLoop(ctx)

	// Install the firmware acceptance filter only AFTER the read loop is
	// running (it delivers the ack) and only on firmware >= 1.2; older
	// firmware has no cmdCanFilter and the Bus filters host-side.
	if len(ca.cfg.CANFilter) > 0 && verErr == nil && (major > 1 || (major == 1 && minor >= 2)) {
		if err := ca.setHardwareFilter(ca.cfg.CANFilter); err != nil {
			ca.emit(gocan.EventTypeWarning, fmt.Sprintf("failed to set CAN filter: %v", err))
		}
	}
	return nil
}

func (ca *Combi) Close() error {
	ca.closeOnce.Do(func() {
		_ = ca.canCtrl(0) // best-effort close CAN
		time.Sleep(20 * time.Millisecond)
		ca.closeUSB()
	})
	return nil
}

// Send writes one frame and waits for the firmware's per-frame ack (the
// firmware is single-threaded; outrunning it loses responses). The Bus never
// calls Send concurrently.
func (ca *Combi) Send(ctx context.Context, f gocan.Frame) error {
	// cmd, size(2), ID(4), data(8), len, ext, rtr, term
	buf := [19]byte{cmdCanTxFrame, 0x00, 0x0F}
	binary.LittleEndian.PutUint32(buf[3:], f.ID)
	copy(buf[7:15], f.Data[:])
	buf[15] = min(f.Length, 8)
	if f.Extended {
		buf[16] = 1
	}
	if f.Remote {
		buf[17] = 1
	}

	// Drop any stale ack so we wait for *this* frame's ack.
	select {
	case <-ca.txAck:
	default:
	}

	if _, err := ca.bulkOut(buf[:], 250); err != nil {
		err = fmt.Errorf("failed to send frame: %w", err)
		ca.bus.Fatal(err)
		return err
	}

	if ca.cfg.Debug {
		ca.emit(gocan.EventTypeDebug, fmt.Sprintf("sent frame: %s", f.String()))
	}

	select {
	case <-ca.txAck:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(100 * time.Millisecond):
		return errors.New("timeout waiting for TX ack")
	}
}

// setHardwareFilter installs the firmware CAN acceptance filter (up to 32
// IDs; empty payload disables it). Requires the read loop for the ack.
func (ca *Combi) setHardwareFilter(ids []uint32) error {
	if len(ids) > maxHWFilterIDs {
		return fmt.Errorf("too many filter ids: %d (firmware max %d)", len(ids), maxHWFilterIDs)
	}
	plen := 4 * len(ids)
	pkt := make([]byte, 3+plen+1)
	pkt[0] = cmdCanFilter
	pkt[1] = byte(plen >> 8)
	pkt[2] = byte(plen)
	for i, id := range ids {
		binary.LittleEndian.PutUint32(pkt[3+4*i:], id)
	}
	pkt[3+plen] = termAck
	if _, err := ca.bulkOut(pkt, 200); err != nil {
		return err
	}
	select {
	// 200ms: on a busy bus the ack queues behind relayed frames.
	case <-ca.filterAck:
		return nil
	case <-time.After(200 * time.Millisecond):
		return errors.New("timeout waiting for filter ack")
	}
}

// GetADCValue reads one of the board's analog inputs (float32 reply).
// Requires the read loop for the reply.
func (ca *Combi) GetADCValue(ctx context.Context, channel int) (float64, error) {
	select {
	case <-ca.adcValue: // drop a stale value
	default:
	}
	pkt := []byte{cmdBrdADC, 0x00, 0x01, byte(channel), termAck}
	if _, err := ca.bulkOut(pkt, 200); err != nil {
		return 0, fmt.Errorf("failed to send ADC value request: %w", err)
	}
	select {
	case v := <-ca.adcValue:
		return float64(v), nil
	case <-ctx.Done():
		return 0, context.Cause(ctx)
	case <-time.After(200 * time.Millisecond):
		return 0, errors.New("timeout waiting for ADC value response")
	}
}

// =====================
// USB transport
// =====================

func (ca *Combi) bulkOut(data []byte, timeoutMs int) (int, error) {
	if ca.useEP2 {
		return ca.handle.BulkTransferOut(combiOutE2, data, timeoutMs)
	}
	return ca.handle.BulkTransferOut(combiOutEP, data, timeoutMs)
}

func (ca *Combi) bulkIn(buf []byte, timeoutMs int) (int, error) {
	return ca.handle.BulkTransfer(combiInEP, buf, len(buf), timeoutMs)
}

func isUSBTimeout(err error) bool {
	ec, ok := err.(libusb.ErrorCode)
	return ok && ec == libusbErrTimeout
}

// hasOutEP5 reports whether interface usbInterfaceNum exposes OUT endpoint 5.
func (ca *Combi) hasOutEP5() bool {
	cfg, err := ca.dev.ActiveConfigDescriptor()
	if err != nil {
		return true // assume standard layout
	}
	for _, si := range cfg.SupportedInterfaces {
		for _, id := range si.InterfaceDescriptors {
			if id.InterfaceNumber != usbInterfaceNum {
				continue
			}
			for _, ep := range id.EndpointDescriptors {
				if byte(ep.EndpointAddress) == combiOutEP {
					return true
				}
			}
		}
	}
	return false
}

func (ca *Combi) closeUSB() {
	if ca.handle != nil {
		_ = ca.handle.ReleaseInterface(usbInterfaceNum)
		_ = ca.handle.Close()
	}
	if ca.dev != nil {
		ca.dev.Close()
	}
	if ca.usbCtx != nil {
		_ = ca.usbCtx.Close()
	}
}

func (ca *Combi) drainInput(max time.Duration) {
	deadline := time.Now().Add(max)
	tmp := make([]byte, 512)
	for time.Now().Before(deadline) {
		n, err := ca.bulkIn(tmp, 10)
		if err != nil {
			if isUSBTimeout(err) {
				return // input quiet
			}
			continue
		}
		if n == 0 {
			return
		}
	}
}

// readVersion asks for the firmware version and reads the reply directly;
// only usable before the read loop starts (it would race for the reply).
func (ca *Combi) readVersion() (major, minor byte, err error) {
	if _, err := ca.bulkOut([]byte{cmdBrdFWVersion, 0x00, 0x00, termAck}, 200); err != nil {
		return 0, 0, err
	}
	buf := make([]byte, 64)
	if _, err := ca.bulkIn(buf, 200); err != nil {
		return 0, 0, err
	}
	// reply: cmd, size(2), minor, major, term
	return buf[4], buf[3], nil
}

func (ca *Combi) canCtrl(mode byte) error {
	if _, err := ca.bulkOut([]byte{cmdCanOpen, 0x00, 0x01, mode, termAck}, 200); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

func (ca *Combi) setBitrate() error {
	var rate uint32
	switch ca.cfg.CANRate {
	case 615.384:
		rate = 615000
	default:
		rate = uint32(ca.cfg.CANRate * 1000)
	}
	pkt := []byte{cmdCanBitrate, 0x00, 0x04, byte(rate >> 24), byte(rate >> 16), byte(rate >> 8), byte(rate), termAck}
	if _, err := ca.bulkOut(pkt, 40); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

// =====================
// Receive path
// =====================

// Parser states for the incoming packet state machine.
const (
	psCmd = iota
	psSizeHigh
	psSizeLow
	psData
	psTerm
)

func (ca *Combi) readLoop(ctx context.Context) {
	var (
		readBuf [maxCommandSize]byte
		state   = psCmd
		cmd     byte
		size    uint16
		ptr     uint16
		cdata   [maxCommandSize]byte
	)

	for ctx.Err() == nil {
		n, err := ca.bulkIn(readBuf[:], 100)
		if err != nil {
			if isUSBTimeout(err) {
				continue // idle, no data
			}
			if ctx.Err() == nil { // dead device, not a shutdown
				ca.bus.Fatal(fmt.Errorf("failed to read from usb device: %w", err))
			}
			return
		}
		for _, b := range readBuf[:n] {
			switch state {
			case psCmd:
				if _, ok := validCommands[b]; !ok {
					ca.errorf("invalid command: %02X", b)
					continue
				}
				cmd = b
				ptr, size = 0, 0
				state = psSizeHigh
			case psSizeHigh:
				size = uint16(b) << 8
				state = psSizeLow
			case psSizeLow:
				size |= uint16(b)
				if size >= maxCommandSize {
					ca.errorf("command size too large: %d", size)
					state = psCmd
					continue
				}
				if size == 0 {
					state = psTerm
				} else {
					state = psData
				}
			case psData:
				cdata[ptr] = b
				ptr++
				if ptr >= size {
					state = psTerm
				}
			case psTerm:
				state = psCmd
				if b == termNak {
					ca.errorf("command %02X failed (NAK)", cmd)
					continue
				} else if b != termAck {
					ca.errorf("unexpected termination byte: %02X, expected: %02X", b, termAck)
					continue
				}
				if ca.cfg.Debug {
					ca.emit(gocan.EventTypeDebug, fmt.Sprintf("recv cmd %02X: % 02X", cmd, cdata[:ptr]))
				}
				ca.processCommand(cmd, cdata[:ptr])
			}
		}
	}
}

func (ca *Combi) processCommand(cmd byte, data []byte) {
	switch cmd {
	case cmdCanFrame:
		ca.deliverFrame(data)
	case cmdCanTxFrame:
		select {
		case ca.txAck <- struct{}{}:
		default:
		}
	case cmdCanFilter:
		select {
		case ca.filterAck <- struct{}{}:
		default:
		}
	case cmdBrdADC:
		if len(data) == 4 {
			v := math.Float32frombits(binary.LittleEndian.Uint32(data))
			select {
			case ca.adcValue <- v:
			default:
			}
		}
	default:
		// Board replies (FW version, ADC, EGT, open, bitrate) with no waiter.
	}
}

// deliverFrame decodes the 15-byte cmdCanFrame payload:
// ID (u32 LE), data (8), DLC, extended, remote.
func (ca *Combi) deliverFrame(data []byte) {
	if len(data) != 15 {
		ca.errorf("invalid CAN frame size: %d", len(data))
		return
	}
	f := gocan.Frame{
		ID:       binary.LittleEndian.Uint32(data[:4]),
		Length:   min(data[12], 8),
		Extended: data[13] == 1,
		Remote:   data[14] == 1,
	}
	copy(f.Data[:], data[4:12])
	if ca.cfg.Debug {
		ca.emit(gocan.EventTypeDebug, fmt.Sprintf("recv frame: %s", f.String()))
	}
	ca.bus.Deliver(f)
}

// =====================
// Events
// =====================

func (ca *Combi) emit(t gocan.EventType, details string) {
	ca.bus.Emit(gocan.Event{Type: t, Details: details})
}

func (ca *Combi) errorf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	ca.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
}
