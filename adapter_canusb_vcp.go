package gocan

// Lawicel CANUSB driver, implemented from the CANUSB Manual (Version 1.0D,
// November 2011). It speaks the ASCII command set over the FTDI Virtual COM
// Port (VCP). This is a clean reimplementation living alongside the older
// "CANUSB VCP" adapter; the two do not share runtime code.
//
// Command set used (all commands end with CR, ASCII 13, and are case sensitive):
//
//	Sn          set standard bit-rate (S0..S8)
//	sxxyy       set bit-rate via BTR0/BTR1 (used for non-standard rates)
//	O / C       open / close the CAN channel
//	tiiildd..   transmit standard (11-bit) frame, ack: z
//	Tiiiiiiiildd.. transmit extended (29-bit) frame, ack: Z
//	Mxxxxxxxx   acceptance code   (channel initiated, not open)
//	mxxxxxxxx   acceptance mask    (channel initiated, not open)
//	V / N       hardware+software version / serial number
//	Zn          received-frame timestamp on/off (we keep it OFF)
//	F           read & clear status flags (only while open)
//
// Replies: CR (OK) / BELL 0x07 (error) for setup commands, z/Z for transmit
// acks, and unsolicited t.../T... lines for received frames.

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.bug.st/serial"
)

const (
	cr   = 0x0D // command terminator / OK
	bell = 0x07 // error reply
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "CANUSB VCP",
		Description:        "Lawicel CANUSB (VCP, manual 1.0D)",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: true,
		},
		New: NewCanusb,
	}); err != nil {
		panic(err)
	}
}

type CanusbVCP struct {
	*BaseAdapter
	port    serial.Port
	canRate string // S/s command for the configured bit-rate
	code    string // M acceptance-code command
	mask    string // m acceptance-mask command
	buff    *bytes.Buffer
	sendSem chan struct{} // one outstanding command at a time (manual §1.4/1.5)
}

func NewCanusb(cfg *AdapterConfig) (Adapter, error) {
	rate, err := canusbBitRate(cfg.CANRate)
	if err != nil {
		return nil, err
	}
	code, mask := canusbAcceptanceFilters(cfg.CANFilter)
	return &CanusbVCP{
		BaseAdapter: NewBaseAdapter("CANUSB", cfg),
		canRate:     rate,
		code:        code,
		mask:        mask,
		buff:        bytes.NewBuffer(nil),
		sendSem:     make(chan struct{}, 1),
	}, nil
}

// canusbBitRate maps a CAN rate in kbit/s to the matching Sn command, or an
// sxxyy BTR0/BTR1 command for the non-standard rates this project uses.
func canusbBitRate(rate float64) (string, error) {
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

func (cu *CanusbVCP) Open(ctx context.Context) error {
	p, err := serial.Open(cu.cfg.Port, &serial.Mode{
		BaudRate: cu.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("failed to open com port %q: %w", cu.cfg.Port, err)
	}
	p.SetReadTimeout(4 * time.Millisecond)
	cu.port = p

	// Setup sequence per manual §1.5: flush stale queue, probe version, force
	// timestamp off, set bit-rate, set acceptance filters. Channel stays
	// closed until we send O below (filters require closed-but-initiated).
	for _, c := range []string{"\r", "\r", "\r", "V", "N", "Z0", cu.canRate, cu.code, cu.mask} {
		if _, err := p.Write([]byte(c + "\r")); err != nil {
			p.Close()
			return fmt.Errorf("canusb setup write failed: %w", err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	p.ResetInputBuffer()

	go cu.recvManager(ctx)
	go cu.sendManager(ctx)

	// Open the channel.
	cu.sendChan <- &CANFrame{Identifier: SystemMsg, Data: []byte("O")}
	return nil
}

func (cu *CanusbVCP) Close() error {
	cu.BaseAdapter.Close()
	if cu.port == nil {
		return nil
	}
	if _, err := cu.port.Write([]byte("C\r")); err != nil {
		return fmt.Errorf("canusb close write failed: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	cu.port.ResetInputBuffer()
	cu.port.ResetOutputBuffer()
	err := cu.port.Close()
	cu.port = nil
	if err != nil {
		return fmt.Errorf("failed to close com port: %w", err)
	}
	return nil
}

// SetFilter reconfigures the SJA1000 acceptance filter at runtime. The channel
// must be closed to set M/m, so we bounce C -> M -> m -> O.
func (cu *CanusbVCP) SetFilter(filters []uint32) error {
	code, mask := canusbAcceptanceFilters(filters)
	for _, c := range []string{"C", code, mask, "O"} {
		cu.sendChan <- &CANFrame{Identifier: SystemMsg, Data: []byte(c)}
	}
	return nil
}

func (cu *Canusb) recvManager(ctx context.Context) {
	read := make([]byte, 64)
	for {
		select {
		case <-ctx.Done():
			return
		case <-cu.closeChan:
			return
		default:
		}
		n, err := cu.port.Read(read)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			cu.Fatal(fmt.Errorf("failed to read com port: %w", err))
			return
		}
		if n > 0 {
			cu.parse(read[:n])
		}
	}
}

// parse accumulates bytes into CR-terminated lines and dispatches them.
func (cu *CanusbVCP) parse(data []byte) {
	for _, b := range data {
		switch b {
		case bell: // command error: release the send semaphore
			cu.Error(errors.New("command error (BELL)"))
			cu.ack()
		case cr:
			if cu.buff.Len() > 0 {
				cu.dispatch(cu.buff.Bytes())
			}
			cu.buff.Reset()
		default:
			cu.buff.WriteByte(b)
		}
	}
}

func (cu *CanusbVCP) dispatch(line []byte) {
	if cu.cfg.Debug {
		cu.Debug("<< " + string(line))
	}
	switch line[0] {
	case 't', 'r':
		cu.emitFrame(line, false)
	case 'T', 'R':
		cu.emitFrame(line, true)
	case 'z', 'Z': // transmit ack
		cu.ack()
	case 'F': // status reply (also a command ack)
		cu.ack()
		if err := canusbStatus(line[1:]); err != nil {
			cu.Error(fmt.Errorf("CAN status error: %w", err))
		}
	case 'V':
		if cu.cfg.PrintVersion {
			cu.Info("CANUSB version " + string(line[1:]))
		}
	case 'N':
		if cu.cfg.PrintVersion {
			cu.Info("CANUSB serial " + string(line[1:]))
		}
	default:
		cu.Warn("Unknown>> " + string(line))
	}
}

// ack releases one slot of the send semaphore (non-blocking).
func (cu *CanusbVCP) ack() {
	select {
	case <-cu.sendSem:
	default:
	}
}

// emitFrame decodes a received frame line. Layout (timestamp off):
//
//	standard: t iii l dd..   (id 3 hex, dlc 1, data dlc*2 hex)
//	extended: T iiiiiiii l dd..
func (cu *CanusbVCP) emitFrame(line []byte, extended bool) {
	idLen := 3
	if extended {
		idLen = 8
	}
	// 1 (type) + idLen + 1 (dlc) minimum
	if len(line) < 1+idLen+1 {
		cu.Error(fmt.Errorf("short frame: %q", line))
		return
	}
	id, err := strconv.ParseUint(string(line[1:1+idLen]), 16, 32)
	if err != nil {
		cu.Error(fmt.Errorf("bad identifier in %q: %w", line, err))
		return
	}
	dlc := int(line[1+idLen] - '0')
	if dlc < 0 || dlc > 8 {
		cu.Error(fmt.Errorf("bad DLC in %q", line))
		return
	}
	body := line[2+idLen:]
	if len(body) < dlc*2 { // ignore any trailing bytes (e.g. timestamp)
		cu.Error(fmt.Errorf("truncated data in %q", line))
		return
	}
	data := make([]byte, dlc)
	if _, err := hex.Decode(data, body[:dlc*2]); err != nil {
		cu.Error(fmt.Errorf("bad data in %q: %w", line, err))
		return
	}

	var f *CANFrame
	if extended {
		f = NewExtendedFrame(uint32(id), data, Incoming)
	} else {
		f = NewFrame(uint32(id), data, Incoming)
	}
	f.RTR = line[0] == 'r' || line[0] == 'R'

	select {
	case cu.recvChan <- f:
	default:
		cu.Error(ErrDroppedFrame)
	}
}

func (cu *CanusbVCP) sendManager(ctx context.Context) {
	ticker := time.NewTicker(time.Second) // poll status (manual §1.5: 500-1000ms)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cu.closeChan:
			return
		case <-ticker.C:
			cu.sendSem <- struct{}{}
			cu.write([]byte("F\r"))
		case msg := <-cu.sendChan:
			if msg.Identifier >= SystemMsg {
				// System commands (O, C, M, m, ...) get a bare-CR ack we
				// don't gate on; only CAN frames hold the send semaphore.
				if msg.Identifier == SystemMsg {
					cu.write(append(msg.Data, cr))
				}
				continue
			}
			cu.sendSem <- struct{}{}
			cu.write(canusbEncode(msg))
		}
	}
}

func (cu *CanusbVCP) write(b []byte) {
	if cu.cfg.Debug {
		cu.Debug(">> " + string(b))
	}
	if _, err := cu.port.Write(b); err != nil {
		cu.Fatal(fmt.Errorf("failed to write to com port: %w", err))
	}
}

// canusbEncode builds the ASCII transmit command for a CAN frame.
func canusbEncode(msg *CANFrame) []byte {
	dlc := msg.DLC()
	if dlc > 8 {
		dlc = 8
	}
	var b []byte
	if msg.Extended {
		b = append(b, 'T')
		b = append(b, fmt.Sprintf("%08X", msg.Identifier&0x1FFFFFFF)...)
	} else {
		b = append(b, 't')
		b = append(b, fmt.Sprintf("%03X", msg.Identifier&0x7FF)...)
	}
	b = append(b, '0'+byte(dlc))
	b = append(b, []byte(hex.EncodeToString(msg.Data[:dlc]))...)
	return append(b, cr)
}

// canusbStatus decodes the F status flags (2 BCD/hex chars). Returns the first
// error condition set, or nil. Bit map per manual §2.0 (F command).
func canusbStatus(b []byte) error {
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
