// Package canusb drives the Lawicel CANUSB over its FTDI Virtual COM Port,
// implemented from the CANUSB Manual (Version 1.0D, November 2011).
// Importing the package registers the "CANUSB VCP" adapter.
//
// Command set used (all commands end with CR, ASCII 13, and are case sensitive):
//
//	Sn          set standard bit-rate (S0..S8)
//	sxxyy       set bit-rate via BTR0/BTR1 (used for non-standard rates)
//	O / C       open / close the CAN channel
//	tiiildd..   transmit standard (11-bit) frame, ack: z
//	Tiiiiiiiildd.. transmit extended (29-bit) frame, ack: Z
//	Mxxxxxxxx   acceptance code   (channel initiated, not open)
//	mxxxxxxxx   acceptance mask   (channel initiated, not open)
//	V / N       hardware+software version / serial number
//	Zn          received-frame timestamp on/off (we keep it OFF)
//	F           read & clear status flags (only while open)
//
// Replies: CR (OK) / BELL 0x07 (error) for setup commands, z/Z for transmit
// acks, and unsolicited t.../T... lines for received frames.
//
// Transmit is credit-gated: Send takes a slot from a semaphore that the reply
// parser releases on z/Z/F/BELL, so at most defaultTxCredits commands are ever
// outstanding (see the constant for what the manual allows and why the depth
// matters). Writes are serialized, and the device drains its USB FIFO in order,
// so frame order is preserved regardless of the window.
package canusb

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

const (
	cr   = 0x0D // command terminator / OK
	bell = 0x07 // error reply
)

var cmdStatus = []byte{'F', cr} // polled every second; shared so it isn't reallocated

// Transmit flow control: how many commands may be outstanding before Send
// blocks for the device's z/Z ack.
//
// The manual gives two different depths and the smaller one binds. §1.2: the
// CAN transmit FIFO "can handle 8 frames". §1.5: "the CANUSB has also USB
// FIFO's built in the hardware, so it can only handle one or two command at a
// time, meaning before sending the next command to it, you must wait for an
// answer". So the USB command parser, not the CAN queue, is the limit — 2 is
// the documented ceiling and 8 the hard one.
//
// The window only matters for bulk transfers, where it matters a lot: a
// Trionic 7 flash is ~75k frames and every one of them is round-trip bound, so
// each 0.1 ms of round trip costs ~7 s of flash time. One credit serialises all
// 75k round trips; two overlaps the write of the next frame with the ack of the
// previous. Extra["txcredits"] tunes it for a specific unit — the manual's
// "one or two" is vague enough that the right value is a property of the
// hardware, not of this code.
const (
	defaultTxCredits = 2
	maxTxCredits     = 8 // CAN transmit FIFO depth, manual §1.2
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:               "CANUSB VCP",
		Description:        "Lawicel CANUSB (VCP, manual 1.0D)",
		RequiresSerialPort: true,
		Capabilities:       gocan.Capabilities{HSCAN: true, SWCAN: true},
		New:                New,
	})
}

// serialPort is the slice of serial.Port the adapter uses, so tests can
// substitute a fake.
type serialPort interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	SetReadTimeout(t time.Duration) error
	ResetInputBuffer() error
	ResetOutputBuffer() error
}

type CANUSB struct {
	cfg      gocan.Config
	bus      *gocan.Bus
	port     serialPort                 // pre-set by tests; opened via openPort otherwise
	openPort func() (serialPort, error) // opens the transport; nil = VCP from cfg.Port

	canRate    string // S/s command for the configured bit-rate
	code, mask string // M acceptance-code / m acceptance-mask commands

	sendSem chan struct{} // outstanding-command credits, released by the z/Z/F/BELL reply
	writeMu sync.Mutex    // serializes port writes (Send vs status poll vs SetFilter)
	line    []byte        // reply parser accumulator
	txBuf   [27]byte      // longest transmit command (T + 8 id + dlc + 16 data + CR); Send-only, which the Bus serializes
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	rate, err := bitRate(cfg.CANRate)
	if err != nil {
		return nil, err
	}
	if cfg.PortBaudrate == 0 {
		cfg.PortBaudrate = 3_000_000
	}
	credits, err := txCredits(cfg)
	if err != nil {
		return nil, err
	}
	code, mask := acceptanceFilters(cfg.CANFilter)
	return &CANUSB{
		cfg:     cfg,
		canRate: rate,
		code:    code,
		mask:    mask,
		sendSem: make(chan struct{}, credits),
	}, nil
}

// txCredits reads the transmit window from Extra["txcredits"], defaulting to
// defaultTxCredits.
func txCredits(cfg gocan.Config) (int, error) {
	v, ok := cfg.Extra["txcredits"]
	if !ok || v == "" {
		return defaultTxCredits, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxTxCredits {
		return 0, fmt.Errorf("txcredits must be 1-%d (CAN transmit FIFO depth), got %q", maxTxCredits, v)
	}
	return n, nil
}

func (cu *CANUSB) Open(ctx context.Context, bus *gocan.Bus) error {
	cu.bus = bus
	if cu.port == nil {
		if cu.openPort == nil {
			cu.openPort = func() (serialPort, error) {
				p, err := serial.Open(cu.cfg.Port, &serial.Mode{
					BaudRate: cu.cfg.PortBaudrate,
					Parity:   serial.NoParity,
					DataBits: 8,
					StopBits: serial.OneStopBit,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to open com port %q: %w", cu.cfg.Port, err)
				}
				return p, nil
			}
		}
		p, err := cu.openPort()
		if err != nil {
			return err
		}
		cu.port = p
	}
	cu.port.SetReadTimeout(4 * time.Millisecond)

	// Setup sequence per manual §1.5: flush stale queue, probe version, force
	// timestamp off, set bit-rate, set acceptance filters. Channel stays
	// closed until we send O below (filters require closed-but-initiated).
	for _, c := range []string{"", "", "", "V", "N", "Z0", cu.canRate, cu.code, cu.mask} {
		if _, err := cu.port.Write(append([]byte(c), cr)); err != nil {
			cu.port.Close()
			return fmt.Errorf("canusb setup write failed: %w", err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	cu.port.ResetInputBuffer()

	go cu.readLoop(ctx)
	go cu.statusPoll(ctx)

	// Open the CAN channel.
	return cu.write([]byte{'O', cr})
}

// Close shuts the CAN channel and the port. cu.port is written once in Open
// and never nilled, so the read loop can race-freely use it until the closed
// port errors it out.
func (cu *CANUSB) Close() error {
	if cu.port == nil { // Open never ran or failed before assigning
		return nil
	}
	if _, err := cu.port.Write([]byte{'C', cr}); err != nil {
		return fmt.Errorf("canusb close write failed: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	cu.port.ResetInputBuffer()
	cu.port.ResetOutputBuffer()
	if err := cu.port.Close(); err != nil {
		return fmt.Errorf("failed to close com port: %w", err)
	}
	return nil
}

// Send encodes and writes one frame, blocking while the transmit window is
// full. It returns once the frame is written, not once the device acks it —
// the ack releases the credit for a later Send. The Bus serializes callers.
func (cu *CANUSB) Send(ctx context.Context, f gocan.Frame) error {
	select {
	case cu.sendSem <- struct{}{}:
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-cu.bus.Done():
		return gocan.ErrClosed
	}
	return cu.write(encode(cu.txBuf[:0], f))
}

// SetFilter reconfigures the SJA1000 acceptance filter at runtime. The
// channel must be closed to set M/m, so we bounce C -> M -> m -> O.
func (cu *CANUSB) SetFilter(filters []uint32) error {
	code, mask := acceptanceFilters(filters)
	for _, c := range []string{"C", code, mask, "O"} {
		if err := cu.write(append([]byte(c), cr)); err != nil {
			return err
		}
	}
	return nil
}

func (cu *CANUSB) write(b []byte) error {
	if cu.cfg.Debug {
		cu.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: ">> " + string(b)})
	}
	cu.writeMu.Lock()
	defer cu.writeMu.Unlock()
	if _, err := cu.port.Write(b); err != nil {
		err = fmt.Errorf("failed to write to com port: %w", err)
		cu.bus.Fatal(err)
		return err
	}
	return nil
}

// statusPoll asks for the status flags every second (manual §1.5 recommends
// 500-1000ms). The F reply also acks, recovering a lost transmit ack.
func (cu *CANUSB) statusPoll(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case cu.sendSem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			if cu.write(cmdStatus) != nil {
				return
			}
		}
	}
}

func (cu *CANUSB) readLoop(ctx context.Context) {
	read := make([]byte, 64)
	for {
		n, err := cu.port.Read(read)
		if err != nil {
			if ctx.Err() == nil { // dead port, not a shutdown
				cu.bus.Fatal(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		cu.parse(read[:n])
	}
}

// parse accumulates bytes into CR-terminated lines and dispatches them.
func (cu *CANUSB) parse(data []byte) {
	for _, b := range data {
		switch b {
		case bell: // command error: release the send semaphore
			cu.error(errors.New("command error (BELL)"))
			cu.ack()
		case cr:
			if len(cu.line) > 0 {
				cu.dispatch(cu.line)
			}
			cu.line = cu.line[:0]
		default:
			cu.line = append(cu.line, b)
		}
	}
}

func (cu *CANUSB) dispatch(line []byte) {
	if cu.cfg.Debug {
		cu.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "<< " + string(line)})
	}
	switch line[0] {
	case 't', 'r':
		cu.deliverFrame(line, false)
	case 'T', 'R':
		cu.deliverFrame(line, true)
	case 'z', 'Z': // transmit ack
		cu.ack()
	case 'F': // status reply (also a command ack)
		cu.ack()
		if err := decodeStatus(line[1:]); err != nil {
			cu.error(fmt.Errorf("CAN status error: %w", err))
		}
	case 'V':
		cu.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "CANUSB version " + string(line[1:])})
	case 'N':
		cu.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "CANUSB serial " + string(line[1:])})
	default:
		cu.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "Unknown>> " + string(line)})
	}
}

// ack releases one slot of the send semaphore (non-blocking).
func (cu *CANUSB) ack() {
	select {
	case <-cu.sendSem:
	default:
	}
}

func (cu *CANUSB) error(err error) {
	cu.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
}

// deliverFrame decodes a received frame line. Layout (timestamp off):
//
//	standard: t iii l dd..   (id 3 hex, dlc 1, data dlc*2 hex)
//	extended: T iiiiiiii l dd..
func (cu *CANUSB) deliverFrame(line []byte, extended bool) {
	idLen := 3
	if extended {
		idLen = 8
	}
	// 1 (type) + idLen + 1 (dlc) minimum
	if len(line) < 1+idLen+1 {
		cu.error(fmt.Errorf("short frame: %q", line))
		return
	}
	id, err := strconv.ParseUint(string(line[1:1+idLen]), 16, 32)
	if err != nil {
		cu.error(fmt.Errorf("bad identifier in %q: %w", line, err))
		return
	}
	dlc := int(line[1+idLen] - '0')
	if dlc < 0 || dlc > 8 {
		cu.error(fmt.Errorf("bad DLC in %q", line))
		return
	}
	body := line[2+idLen:]
	if len(body) < dlc*2 { // ignore any trailing bytes (e.g. timestamp)
		cu.error(fmt.Errorf("truncated data in %q", line))
		return
	}

	f := gocan.Frame{ID: uint32(id), Extended: extended, Length: uint8(dlc)}
	if _, err := hex.Decode(f.Data[:dlc], body[:dlc*2]); err != nil {
		cu.error(fmt.Errorf("bad data in %q: %w", line, err))
		return
	}
	f.Remote = line[0] == 'r' || line[0] == 'R'
	cu.bus.Deliver(f)
}

// encode appends the ASCII transmit command for a CAN frame to b.
func encode(b []byte, f gocan.Frame) []byte {
	cmd, id, digits := byte('t'), f.ID&0x7FF, 3
	if f.Extended {
		cmd, id, digits = 'T', f.ID&0x1FFFFFFF, 8
	}
	b = append(b, cmd)
	for i := digits - 1; i >= 0; i-- {
		b = append(b, "0123456789ABCDEF"[id>>(4*i)&0xF])
	}
	b = append(b, '0'+f.Length)
	b = hex.AppendEncode(b, f.Data[:f.Length])
	return append(b, cr)
}

// bitRate maps a CAN rate in kbit/s to the matching Sn command, or an sxxyy
// BTR0/BTR1 command for the non-standard rates this project uses.
func bitRate(rate float64) (string, error) {
	switch rate {
	case 10:
		return "S0", nil
	case 20:
		return "S1", nil
	case 33.3:
		return "s0e1c", nil // SWCAN / GMLAN
	case 47.619:
		return "scb9a", nil
	case 50:
		return "S2", nil
	case 100:
		return "S3", nil
	case 125:
		return "S4", nil
	case 250:
		return "S5", nil
	case 500:
		return "S6", nil
	case 615.384:
		return "s4037", nil
	case 800:
		return "S7", nil
	case 1000:
		return "S8", nil
	default:
		return "", fmt.Errorf("unsupported CAN rate: %g kbit/s", rate)
	}
}

// decodeStatus decodes the F status flags (2 hex chars). Returns the first
// error condition set, or nil. Bit map per manual §2.0 (F command).
func decodeStatus(b []byte) error {
	v, err := strconv.ParseUint(string(b), 16, 16)
	if err != nil {
		return fmt.Errorf("failed to decode status %q: %w", b, err)
	}
	flags := []struct {
		bit uint
		msg string
	}{
		{0, "CAN receive FIFO queue full"},
		{1, "CAN transmit FIFO queue full"},
		{2, "error warning (EI)"},
		{3, "data overrun (DOI)"},
		{5, "error passive (EPI)"},
		// bit 6 (arbitration lost, ALI) is omitted on purpose: it's normal bus
		// contention, auto-retransmitted, and the device doesn't even flag it
		// as a fault (manual §2.0: "doesn't generate a blinking RED light").
		{7, "bus error (BEI)"},
	}
	for _, f := range flags {
		if v&(1<<f.bit) != 0 {
			return errors.New(f.msg)
		}
	}
	return nil
}

// acceptanceFilters maps a CAN ID list to M/m commands for the SJA1000
// dual-filter layout, falling back to accept-everything when it cannot.
func acceptanceFilters(ids []uint32) (string, string) {
	code, mask, err := accept11(ids)
	if err != nil {
		return "M00000000", "mFFFFFFFF"
	}
	return fmt.Sprintf("M%02X%02X%02X%02X", code[0], code[1], code[2], code[3]),
		fmt.Sprintf("m%02X%02X%02X%02X", mask[0], mask[1], mask[2], mask[3])
}

// accept11 computes SJA1000 acceptance code/mask registers covering all
// given 11-bit IDs (a superset may pass; software filtering still applies).
func accept11(ids []uint32) (ac, am [4]byte, err error) {
	if len(ids) == 0 {
		return ac, am, errors.New("accept11: empty id slice")
	}

	const idMask = 0x7FF // 11-bit

	base := ids[0] & idMask
	var diff uint32
	for _, raw := range ids[1:] {
		if raw > idMask {
			return ac, am, fmt.Errorf("accept11: id 0x%X > 0x7FF (not 11-bit)", raw)
		}
		diff |= base ^ (raw & idMask)
	}

	// maskID: 1 where *all* IDs share the same bit, 0 where they differ
	maskID := (^diff) & idMask
	codeID := base & maskID

	// ---- Map into SJA1000 dual-standard filter 2 layout ----
	//
	// Filter 2 uses:
	//   AC2  = ACR2   = ID10..ID3  (8 bits)
	//   AC3[7:5]      = ID2..ID0
	//   AM2  = AMR2   = mask for ID10..ID3 (0 = care, 1 = don't care)
	//   AM3[7:5]      = mask for ID2..ID0  (0 = care, 1 = don't care)
	//   AM3[4]        = mask for RTR (we set to 1 to ignore RTR)
	//   AM3[3:0]      = data bits for filter 1; left 0 as in the manual example.
	id10_3 := byte((codeID >> 3) & 0xFF)
	id2_0 := byte(codeID & 0x7)
	mask10_3 := byte((maskID >> 3) & 0xFF)
	mask2_0 := byte(maskID & 0x7)

	ac[2] = id10_3
	ac[3] = id2_0 << 5

	// SJA1000 masks are inverted: 0 = care, 1 = don't care.
	am[2] = ^mask10_3
	am[3] = ^(mask2_0<<5)&0xE0 | 0x10 // ID2..0 mask, RTR = don't care

	return ac, am, nil
}
