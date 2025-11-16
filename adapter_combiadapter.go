//go:build combi

package gocan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gousb"
)

// ==========================
// Device constants & protocol
// ==========================

const (
	combiVid = 0xFFFF
	combiPid = 0x0005
)

// USB topology
const (
	usbConfigNumber   = 1
	usbInterfaceNum   = 1
	usbAltSetting     = 0
	usbInEndpointNum  = 2
	usbOutEndpointNum = 5 // fall back to 2 for STM32 clone
)

// Limits
const (
	MaxCommandSize = 1024
)

// Command terminators
const (
	termAck = 0x00 // command acknowledged
	termNak = 0xFF // command failed
)

// Board & CAN commands (as observed on the wire)
const (
	cmdBrdFWVersion = 0x20 // firmware version
	cmdBrdADCFilter = 0x21 // ADC filter settings
	cmdBrdADC       = 0x22 // ADC value (float32 LE)
	cmdBrdEGT       = 0x23 // EGT value (float32 LE)

	cmdBDMStop        = 0x40
	cmdBDMReset       = 0x41
	cmdBDMRun         = 0x42
	cmdBDMStep        = 0x43
	cmdBDMRestart     = 0x44
	cmdBDMReadMem     = 0x45
	cmdBDMWriteMem    = 0x46
	cmdBDMReadSysReg  = 0x47
	cmdBDMWriteSysReg = 0x48
	cmdBDMReadADReg   = 0x49
	cmdBDMWriteADReg  = 0x4a
	cmdBDMReadFlash   = 0x4b
	cmdBDMEraseFlash  = 0x4c
	cmdBDMWriteFlash  = 0x4d
	cmdBDMPinState    = 0x4e

	cmdCanOpen       = 0x80 // open/close CAN channel (size=1, 0/1)
	cmdCanBitrate    = 0x81 // set bitrate (size=4, u32)
	cmdCanFrame      = 0x82 // incoming frame (15 bytes payload)
	cmdCanTxFrame    = 0x83 // outgoing frame (15 bytes payload)
	cmdCanECUConnect = 0x89
	cmdCanReadFlash  = 0x8a
	cmdCanWriteFlash = 0x8b

	cmdMWReadAll  = 0xa0
	cmdMWWriteAll = 0xa1
	cmdMWEraseAll = 0xa2
)

// Parser steps
const (
	psCmd = iota
	psSizeHigh
	psSizeLow
	psData
	psTerm
)

// ==============
// Errors & guards
// ==============

var (
	ErrInvalidCommand      = errors.New("invalid command")
	ErrCommandSizeTooLarge = errors.New("command size too large")
	ErrCommandTermination  = errors.New("command terminated with NAK")
)

// Valid commands we expect from the device (for basic sanity checking)
var combiValidCommands = map[byte]struct{}{
	cmdCanOpen:      {},
	cmdCanBitrate:   {},
	cmdCanFrame:     {},
	cmdCanTxFrame:   {},
	cmdBrdFWVersion: {},
	cmdBrdADCFilter: {},
	cmdBrdADC:       {},
	cmdBrdEGT:       {},
}

// =====================
// Adapter implementation
// =====================

type CombiAdapter struct {
	BaseAdapter

	// USB handles
	usbCtx *gousb.Context
	dev    *gousb.Device
	devCfg *gousb.Config
	iface  *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint

	closeOnce sync.Once

	// Tx pooling for CAN frames (fixed 19-byte packet)
	txPool sync.Pool

	// Control-plane rendezvous: exactly one in-flight control command
	ctrlMu   sync.Mutex
	ctrlWait *ctrlWait

	// App-side channels for helper queries
	adcFilterChan   chan bool
	adcValueChan    chan float32
	thermoValueChan chan float32

	// Diagnostics
	dropCount uint64
}

type ctrlWait struct {
	cmd byte
	ch  chan []byte // payload of the control message, if any
}

// Boilerplate registration
func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "CombiAdapter",
		Description:        "libusb windows driver",
		RequiresSerialPort: false,
		Capabilities:       AdapterCapabilities{HSCAN: true},
		New:                NewCombi,
	}); err != nil {
		panic(err)
	}
}

// NewCombi constructs an adapter with sane defaults.
func NewCombi(cfg *AdapterConfig) (Adapter, error) {
	return &CombiAdapter{
		BaseAdapter: NewBaseAdapter("CombiAdapter", cfg),
		txPool: sync.Pool{New: func() any {
			// cmd, size(2), ID(4), data(8), len, ext, rtr, term
			return []byte{cmdCanTxFrame, 0x00, 0x0F, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		}},
		adcFilterChan:   make(chan bool, 1),
		adcValueChan:    make(chan float32),
		thermoValueChan: make(chan float32, 1),
	}, nil
}

// ============
// Public API
// ============

func (ca *CombiAdapter) SetFilter(_ []uint32) error { return nil }

func (ca *CombiAdapter) Open(ctx context.Context) error {
	// --- Discover & bind USB ---
	ctxUSB := gousb.NewContext()
	dev, err := ctxUSB.OpenDeviceWithVIDPID(combiVid, combiPid)
	if err != nil {
		if dev == nil {
			ctxUSB.Close()
			return errors.New("CombiAdapter not found")
		}
		dev.Close()
		ctxUSB.Close()
		return err
	}
	if dev == nil {
		ctxUSB.Close()
		return errors.New("CombiAdapter not found 2")
	}

	cfg, err := dev.Config(usbConfigNumber)
	if err != nil {
		dev.Close()
		ctxUSB.Close()
		return err
	}
	iface, err := cfg.Interface(usbInterfaceNum, usbAltSetting)
	if err != nil {
		cfg.Close()
		dev.Close()
		ctxUSB.Close()
		return err
	}
	in, err := iface.InEndpoint(usbInEndpointNum)
	if err != nil {
		iface.Close()
		cfg.Close()
		dev.Close()
		ctxUSB.Close()
		return fmt.Errorf("InEndpoint(%d): %w", usbInEndpointNum, err)
	}
	out, err := iface.OutEndpoint(usbOutEndpointNum)
	if err != nil {
		// Some units expose OUT on EP2 (STM32 clones)
		ca.Info("trying EP 2 (stm32 clone)")
		out, err = iface.OutEndpoint(2)
		if err != nil {
			iface.Close()
			cfg.Close()
			dev.Close()
			ctxUSB.Close()
			return err
		}
	}

	// Save
	ca.usbCtx, ca.dev, ca.devCfg = ctxUSB, dev, cfg
	ca.iface, ca.in, ca.out = iface, in, out

	// --- Put CAN into a known state & drain stale bytes ---
	if err := ca.canCtrl(0); err != nil {
		ca.closeUSB()
		return fmt.Errorf("failed to close canbus: %w", err)
	}
	ca.drainInput(ctx, 150*time.Millisecond)

	// Optional: print FW version
	if ca.cfg.PrintVersion {
		if ver, err := ca.ReadVersion(ctx); err == nil {
			ca.Info(ver)
		}
	}

	// Configure + open CAN unless disabled
	if ca.cfg.AdditionalConfig["NoConnect"] != "true" {
		if err := ca.setBitrate(ctx); err != nil {
			ca.closeUSB()
			return err
		}
		if err := ca.canCtrl(1); err != nil {
			ca.closeUSB()
			return fmt.Errorf("failed to open canbus: %w", err)
		}
	}

	// Start I/O goroutines after endpoints are ready
	go ca.recvManager(ctx)
	go ca.sendManager(ctx)
	return nil
}

func (ca *CombiAdapter) Close() error {
	ca.BaseAdapter.Close()
	var err error
	ca.closeOnce.Do(func() { err = ca.closeAdapter() })
	return err
}

func (ca *CombiAdapter) ReadVersion(ctx context.Context) (string, error) {
	wctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if err := ca.sendCommand(wctx, cmdBrdFWVersion, nil, 200); err != nil {
		return "", err
	}
	buf := make([]byte, ca.in.Desc.MaxPacketSize)
	_, err := ca.in.ReadContext(wctx, buf)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CombiAdapter v%d.%d", buf[4], buf[3]), nil
}

func (ca *CombiAdapter) GetADCFiltering(ctx context.Context, channel int) (bool, error) {
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

func (ca *CombiAdapter) SetADCFiltering(ctx context.Context, channel int, enabled bool) error {
	var en byte
	if enabled {
		en = 0x01
	}
	return ca.sendCommand(ctx, cmdBrdADCFilter, []byte{byte(channel), en}, 10)
}

func (ca *CombiAdapter) GetADCValue(ctx context.Context, channel int) (float64, error) {
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

func (ca *CombiAdapter) GetThermoValue(ctx context.Context) (float32, error) {
	// This command is triggered via the system message path on purpose
	ca.sendChan <- &CANFrame{Identifier: SystemMsg, Data: []byte{cmdBrdEGT, 0x00, 0x00, 0x00}}
	select {
	case f := <-ca.thermoValueChan:
		return f, nil
	case <-time.After(5 * time.Millisecond):
		return 0, fmt.Errorf("timeout waiting for thermo value response")
	}
}

func (ca *CombiAdapter) DroppedFrames() uint64 { return atomic.LoadUint64(&ca.dropCount) }

// =====================
// Internal: USB lifecycle
// =====================

func (ca *CombiAdapter) closeAdapter() error {
	_ = ca.canCtrl(0) // best-effort close CAN
	time.Sleep(20 * time.Millisecond)
	ca.closeUSB()
	return nil
}

func (ca *CombiAdapter) closeUSB() {
	if ca.iface != nil {
		ca.iface.Close()
	}
	if ca.devCfg != nil {
		_ = ca.devCfg.Close()
	}
	if ca.dev != nil {
		_ = ca.dev.Close()
	}
	if ca.usbCtx != nil {
		_ = ca.usbCtx.Close()
	}
}

// =====================
// Internal: Control plane
// =====================

func (ca *CombiAdapter) sendCommand(ctx context.Context, cmd byte, data []byte, timeoutMs int) error {
	if len(data) > MaxCommandSize {
		return fmt.Errorf("%w: %d", ErrCommandSizeTooLarge, len(data))
	}
	w := &ctrlWait{cmd: cmd, ch: make(chan []byte, 1)}

	// Register waiter (single in-flight)
	ca.ctrlMu.Lock()
	if ca.ctrlWait != nil {
		ca.ctrlMu.Unlock()
		return fmt.Errorf("control pipe busy, cmd %02X in-flight", ca.ctrlWait.cmd)
	}
	ca.ctrlWait = w
	ca.ctrlMu.Unlock()

	// Build packet: cmd + len(2) + payload + term
	plen := len(data)
	pkt := make([]byte, 3+plen+1)
	pkt[0] = cmd
	pkt[1] = byte(plen >> 8)
	pkt[2] = byte(plen)
	copy(pkt[3:], data)
	pkt[3+plen] = termAck

	// Write with timeout
	t := time.Duration(timeoutMs) * time.Millisecond
	if t == 0 {
		t = 250 * time.Millisecond
	}
	wctx, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	if _, err := ca.out.WriteContext(wctx, pkt); err != nil {
		ca.ctrlMu.Lock()
		ca.ctrlWait = nil
		ca.ctrlMu.Unlock()
		return fmt.Errorf("send cmd %02X failed: %w", cmd, err)
	}

	// Wait for matching control response (delivered by recvManager)
	select {
	case <-wctx.Done():
		ca.ctrlMu.Lock()
		ca.ctrlWait = nil
		ca.ctrlMu.Unlock()
		return wctx.Err()
	case <-w.ch:
		return nil
	}
}

func (ca *CombiAdapter) handleControlCommand(cmd byte, data []byte) error {
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

// ====================
// Internal: Data plane
// ====================

func (ca *CombiAdapter) sendManager(ctx context.Context) {
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
				// Raw write for board/system messages
				if _, err := ca.out.WriteContext(ctx, frame.Data); err != nil {
					ca.Error(fmt.Errorf("failed to send frame: %w", err))
				}
				continue
			}
			ca.sendCANMessage(ctx, frame)
		}
	}
}

func (ca *CombiAdapter) sendCANMessage(ctx context.Context, frame *CANFrame) {
	buf := ca.txPool.Get().([]byte)
	defer ca.txPool.Put(buf)

	binary.LittleEndian.PutUint32(buf[3:], frame.Identifier)
	copy(buf[7:], frame.Data[:min(frame.Length(), 8)])
	buf[15] = uint8(frame.Length())
	buf[16] = boolToByte(frame.Extended)
	buf[17] = boolToByte(frame.RTR)
	// buf[18] is the pre-set terminator (0)

	wctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	n, err := ca.out.WriteContext(wctx, buf)
	if err != nil {
		ca.Fatal(fmt.Errorf("failed to send frame: %w", err))
		return
	}
	if n != 19 {
		ca.Error(fmt.Errorf("sent %d bytes of data out of 19", n))
	}
}

func (ca *CombiAdapter) recvManager(ctx context.Context) {
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
			n, err := ca.in.ReadContext(ctx, readBuf[:])
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				ca.Error(fmt.Errorf("failed to read from usb device: %w", err))
				if n == 0 {
					continue
				}
			}
			for _, b := range readBuf[:n] {
				switch state {
				case psCmd:
					if _, ok := combiValidCommands[b]; !ok {
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

func (ca *CombiAdapter) processCommand(cmd byte, data []byte) error {
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

	case cmdBrdFWVersion, cmdCanOpen, cmdCanBitrate, cmdCanTxFrame:
		return ca.handleControlCommand(cmd, data)

	case cmdCanFrame:
		return ca.handleCANFrame(data)
	}
	return fmt.Errorf("%w: %02X", ErrInvalidCommand, cmd)
}

func (ca *CombiAdapter) handleCANFrame(data []byte) error {
	if len(data) != 15 {
		return fmt.Errorf("invalid CAN frame size: %d", len(data))
	}
	frame := NewFrame(
		binary.LittleEndian.Uint32(data[:4]),
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

// =========================
// Internal: Utility helpers
// =========================

func (ca *CombiAdapter) drainInput(ctx context.Context, max time.Duration) {
	deadline := time.Now().Add(max)
	tmp := make([]byte, ca.in.Desc.MaxPacketSize)
	for time.Now().Before(deadline) {
		dctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		_, err := ca.in.ReadContext(dctx, tmp)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			// otherwise, keep draining
		}
	}
}

func (ca *CombiAdapter) canCtrl(mode byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(ctx, []byte{cmdCanOpen, 0x00, 0x01, mode, termAck}); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

func (ca *CombiAdapter) setBitrate(ctx context.Context) error {
	var rate uint32
	switch ca.cfg.CANRate {
	case 615.384:
		rate = 615000
	default:
		rate = uint32(ca.cfg.CANRate * 1000)
	}
	log.Println(rate)
	wctx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	pkt := []byte{cmdCanBitrate, 0x00, 0x04, byte(rate >> 24), byte(rate >> 16), byte(rate >> 8), byte(rate), termAck}
	if _, err := ca.out.WriteContext(wctx, pkt); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

/*
// Dev helper to list USB topology
func list() {
	ctx := gousb.NewContext()
	defer ctx.Close()
	ctx.Debug(0)
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		fmt.Printf("%03d.%03d %s:%s %s\n", desc.Bus, desc.Address, desc.Vendor, desc.Product, usbid.Describe(desc))
		fmt.Printf("  Protocol: %s\n", usbid.Classify(desc))
		for _, cfg := range desc.Configs {
			fmt.Printf("  %s:\n", cfg)
			for _, intf := range cfg.Interfaces {
				fmt.Printf("    --------------\n")
				for _, as := range intf.AltSettings {
					fmt.Printf("    %s\n", as)
					fmt.Printf("      %s\n", usbid.Classify(as))
					for _, end := range as.Endpoints {
						fmt.Printf("      %s\n", end)
					}
				}
				fmt.Printf("    --------------\n")
			}
		}
		return false
	})
	defer func() {
		for _, d := range devs {
			d.Close()
		}
	}()
	if err != nil {
		log.Fatalf("list: %s", err)
	}
}
*/
