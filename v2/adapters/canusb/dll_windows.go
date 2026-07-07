//go:build canusb

package canusb

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	gocan "github.com/roffe/gocan/v2"
	dll "github.com/roffe/gocan/v2/pkg/canusb"
)

// Registers Lawicel CANUSB devices attached via the Lawicel canusbdrv DLL as
// "CANUSB <serial>" (the v1 names). Opt-in with the "canusb" build tag; needs
// canusbdrv(64).dll installed.
func init() {
	gocan.RegisterScanner(scanDLLDevices)
}

func scanDLLDevices() []gocan.AdapterInfo {
	if err := dll.Init(); err != nil {
		log.Println("CANUSB driver not loaded:", err)
		return nil
	}
	adapters, err := dll.GetAdapters()
	if err != nil {
		log.Println("CANUSB adapter enumeration failed:", err)
		return nil
	}
	log.Printf("CANUSB DLL found %d adapter(s): %v", len(adapters), adapters)
	var out []gocan.AdapterInfo
	for _, serial := range adapters {
		out = append(out, gocan.AdapterInfo{
			Name:         "CANUSB " + serial,
			Description:  "Lawicel CANUSB via canusbdrv.dll",
			Capabilities: gocan.Capabilities{HSCAN: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				return &DLL{cfg: cfg, serial: serial}, nil
			},
		})
	}
	return out
}

// DLL drives a Lawicel CANUSB through the vendor canusbdrv DLL. Receive is
// push: the DLL invokes our callback from its own thread and we hand the
// frame straight to bus.Deliver (which never blocks).
type DLL struct {
	cfg    gocan.Config
	bus    *gocan.Bus
	serial string

	h         *dll.CANHANDLE
	closeOnce sync.Once
}

func (d *DLL) Open(ctx context.Context, bus *gocan.Bus) error {
	d.bus = bus
	code, mask := dllAcceptance(d.cfg.CANFilter)
	rate := dllBitRate(d.cfg.CANRate)
	d.debug(fmt.Sprintf("open serial=%q rate=%q code=%08X mask=%08X", d.serial, rate, code, mask))
	// Physical channel teardown is asynchronous after the last canusb_Close,
	// so a quick stop→start can find the device still busy (Open returns 0);
	// retry briefly before giving up.
	var h *dll.CANHANDLE
	var err error
	for attempt := 0; ; attempt++ {
		h, err = dll.Open(d.serial, rate, code, mask,
			dll.FLAG_NO_LOCAL_SEND|dll.FLAG_BLOCK|dll.FLAG_TIMESTAMP|dll.FLAG_SLOW)
		if err == nil {
			break
		}
		if err != dll.ErrNoDeviceAvailable || attempt >= 5 {
			return fmt.Errorf("canusb_Open failed: %w", err)
		}
		d.debug(fmt.Sprintf("device busy, open retry %d", attempt+1))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	d.h = h

	if ver, err := h.VersionInfo(); err != nil {
		bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "get version failed: " + err.Error()})
	} else {
		bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "CANUSB " + ver})
	}

	if err := h.SetReceiveCallback(d.deliver); err != nil {
		h.Close()
		return fmt.Errorf("SetReceiveCallback failed: %w", err)
	}

	go d.statusLoop(ctx)
	return nil
}

// Close empties the receive queue, waits out the transmit queue and releases
// the handle. Guarded for the Bus.Close and failed-Open double call.
func (d *DLL) Close() error {
	if d.h == nil { // Open never ran or failed
		return nil
	}
	var err error
	d.closeOnce.Do(func() {
		// Unhook the callback first: closing with it registered can wedge the
		// DLL's receive thread so the physical FTDI channel never terminates
		// (opens are ref-counted; only the last clean close releases the
		// device) and the next canusb_Open returns 0.
		if cerr := d.h.SetReceiveCallback(nil); cerr != nil {
			d.error(fmt.Errorf("clear callback failed: %w", cerr))
		}
		if ferr := d.h.Flush(dll.FLUSH_EMPTY_INQUEUE | dll.FLUSH_WAIT); ferr != nil {
			d.error(fmt.Errorf("flush failed: %w", ferr))
		}
		err = d.h.Close()
	})
	return err
}

// Send writes one frame and flushes with FLUSH_WAIT, which blocks until the
// transmit queue drains — the closest the DLL offers to an on-the-wire ack.
func (d *DLL) Send(ctx context.Context, f gocan.Frame) error {
	msg := &dll.CANMsg{ID: f.ID, Len: f.Length}
	if f.Extended {
		msg.Flags |= dll.CANMSG_EXTENDED
	}
	if f.Remote {
		msg.Flags |= dll.CANMSG_RTR
	}
	copy(msg.Data[:], f.Data[:f.Length])
	if d.cfg.Debug {
		d.debug(">> " + f.String())
	}
	if err := d.h.Write(msg); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	if err := d.h.Flush(dll.FLUSH_WAIT); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}
	return nil
}

// deliver is the DLL receive callback.
func (d *DLL) deliver(msg *dll.CANMsg) uintptr {
	if msg.Len > 8 {
		// Len > 8 means the callback struct layout is off, not a long frame.
		d.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning,
			Details: fmt.Sprintf("dropped frame with bogus len: id=%X flags=%02X len=%d", msg.ID, msg.Flags, msg.Len)})
		return 0
	}
	if d.cfg.Debug {
		d.debug("<< " + msg.String())
	}
	f := gocan.Frame{
		ID:       msg.ID,
		Extended: msg.Flags&dll.CANMSG_EXTENDED != 0,
		Remote:   msg.Flags&dll.CANMSG_RTR != 0,
		Length:   msg.Len,
	}
	copy(f.Data[:], msg.Data[:msg.Len])
	d.bus.Deliver(f)
	return 0
}

// statusLoop polls the controller status flags and reports error conditions.
// The API spec warns Status degrades performance and recommends calling it at
// most once every ten seconds. Arbitration lost is normal bus contention and
// ERROR_CANUSB_TIMEOUT just means "call again" (spec), so both are skipped.
func (d *DLL) statusLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.h.Status(); err != nil && err != dll.ErrArbitrationLost && err != dll.ErrTimeout {
				d.error(err)
			}
			if d.cfg.Debug {
				if stats, err := d.h.GetStatistics(); err == nil {
					d.debug(stats.String())
				}
			}
		}
	}
}

func (d *DLL) error(err error) {
	d.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
}

func (d *DLL) debug(msg string) {
	d.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: msg})
}

// dllAcceptance packs the SJA1000 dual-filter registers from accept11 into
// the DWORDs canusb_Open wants, falling back to accept-everything.
//
// Little-endian on purpose: the DLL byte-swaps each DWORD before formatting
// it into the firmware "M%08X"/"m%08X" command (canusbdrv64.dll @180002cf2,
// canusbdrv.dll @10002ab6), so LE packing puts ACR0..ACR3 on the wire in
// M-string order — the same bytes the VCP/d2xx setup path sends. Big-endian
// packing shifts every register one byte and kills all reception.
func dllAcceptance(ids []uint32) (uint32, uint32) {
	ac, am, err := accept11(ids)
	if err != nil {
		return dll.ACCEPTANCE_CODE_ALL, dll.ACCEPTANCE_MASK_ALL
	}
	return binary.LittleEndian.Uint32(ac[:]), binary.LittleEndian.Uint32(am[:])
}

// dllBitRate maps a CAN rate in kbit/s to the DLL's bit-rate string: a plain
// kbit/s number ("500") for the standard rates per the API spec, BTR0:BTR1
// for the odd ones.
func dllBitRate(rate float64) string {
	switch rate {
	case 33.3:
		return "0x0e:0x1c"
	case 47.619:
		return "0xcb:0x9a"
	case 615.384:
		return "0x40:0x37"
	default:
		return strconv.FormatFloat(rate, 'f', -1, 64)
	}
}
