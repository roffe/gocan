//go:build j2534 && (windows || linux)

package j2534

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/passthru"
)

func init() {
	gocan.RegisterScanner(scanDevices)
}

func scanDevices() []gocan.AdapterInfo {
	prefix, dlls := passthru.FindDLLs()
	var out []gocan.AdapterInfo
	for i, dll := range dlls {
		name := fmt.Sprintf("%sJ2534 #%d %s", prefix, i, dll.Name)
		dllPath := dll.FunctionLibrary
		out = append(out, gocan.AdapterInfo{
			Name:        name,
			Description: "J2534 Interface",
			Capabilities: gocan.Capabilities{
				HSCAN: dll.Capabilities.CAN || dll.Capabilities.CANPS,
				KLine: dll.Capabilities.ISO9141 || dll.Capabilities.ISO14230,
				SWCAN: dll.Capabilities.SWCANPS,
			},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				cfg.Port = dllPath
				return New(cfg)
			},
		})
	}
	return out
}

const (
	// Reads never block (timeout 0); an idle worker waits this long for queued
	// work before polling again. A blocking read held every Send behind it:
	// with a 10 ms read timeout a T7, which sends the next response frame only
	// after our 0x266 ACK, paid ~10 ms per frame and logged at ~16 Hz.
	// Measured on a MongoosePro + T7 (4-frame read): 10 ms blocking 50 ms,
	// 1 ms poll 9.1 ms, 100 µs poll 6.6 ms at ~1-5% of a core.
	// ponytail: fixed poll, up to 10k empty reads/s; back off when idle if a
	// vendor DLL turns out to do device I/O per read.
	pollInterval = 100 * time.Microsecond
	// A DLL that fails reads instantly (cable pulled, channel invalidated)
	// would otherwise spin the worker at 100% CPU spamming events. Bail out
	// rather than trying to enumerate every return code that means "dead".
	maxReadErrors = 20
	// How long the worker keeps serving calls after the bus context dies
	// before tearing the DLL down itself. Bus.Fatal cancels the context
	// without calling Close, so without this the locked OS thread would leak.
	closeGrace = 3 * time.Second
)

type J2534 struct {
	cfg gocan.Config
	bus *gocan.Bus

	h *passthru.PassThru

	channelID uint32
	deviceID  uint32
	flags     uint32
	protocol  uint32

	tech2passThru bool

	calls chan func()   // DLL work, run on the worker's locked OS thread
	done  chan struct{} // closed once the worker has exited

	// Worker-thread only, no synchronization needed.
	filters  []uint32
	rx       passthru.PassThruMsg // reused; PassThruMsg is 4KB
	torn     bool                 // DLL released, nothing may call into it
	closeErr error
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &J2534{
		cfg:     cfg,
		flags:   passthru.CAN_ID_BOTH | passthru.CAN_29BIT_ID,
		filters: cfg.CANFilter,
		calls:   make(chan func()),
		done:    make(chan struct{}),
	}, nil
}

func (ma *J2534) Open(ctx context.Context, bus *gocan.Bus) error {
	ma.bus = bus
	ma.tech2passThru = strings.HasSuffix(ma.cfg.Port, "Tech2_32.dll")

	// Resolved before the worker starts so an unusable rate costs no DLL work.
	var swcan bool
	var baudRate uint32
	switch ma.cfg.CANRate {
	case 250:
		baudRate = 250000
		ma.protocol = passthru.CAN
	case 33.3:
		baudRate = 33333
		ma.protocol = passthru.SW_CAN_PS
		swcan = true
	case 47.619:
		baudRate = 47619
		ma.protocol = passthru.CAN
	case 500:
		baudRate = 500000
		ma.protocol = passthru.CAN
	case 615.384:
		baudRate = 615384
		ma.protocol = passthru.CAN
	default:
		close(ma.done)
		return errors.New("invalid CAN rate")
	}

	errc := make(chan error, 1)
	go ma.run(ctx, baudRate, swcan, errc)
	return <-errc
}

// run owns the single OS thread every call into the DLL is made from, loading
// it included. J2534 DLLs are written for single-threaded use — several keep
// the device handle, the PassThruGetLastError text or a COM apartment in
// thread-local storage — and a goroutine would otherwise hop threads between
// calls. Serializing here also covers what the Tech2_32.dll mutex used to.
func (ma *J2534) run(ctx context.Context, baudRate uint32, swcan bool, errc chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(ma.done)

	if err := ma.open(baudRate, swcan); err != nil {
		errc <- err
		return
	}
	errc <- nil

	errCount := 0
	for {
		// Queued work (Send, SetFilter, teardown) goes first; polling the bus
		// is what this thread does with the time left over.
		select {
		case fn := <-ma.calls:
			fn()
			if ma.torn {
				return
			}
			continue
		default:
		}

		if ctx.Err() != nil {
			// Stop touching the bus, but stay available for Close's teardown.
			select {
			case fn := <-ma.calls:
				fn()
			case <-time.After(closeGrace):
				ma.teardown()
			}
			if ma.torn {
				return
			}
			continue
		}

		got, err := ma.pump()
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			errCount++
			if errors.Is(err, passthru.ErrDeviceNotConnected) || errCount >= maxReadErrors {
				ma.teardown()
				ma.bus.Fatal(fmt.Errorf("read failed %d time(s) in a row: %w", errCount, err))
				return
			}
			ma.emit(gocan.EventTypeError, err.Error())
			continue
		}
		errCount = 0
		if got {
			continue // drain before idling
		}
		select {
		case fn := <-ma.calls:
			fn()
			if ma.torn {
				return
			}
		case <-time.After(pollInterval):
		}
	}
}

// open runs on the worker thread. Every failure path releases what it already
// acquired and marks the adapter torn down, so run exits without polling;
// without it the DLL keeps the device and the next open fails with
// ERR_DEVICE_IN_USE.
func (ma *J2534) open(baudRate uint32, swcan bool) error {
	var err error
	if ma.h, err = passthru.New(ma.cfg.Port); err != nil {
		ma.torn = true
		return err
	}

	if err := ma.h.PassThruOpen("", &ma.deviceID); err != nil {
		ma.h.Close()
		ma.torn = true
		return fmt.Errorf("PassThruOpen: %w", err)
	}

	if firmwareVersion, dllVersion, apiVersion, err := ma.h.PassThruReadVersion(ma.deviceID); err == nil {
		ma.emit(gocan.EventTypeInfo, fmt.Sprintf("Firmware: %s DLL: %s API: %s", firmwareVersion, dllVersion, apiVersion))
	}

	if err := ma.h.PassThruConnect(ma.deviceID, ma.protocol, ma.flags, baudRate, &ma.channelID); err != nil {
		ma.h.PassThruClose(ma.deviceID)
		ma.h.Close()
		ma.torn = true
		return fmt.Errorf("PassThruConnect: %w", err)
	}

	if ma.tech2passThru {
		time.Sleep(2 * time.Second)
	}

	if swcan {
		opts := &passthru.SCONFIG_LIST{
			Params: []passthru.SCONFIG{
				{Parameter: passthru.J1962_PINS, Value: 0x0100},
			},
		}
		if err := ma.h.PassThruIoctl(ma.channelID, passthru.SET_CONFIG, opts, nil); err != nil {
			ma.teardown()
			return fmt.Errorf("PassThruIoctl set SWCAN: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := ma.h.PassThruIoctl(ma.channelID, passthru.CLEAR_RX_BUFFER, nil, nil); err != nil {
		ma.teardown()
		return fmt.Errorf("PassThruIoctl clear rx buffer: %w", err)
	}

	if len(ma.filters) > 0 {
		if err := ma.setupFilters(); err != nil {
			ma.teardown()
			return err
		}
	} else {
		ma.allowAll()
	}
	return nil
}

// pump does one non-blocking read and delivers what it got. got reports
// whether the DLL returned a message at all (so the caller keeps draining); err
// is only for conditions that say something about the health of the channel.
func (ma *J2534) pump() (got bool, err error) {
	ma.rx.ProtocolID = ma.protocol
	n, err := ma.h.PassThruReadMsg(ma.channelID, &ma.rx, 0)
	if err != nil {
		// The spec lists both for an empty read and DLLs disagree on which
		// they return: Tactrix/DrewTech/MDI answer ERR_TIMEOUT once the
		// window expires. Treating that as an error spams one event per poll
		// on an idle bus.
		if errors.Is(err, passthru.ErrBufferEmpty) || errors.Is(err, passthru.ErrTimeout) {
			return false, nil
		}
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	// Loopback echoes and start-of-message indications arrive on the same
	// channel as real traffic; delivering them duplicates frames we sent.
	if ma.rx.RxStatus&(passthru.TX_MSG_TYPE|passthru.START_OF_MESSAGE) != 0 {
		return true, nil
	}
	if ma.rx.DataSize < 4 || ma.rx.DataSize > 12 {
		ma.emit(gocan.EventTypeError, fmt.Sprintf("bad message size: %d", ma.rx.DataSize))
		return true, nil
	}
	f := gocan.Frame{
		ID:       binary.BigEndian.Uint32(ma.rx.Data[0:4]),
		Length:   uint8(ma.rx.DataSize - 4),
		Extended: ma.rx.RxStatus&passthru.CAN_29BIT_ID != 0,
	}
	copy(f.Data[:], ma.rx.Data[4:ma.rx.DataSize])
	ma.bus.Deliver(f)
	return true, nil
}

// do runs fn on the worker's OS thread and waits for it to finish.
func (ma *J2534) do(fn func()) error {
	done := make(chan struct{})
	select {
	case ma.calls <- func() { defer close(done); fn() }:
	case <-ma.done:
		return gocan.ErrClosed
	}
	select {
	case <-done:
		return nil
	case <-ma.done:
		return gocan.ErrClosed
	}
}

// teardown runs on the worker thread. Best-effort: run every step even if one
// fails (e.g. cable already unplugged), or the device stays open in the DLL.
func (ma *J2534) teardown() {
	if ma.torn || ma.h == nil {
		return
	}
	ma.torn = true
	ma.closeErr = errors.Join(
		ma.h.PassThruIoctl(ma.channelID, passthru.CLEAR_MSG_FILTERS, nil, nil),
		ma.h.PassThruDisconnect(ma.channelID),
		ma.h.PassThruClose(ma.deviceID),
		ma.h.Close(),
	)
}

// Close tears the DLL down on the worker thread, so it cannot race a read
// still in flight, and waits for that thread to exit.
// ponytail: 1s total escape hatch in case a vendor DLL blocks past its timeout.
func (ma *J2534) Close() error {
	deadline := time.After(time.Second)
	select {
	case ma.calls <- ma.teardown:
	case <-ma.done: // already torn down on the open, fatal or grace path
	case <-deadline:
		return errors.New("timed out queueing J2534 teardown")
	}
	select {
	case <-ma.done:
	case <-deadline:
		return errors.New("timed out waiting for J2534 teardown")
	}
	return ma.closeErr
}

func (ma *J2534) Send(ctx context.Context, f gocan.Frame) error {
	var txflags uint32
	if f.Extended {
		txflags = passthru.CAN_29BIT_ID
	}
	msg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4 + uint32(f.Length),
		ExtraDataIndex: 4 + uint32(f.Length),
		TxFlags:        txflags,
	}
	if ma.protocol == passthru.SW_CAN_PS && !ma.tech2passThru {
		msg.TxFlags |= passthru.SW_CAN_HV_TX
	}
	binary.BigEndian.PutUint32(msg.Data[:], f.ID)
	copy(msg.Data[4:], f.Bytes())

	var err error
	if derr := ma.do(func() {
		// Timeout 0 queues the frame in the device and returns; a non-zero
		// timeout blocks until it is on the wire, a USB round trip per frame
		// (T7 flash on a MongoosePro: 32 s). A full queue refuses the frame
		// with 0 queued: ERR_BUFFER_FULL per spec, ERR_TIMEOUT from the
		// MongoosePro ("only sent 0 of 1", device code 0x103) when a T7 fast
		// download bursts 1024 frames. Keep reading while it drains so replies
		// aren't held up, and retry.
		deadline := time.Now().Add(time.Second)
		for {
			numMsg := uint32(1)
			err = ma.h.PassThruWriteMsgs(ma.channelID, msg, &numMsg, 0)
			full := numMsg == 0 && (errors.Is(err, passthru.ErrBufferFull) || errors.Is(err, passthru.ErrTimeout))
			if !full || ctx.Err() != nil || time.Now().After(deadline) {
				break
			}
			if got, _ := ma.pump(); !got {
				time.Sleep(pollInterval)
			}
		}
		if err == nil {
			return
		}
		// A bare ERR_TIMEOUT hides why the device could not transmit; the
		// library's text usually carries the device code. checkErr only fetches
		// it for ERR_FAILED, and it is thread-local, so ask here.
		if s, lerr := ma.h.PassThruGetLastError(); lerr == nil && s != "" {
			err = fmt.Errorf("%s: %w", s, err)
		}
	}); derr != nil {
		return derr
	}
	return err
}

// SetFilter replaces the installed PASS filters at runtime.
func (ma *J2534) SetFilter(filters []uint32) error {
	var err error
	if derr := ma.do(func() {
		if err = ma.h.PassThruClearMsgFilters(ma.channelID); err != nil {
			return
		}
		ma.filters = filters
		if len(filters) > 0 {
			err = ma.setupFilters()
			return
		}
		ma.allowAll()
	}); derr != nil {
		return derr
	}
	return err
}

func (ma *J2534) allowAll() {
	filterID := uint32(0)
	var txflags uint32
	if ma.cfg.UseExtendedID {
		txflags = passthru.CAN_29BIT_ID
	}
	maskMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		TxFlags:        txflags,
	}
	patternMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		TxFlags:        txflags,
	}
	if err := ma.h.PassThruStartMsgFilter(ma.channelID, passthru.PASS_FILTER, maskMsg, patternMsg, nil, &filterID); err != nil {
		ma.emit(gocan.EventTypeError, fmt.Sprintf("PassThruStartMsgFilter: %v", err))
	}
}

func (ma *J2534) setupFilters() error {
	if len(ma.filters) > 10 {
		return errors.New("too many filters")
	}
	var txflags uint32
	if ma.cfg.UseExtendedID {
		txflags = passthru.CAN_29BIT_ID
	}
	idMask := uint32(0x7FF)
	if ma.cfg.UseExtendedID {
		idMask = 0x1FFFFFFF
	}
	maskMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		TxFlags:        txflags,
	}
	binary.BigEndian.PutUint32(maskMsg.Data[:], idMask)
	for _, filter := range ma.filters {
		var filterID uint32
		patternMsg := &passthru.PassThruMsg{
			ProtocolID:     ma.protocol,
			DataSize:       4,
			ExtraDataIndex: 4,
			TxFlags:        txflags,
		}
		binary.BigEndian.PutUint32(patternMsg.Data[:], filter)
		if err := ma.h.PassThruStartMsgFilter(ma.channelID, passthru.PASS_FILTER, maskMsg, patternMsg, nil, &filterID); err != nil {
			return err
		}
	}
	return nil
}

func (ma *J2534) emit(t gocan.EventType, details string) {
	ma.bus.Emit(gocan.Event{Type: t, Details: details})
}
