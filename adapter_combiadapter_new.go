//go:build combi

package gocan

// CombiAdapterNew is a port of CombiAdapter onto github.com/gotmc/libusb.
// Protocol, constants, parser state machine and the ctrlWait/ helper types are
// shared with adapter_combiadapter.go (same package + build tag); only the USB
// transport differs. gotmc/libusb has no context-based cancellation, so
// transfers use millisecond timeouts and the recv loop polls closeChan/ctx
// between short reads.

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotmc/libusb/v2"
)

// Endpoint addresses (must be passed as untyped constants: gotmc's transfer
// methods take an unexported endpointAddress type that a byte var can't satisfy).
const (
	combiInEP  = 0x82 // IN  endpoint 2
	combiOutEP = 0x05 // OUT endpoint 5
	combiOutE2 = 0x02 // OUT endpoint 2 (STM32 clone fallback)
)

// cmdCanFilter is the firmware-side CAN acceptance-filter command added by the
// can-filter firmware patch (combi-firmware-1.1-canfilter.bin). Payload: up to
// 32 little-endian uint32 CAN IDs; empty payload disables the filter. The
// firmware drops non-matching frames before they cross USB.
const (
	cmdCanFilter   = 0x84
	maxHWFilterIDs = 32
)

const libusbErrTimeout = libusb.ErrorCode(-7) // LIBUSB_ERROR_TIMEOUT

type CombiAdapterNew struct {
	*BaseAdapter

	usbCtx *libusb.Context
	dev    *libusb.Device
	handle *libusb.DeviceHandle
	useEP2 bool // device exposes OUT on EP2 instead of EP5

	closeOnce sync.Once
	txPool    sync.Pool

	ctrlMu   sync.Mutex
	ctrlWait *ctrlWait

	adcFilterChan   chan bool
	adcValueChan    chan float32
	thermoValueChan chan float32

	// txAck receives the firmware's per-frame cmdCanTxFrame ack. The .NET
	// driver waits for this before sending the next command; skipping it lets
	// us outrun the single-threaded firmware and lose responses.
	txAck chan struct{}

	// filter is the software CAN acceptance filter. The combi firmware has no
	// hardware filter, so without this it relays the whole bus over USB and
	// floods recvChan when the engine is running. nil => pass everything.
	filter atomic.Pointer[map[uint32]struct{}]

	dropCount uint64
}

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "CombiAdapterNew",
		Description:        "gotmc/libusb driver",
		RequiresSerialPort: false,
		Capabilities:       AdapterCapabilities{HSCAN: true},
		New:                NewCombiNew,
	}); err != nil {
		panic(err)
	}
}

func NewCombiNew(cfg *AdapterConfig) (Adapter, error) {
	ca := &CombiAdapterNew{
		BaseAdapter: NewBaseAdapter("CombiAdapterNew", cfg),
		txPool: sync.Pool{New: func() any {
			// cmd, size(2), ID(4), data(8), len, ext, rtr, term
			return []byte{cmdCanTxFrame, 0x00, 0x0F, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		}},
		adcFilterChan:   make(chan bool, 1),
		adcValueChan:    make(chan float32),
		thermoValueChan: make(chan float32, 1),
		txAck:           make(chan struct{}, 1),
	}
	ca.syncCapable = true // sendCANMessage blocks on the firmware TX ack, so write-completion is real
	return ca, nil
}

// ============
// Public API
// ============

func (ca *CombiAdapterNew) SetFilter(ids []uint32) error {
	if len(ids) == 0 {
		ca.filter.Store(nil)
		return nil
	}
	m := make(map[uint32]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	ca.filter.Store(&m)
	return nil
}

// SetHardwareFilter installs a CAN acceptance filter in the adapter firmware
// (command 0x84; requires combi-firmware-1.2). Frames whose ID is
// not listed are dropped on the adapter and never sent over USB, which is the
// point of the filter: it stops the firmware flooding recvChan with the whole
// bus. Passing no ids disables the filter (pass all). Up to 32 ids.
//
// This is independent of SetFilter (the host-side software filter); use either
// or both. Send it after the CAN channel is open.
func (ca *CombiAdapterNew) SetHardwareFilter(ctx context.Context, ids []uint32) error {
	if len(ids) > maxHWFilterIDs {
		return fmt.Errorf("too many filter ids: %d (firmware max %d)", len(ids), maxHWFilterIDs)
	}
	payload := make([]byte, 4*len(ids))
	for i, id := range ids {
		binary.LittleEndian.PutUint32(payload[4*i:], id)
	}

	log.Printf("SetFilters: %02X", ids)

	// 200ms: on a busy bus the ACK is queued behind relayed frames in the USB stream.
	return ca.sendCommand(ctx, cmdCanFilter, payload, 200)
}

func (ca *CombiAdapterNew) Open(ctx context.Context) error {
	usbCtx, err := libusb.NewContext()
	if err != nil {
		return fmt.Errorf("libusb init: %w", err)
	}
	dev, handle, err := usbCtx.OpenDeviceWithVendorProduct(uint16(combiVid), uint16(combiPid))
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
		ca.Info("using EP2 (stm32 clone)")
	}

	// The firmware can't filter, so apply the configured acceptance filter in
	// software to avoid relaying the whole bus over USB.
	_ = ca.SetFilter(ca.cfg.CANFilter)

	// Known state & drain stale bytes
	if err := ca.canCtrl(0); err != nil {
		ca.closeUSB()
		return fmt.Errorf("failed to close canbus: %w", err)
	}
	ca.drainInput(150 * time.Millisecond)

	if ca.cfg.PrintVersion {
		if ver, err := ca.ReadVersion(ctx); err == nil {
			ca.Info(ver)
		}
	}

	if ca.cfg.AdditionalConfig["NoConnect"] != "true" {
		if err := ca.setBitrate(); err != nil {
			ca.closeUSB()
			return err
		}
		if err := ca.canCtrl(1); err != nil {
			ca.closeUSB()
			return fmt.Errorf("failed to open canbus: %w", err)
		}
	}

	go ca.recvManager(ctx)
	go ca.sendManager(ctx)

	// Apply the firmware acceptance filter only AFTER recvManager is running:
	// sendCommand waits for the 0x84 ACK to be delivered by recvManager, so if we
	// send it earlier nothing reads the ACK off USB and it times out.
	if ca.cfg.AdditionalConfig["NoConnect"] != "true" {
		if err := ca.SetHardwareFilter(ctx, ca.cfg.CANFilter); err != nil {
			log.Printf("Error setting can filter: %v", err)
		}
	}
	return nil
}

func (ca *CombiAdapterNew) Close() error {
	ca.BaseAdapter.Close()
	var err error
	ca.closeOnce.Do(func() { err = ca.closeAdapter() })
	return err
}

// ReadVersion writes the FW-version command and reads the reply directly.
// ponytail: intended for use during Open before recvManager starts; calling it
// while recvManager runs races for the reply (same as the original driver).
func (ca *CombiAdapterNew) ReadVersion(_ context.Context) (string, error) {
	if _, err := ca.bulkOut([]byte{cmdBrdFWVersion, 0x00, 0x00, termAck}, 200); err != nil {
		return "", err
	}
	buf := make([]byte, 64)
	if _, err := ca.bulkIn(buf, 200); err != nil {
		return "", err
	}
	return fmt.Sprintf("CombiAdapter v%d.%d", buf[4], buf[3]), nil
}

func (ca *CombiAdapterNew) GetADCFiltering(ctx context.Context, channel int) (bool, error) {
	if err := ca.sendCommand(ctx, cmdBrdADCFilter, []byte{byte(channel)}, 10); err != nil {
		return false, err
	}
	select {
	case b := <-ca.adcFilterChan:
		return b, nil
	case <-time.After(5 * time.Millisecond):
		return false, fmt.Errorf("timeout waiting for ADC filter response")
	}
}

func (ca *CombiAdapterNew) SetADCFiltering(ctx context.Context, channel int, enabled bool) error {
	var en byte
	if enabled {
		en = 0x01
	}
	return ca.sendCommand(ctx, cmdBrdADCFilter, []byte{byte(channel), en}, 10)
}

func (ca *CombiAdapterNew) GetADCValue(ctx context.Context, channel int) (float64, error) {
	if err := ca.sendCommand(ctx, cmdBrdADC, []byte{byte(channel)}, 10); err != nil {
		return 0, fmt.Errorf("failed to send ADC value request: %w", err)
	}
	select {
	case f := <-ca.adcValueChan:
		return float64(f), nil
	case <-time.After(200 * time.Millisecond):
		return 0, fmt.Errorf("timeout waiting for ADC value response")
	}
}

func (ca *CombiAdapterNew) GetThermoValue(_ context.Context) (float32, error) {
	ca.sendChan <- &CANFrame{Identifier: SystemMsg, Data: []byte{cmdBrdEGT, 0x00, 0x00, 0x00}}
	select {
	case f := <-ca.thermoValueChan:
		return f, nil
	case <-time.After(5 * time.Millisecond):
		return 0, fmt.Errorf("timeout waiting for thermo value response")
	}
}

func (ca *CombiAdapterNew) DroppedFrames() uint64 { return atomic.LoadUint64(&ca.dropCount) }

// =====================
// USB transport (gotmc/libusb)
// =====================

func (ca *CombiAdapterNew) bulkOut(data []byte, timeoutMs int) (int, error) {
	if ca.useEP2 {
		return ca.handle.BulkTransferOut(combiOutE2, data, timeoutMs)
	}
	return ca.handle.BulkTransferOut(combiOutEP, data, timeoutMs)
}

func (ca *CombiAdapterNew) bulkIn(buf []byte, timeoutMs int) (int, error) {
	return ca.handle.BulkTransfer(combiInEP, buf, len(buf), timeoutMs)
}

func isUSBTimeout(err error) bool {
	ec, ok := err.(libusb.ErrorCode)
	return ok && ec == libusbErrTimeout
}

// hasOutEP5 reports whether interface usbInterfaceNum exposes OUT endpoint 5.
func (ca *CombiAdapterNew) hasOutEP5() bool {
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

func (ca *CombiAdapterNew) closeAdapter() error {
	_ = ca.canCtrl(0) // best-effort close CAN
	time.Sleep(20 * time.Millisecond)
	ca.closeUSB()
	return nil
}

func (ca *CombiAdapterNew) closeUSB() {
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

// =====================
// Control plane
// =====================

func (ca *CombiAdapterNew) sendCommand(ctx context.Context, cmd byte, data []byte, timeoutMs int) error {
	if len(data) > MaxCommandSize {
		return fmt.Errorf("%w: %d", ErrCommandSizeTooLarge, len(data))
	}
	w := &ctrlWait{cmd: cmd, ch: make(chan []byte, 1)}

	ca.ctrlMu.Lock()
	if ca.ctrlWait != nil {
		busy := ca.ctrlWait.cmd
		ca.ctrlMu.Unlock()
		return fmt.Errorf("control pipe busy, cmd %02X in-flight", busy)
	}
	ca.ctrlWait = w
	ca.ctrlMu.Unlock()

	plen := len(data)
	pkt := make([]byte, 3+plen+1)
	pkt[0] = cmd
	pkt[1] = byte(plen >> 8)
	pkt[2] = byte(plen)
	copy(pkt[3:], data)
	pkt[3+plen] = termAck

	t := timeoutMs
	if t == 0 {
		t = 250
	}
	if _, err := ca.bulkOut(pkt, t); err != nil {
		ca.clearCtrlWait()
		return fmt.Errorf("send cmd %02X failed: %w", cmd, err)
	}

	select {
	case <-ctx.Done():
		ca.clearCtrlWait()
		return ctx.Err()
	case <-time.After(time.Duration(t) * time.Millisecond):
		ca.clearCtrlWait()
		return fmt.Errorf("timeout waiting for cmd %02X response", cmd)
	case <-w.ch:
		return nil
	}
}

func (ca *CombiAdapterNew) clearCtrlWait() {
	ca.ctrlMu.Lock()
	ca.ctrlWait = nil
	ca.ctrlMu.Unlock()
}

func (ca *CombiAdapterNew) handleControlCommand(cmd byte, data []byte) error {
	ca.ctrlMu.Lock()
	w := ca.ctrlWait
	if w != nil && w.cmd == cmd {
		ca.ctrlWait = nil
		select { // non-blocking (avoid deadlock on close)
		case w.ch <- append([]byte(nil), data...):
		default:
		}
		ca.ctrlMu.Unlock()
		return nil
	}
	ca.ctrlMu.Unlock()
	if ca.cfg.Debug {
		log.Printf("control %02X with no waiter, len=%d", cmd, len(data))
	}
	return nil
}

// =====================
// Data plane
// =====================

func (ca *CombiAdapterNew) sendManager(ctx context.Context) {
	if ca.cfg.Debug {
		defer log.Println("sendManager exited")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ca.closeChan:
			return
		case frame := <-ca.sendChan:
			if frame.Identifier == SystemMsg {
				if _, err := ca.bulkOut(frame.Data, 250); err != nil {
					ca.Error(fmt.Errorf("failed to send frame: %w", err))
				}
				continue
			}
			ca.sendCANMessage(frame)
		}
	}
}

func (ca *CombiAdapterNew) sendCANMessage(frame *CANFrame) {
	buf := ca.txPool.Get().([]byte)
	defer ca.txPool.Put(buf)
	defer frame.markSent() // release a SendSync waiter once this frame is written + acked (or on any early return)
	dlc := frame.DLC()
	binary.LittleEndian.PutUint32(buf[3:], frame.Identifier)
	copy(buf[7:], frame.Data[:min(dlc, 8)])
	buf[15] = uint8(dlc)
	buf[16] = boolToByte(frame.Extended)
	buf[17] = boolToByte(frame.RTR)
	// buf[18] is the pre-set terminator (0)

	// Drop any stale ack so we wait for *this* frame's ack.
	select {
	case <-ca.txAck:
	default:
	}

	n, err := ca.bulkOut(buf, 250)
	if err != nil {
		ca.Fatal(fmt.Errorf("failed to send frame: %w", err))
		return
	}
	if n != 19 {
		ca.Error(fmt.Errorf("sent %d bytes of data out of 19", n))
	}

	// Wait for the firmware to confirm the frame before letting the next
	// command out (mirrors the .NET driver). sendManager is single-threaded,
	// so only one TX is ever in flight.
	select {
	case <-ca.txAck:
	case <-time.After(100 * time.Millisecond):
		ca.Error(fmt.Errorf("timeout waiting for TX ack"))
	}
}

func (ca *CombiAdapterNew) recvManager(ctx context.Context) {
	if ca.cfg.Debug {
		defer log.Println("recvManager exited")
	}

	var (
		readBuf [MaxCommandSize]byte
		state   = psCmd
		cmd     byte
		size    uint16
		ptr     uint16
		cdata   [MaxCommandSize]byte
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ca.closeChan:
			return
		default:
			n, err := ca.bulkIn(readBuf[:], 100)
			if err != nil {
				if isUSBTimeout(err) {
					continue // idle, no data
				}
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ca.closeChan:
					return
				default:
				}
				ca.Error(fmt.Errorf("failed to read from usb device: %w", err))
				continue
			}
			for _, b := range readBuf[:n] {
				switch state {
				case psCmd:
					if _, ok := combiValidCommands[b]; !ok && b != cmdCanFilter {
						ca.Error(fmt.Errorf("%w: %02X", ErrInvalidCommand, b))
						state = psCmd
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
					if size >= MaxCommandSize {
						ca.Error(fmt.Errorf("command size too large: %d", size))
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
					if b == termNak {
						ca.Error(fmt.Errorf("%w: %02X", ErrCommandTermination, b))
						state = psCmd
						continue
					} else if b != termAck {
						ca.Error(fmt.Errorf("unexpected termination byte: %02X, expected: %02X", b, termAck))
						state = psCmd
						continue
					}

					// Device quirk: ADCFilter size==1 => flag in data[3]
					if cmd == cmdBrdADCFilter && size == 1 {
						cdata[3] = 0x01
					}

					if ca.cfg.Debug {
						pkg := []byte{cmd, byte(size >> 8), byte(size)}
						pkg = append(pkg, cdata[:ptr]...)
						log.Printf("recv cmd: % 02X", pkg)
					}
					if err := ca.processCommand(cmd, cdata[:ptr]); err != nil {
						ca.Error(err)
					}
					state = psCmd
				}
			}
		}
	}
}

func (ca *CombiAdapterNew) processCommand(cmd byte, data []byte) error {
	switch cmd {
	case cmdBrdADCFilter:
		switch len(data) {
		case 1:
			if data[0] == 0xFF {
				log.Printf("ADC filter err: %X", data)
			}
		case 2:
			if data[0] == 0x01 {
				ca.adcFilterChan <- true
			} else {
				ca.adcFilterChan <- false
			}
		case 3:
			log.Printf("ADC filter: %X", data)
		default:
			log.Println("invalid ADC filter size", len(data))
		}
		return ca.handleControlCommand(cmd, data)

	case cmdBrdEGT:
		select {
		case ca.thermoValueChan <- math.Float32frombits(binary.LittleEndian.Uint32(data[1:5])):
		default:
			log.Println("thermoValueChan full, dropping value")
		}
		return ca.handleControlCommand(cmd, data)

	case cmdBrdADC:
		select {
		case ca.adcValueChan <- math.Float32frombits(binary.LittleEndian.Uint32(data[0:4])):
		default:
			log.Println("adcValueChan full, dropping value")
		}
		return ca.handleControlCommand(cmd, data)

	case cmdCanTxFrame:
		select {
		case ca.txAck <- struct{}{}:
		default:
		}
		return nil

	case cmdBrdFWVersion, cmdCanOpen, cmdCanBitrate, cmdCanFilter:
		return ca.handleControlCommand(cmd, data)

	case cmdCanFrame:
		return ca.handleCANFrame(data)
	}
	return fmt.Errorf("%w: %02X", ErrInvalidCommand, cmd)
}

func (ca *CombiAdapterNew) handleCANFrame(data []byte) error {
	if len(data) != 15 {
		return fmt.Errorf("invalid CAN frame size: %d", len(data))
	}
	id := binary.LittleEndian.Uint32(data[:4])
	if f := ca.filter.Load(); f != nil {
		if _, ok := (*f)[id]; !ok {
			return nil // not in acceptance filter, drop early
		}
	}
	frame := NewFrame(
		id,
		data[4:4+data[12]],
		Incoming,
	)
	frame.Extended = data[13] == 1
	frame.RTR = data[14] == 1

	select {
	case ca.recvChan <- frame:
		return nil
	default:
		atomic.AddUint64(&ca.dropCount, 1)
		return ErrDroppedFrame
	}
}

// =====================
// Helpers
// =====================

func (ca *CombiAdapterNew) drainInput(max time.Duration) {
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

func (ca *CombiAdapterNew) canCtrl(mode byte) error {
	if _, err := ca.bulkOut([]byte{cmdCanOpen, 0x00, 0x01, mode, termAck}, 200); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

func (ca *CombiAdapterNew) setBitrate() error {
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
