package drewtech

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

// ErrNotSupported is returned by J2534 operations the Mongoose firmware or this
// native client does not implement (see PROTOCOL.md for why each is out).
var ErrNotSupported = errors.New("drewtech: operation not supported")

// J2534 IOCTL ids (SAE J2534-1).
const (
	GET_CONFIG                    uint32 = 0x01
	SET_CONFIG                    uint32 = 0x02
	READ_VBATT                    uint32 = 0x03
	FIVE_BAUD_INIT                uint32 = 0x04
	FAST_INIT                     uint32 = 0x05
	CLEAR_TX_BUFFER               uint32 = 0x07
	CLEAR_RX_BUFFER               uint32 = 0x08
	CLEAR_PERIODIC_MSGS           uint32 = 0x09
	CLEAR_MSG_FILTERS             uint32 = 0x0A
	CLEAR_FUNCT_MSG_LOOKUP_TABLE  uint32 = 0x0B
	ADD_TO_FUNCT_MSG_LOOKUP_TABLE uint32 = 0x0C
	DELETE_FROM_FUNCT_MSG_LOOKUP  uint32 = 0x0D
	READ_PROG_VOLTAGE             uint32 = 0x0E
)

// J2534 filter types (SAE J2534-1) for PassThruStartMsgFilter.
const (
	PASS_FILTER         uint32 = 0x01
	BLOCK_FILTER        uint32 = 0x02
	FLOW_CONTROL_FILTER uint32 = 0x03
)

// VersionInfo is what PassThruReadVersion reports.
type VersionInfo struct {
	Firmware   string // device firmware, e.g. "1.2.4.0"
	Bootloader string // device bootloader
	Serial     string // device serial number
	DLL        string // reference DLL version string
	API        string // J2534 API version the reference DLL implements
}

// -------- 1. PassThruOpen --------

// PassThruOpen runs the full device initialization handshake (matching the
// DLL's open sequence) and caches firmware/serial info for PassThruReadVersion.
func (d *Device) PassThruOpen() error {
	if err := d.init(); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	info, err := d.getDeviceInfo(0x003C)
	if err != nil {
		return fmt.Errorf("get device info: %w", err)
	}
	d.info = info
	if err := d.getConfig(0x2F); err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if err := d.getStatus(); err != nil {
		return fmt.Errorf("get status: %w", err)
	}
	if _, err := d.getFirmwareVersion(); err != nil {
		return fmt.Errorf("get firmware: %w", err)
	}
	if err := d.sendMagic(); err != nil {
		return fmt.Errorf("send magic: %w", err)
	}
	if _, err := d.getDeviceInfo(0xFFFF); err != nil {
		return fmt.Errorf("get device info ffff: %w", err)
	}
	serial, err := d.getSerial()
	if err != nil {
		return fmt.Errorf("get serial: %w", err)
	}
	d.serial = serial
	return nil
}

// -------- 2. PassThruClose --------

// PassThruClose sends the wire close command and tears down the transport.
func (d *Device) PassThruClose() error {
	payload := make([]byte, 8)
	payload[0] = SubCmdClose
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], 0x4C49)
	// Best-effort: even if the device doesn't ack, still close the port.
	_, werr := d.sendAndWait(cmd(0, payload), time.Second)
	cerr := d.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// -------- 3. PassThruConnect --------

// PassThruConnect connects the CAN channel at the given bit rate and selects the
// CAN protocol. The device accepts off-table rates (e.g. 615384); the rate
// whitelist lived only in the Windows DLL, which this client bypasses.
func (d *Device) PassThruConnect(baudRate uint32) error {
	d.channel = canChannel
	payload := make([]byte, 16)
	payload[0] = SubCmdConnect
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[12:16], baudRate)
	if _, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second); err != nil {
		return err
	}

	// Set protocol (CAN).
	p2 := make([]byte, 20)
	p2[0] = SubCmdSetProtocol
	p2[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(p2[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(p2[8:12], 0x01)
	binary.LittleEndian.PutUint32(p2[12:16], ProtocolCAN)
	binary.LittleEndian.PutUint32(p2[16:20], 0x0E)
	_, err := d.sendAndWait(cmd(d.channel, p2), 2*time.Second)
	return err
}

// -------- 4. PassThruDisconnect --------

// PassThruDisconnect disconnects the channel from the bus.
func (d *Device) PassThruDisconnect() error {
	d.stopAllPeriodic()
	payload := make([]byte, 8)
	payload[0] = SubCmdDisconnect
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	_, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second)
	return err
}

// -------- 5. PassThruReadMsgs --------

// PassThruReadMsgs drains up to max frames from the host-side RX FIFO, waiting
// up to timeout for the first one. There is no wire "read" command: the device
// pushes 0x09 frames unsolicited and the read loop queues them.
func (d *Device) PassThruReadMsgs(max int, timeout time.Duration) ([]*CANFrame, error) {
	if d.overflow.Swap(false) {
		// Report loss but still return whatever is queued.
		defer d.handleError(errors.New("rx buffer overflowed, messages were lost"))
	}
	if max <= 0 {
		max = 1
	}
	out := make([]*CANFrame, 0, max)
	deadline := time.After(timeout)
	for len(out) < max {
		// Drain what is already queued first: per J2534, a zero/expired
		// timeout still returns the messages received so far.
		select {
		case f := <-d.rxQueue:
			out = append(out, f)
			continue
		default:
		}
		if len(out) > 0 {
			return out, nil // don't block once we have something
		}
		select {
		case f := <-d.rxQueue:
			out = append(out, f)
		case <-deadline:
			return out, nil
		case <-d.closeCh:
			return out, errors.New("device closed")
		}
	}
	return out, nil
}

// -------- 6. PassThruWriteMsgs --------

// PassThruWriteMsgs transmits a CAN frame (fire-and-forget; the device replies
// with 0x08/0x0A confirmations that the read loop consumes). The wire frame is
// the fixed-size 0x24 layout the DLL sends: payload padded to 32 bytes, actual
// data length carried at [16:18].
func (d *Device) PassThruWriteMsgs(canID uint32, data []byte) error {
	if len(data) > 8 {
		return fmt.Errorf("invalid message: %d data bytes, CAN max is 8", len(data))
	}
	payload := make([]byte, 32)
	payload[0] = SubCmdCANTx
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[4:6], 0x0001)
	binary.LittleEndian.PutUint16(payload[16:18], uint16(4+len(data))) // id + data
	binary.BigEndian.PutUint32(payload[20:24], canID)
	copy(payload[24:], data)
	return d.send(cmd(d.channel, payload))
}

// -------- 7 & 8. Periodic messages (host-side) --------

type periodicMsg struct {
	id       uint32
	canID    uint32
	data     []byte
	interval time.Duration
	stop     chan struct{}
}

// PassThruStartPeriodicMsg sends canID/data every interval and returns a message
// id for PassThruStopPeriodicMsg. Implemented host-side (a ticker calling
// WriteMsgs); the firmware has no periodic-message wire command.
func (d *Device) PassThruStartPeriodicMsg(canID uint32, data []byte, interval time.Duration) (uint32, error) {
	if interval <= 0 {
		return 0, fmt.Errorf("interval must be > 0")
	}
	id := atomic.AddUint32(&d.periodicSeq, 1)
	pm := &periodicMsg{
		id:       id,
		canID:    canID,
		data:     append([]byte(nil), data...),
		interval: interval,
		stop:     make(chan struct{}),
	}
	d.mu.Lock()
	d.periodics[id] = pm
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := d.PassThruWriteMsgs(pm.canID, pm.data); err != nil {
					d.handleError(fmt.Errorf("periodic %d: %w", id, err))
				}
			case <-pm.stop:
				return
			case <-d.closeCh:
				return
			}
		}
	}()
	return id, nil
}

// PassThruStopPeriodicMsg stops a periodic message by id.
func (d *Device) PassThruStopPeriodicMsg(id uint32) error {
	d.mu.Lock()
	pm, ok := d.periodics[id]
	if ok {
		delete(d.periodics, id)
	}
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("no periodic message %d", id)
	}
	close(pm.stop)
	return nil
}

func (d *Device) stopAllPeriodic() {
	d.mu.Lock()
	pms := make([]*periodicMsg, 0, len(d.periodics))
	for id, pm := range d.periodics {
		pms = append(pms, pm)
		delete(d.periodics, id)
	}
	d.mu.Unlock()
	for _, pm := range pms {
		close(pm.stop)
	}
}

// -------- 9. PassThruStartMsgFilter --------

// PassThruStartMsgFilter installs a message filter and returns its id. A PASS
// filter per CAN id is required: with none installed you receive nothing. Pass
// mask 0xFFFFFFFF for an exact match on pattern. Only PASS_FILTER is
// implemented: BLOCK is wire kind 2 in the same message (PROTOCOL.md §8) but
// untested on hardware, and FLOW_CONTROL needs the ISO15765 3-message variant.
func (d *Device) PassThruStartMsgFilter(filterType, pattern, mask uint32) (uint32, error) {
	if filterType != PASS_FILTER {
		return 0, fmt.Errorf("filter type 0x%X: %w", filterType, ErrNotSupported)
	}
	payload := make([]byte, 24)
	payload[0] = SubCmdStartMsgFilter
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	payload[14] = 0x01 // filter kind = PASS
	payload[15] = 0x04 // pattern length (4 for CAN)
	binary.BigEndian.PutUint32(payload[16:20], mask)
	binary.BigEndian.PutUint32(payload[20:24], pattern)
	resp, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second)
	if err != nil {
		return 0, err
	}
	id := uint32(0)
	if len(resp.Payload) >= 16 {
		id = binary.LittleEndian.Uint32(resp.Payload[12:16])
	}
	d.mu.Lock()
	d.filters = append(d.filters, id)
	d.mu.Unlock()
	return id, nil
}

// -------- 10. PassThruStopMsgFilter --------

// PassThruStopMsgFilter removes a filter by id.
func (d *Device) PassThruStopMsgFilter(filterID uint32) error {
	payload := make([]byte, 12)
	payload[0] = SubCmdClearFilters
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint16(payload[6:8], uint16(filterID&0xFFFF))
	if _, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second); err != nil {
		return err
	}
	d.mu.Lock()
	for i, id := range d.filters {
		if id == filterID {
			d.filters = append(d.filters[:i], d.filters[i+1:]...)
			break
		}
	}
	d.mu.Unlock()
	return nil
}

// -------- 11. PassThruSetProgrammingVoltage --------

// PassThruSetProgrammingVoltage is not implemented: the wire encoding is buried
// in the DLL's per-pin C++ dispatch and untestable without hardware, and wrong
// bytes on a voltage pin are a hardware risk. See PROTOCOL.md.
func (d *Device) PassThruSetProgrammingVoltage(pin, millivolts uint32) error {
	return fmt.Errorf("PassThruSetProgrammingVoltage: %w", ErrNotSupported)
}

// -------- 12. PassThruReadVersion --------

// PassThruReadVersion returns cached firmware/bootloader/serial (from
// PassThruOpen) plus the reference DLL/API version strings.
func (d *Device) PassThruReadVersion() VersionInfo {
	return VersionInfo{
		Firmware:   d.info.FW.String(),
		Bootloader: d.info.BL.String(),
		Serial:     d.serial,
		DLL:        "MongoosePro GM II J2534 Library (native gocan client)",
		API:        "04.04",
	}
}

// -------- 13. PassThruGetLastError --------

// PassThruGetLastError returns the most recent error observed by the device
// (read loop, tx faults, rx overflow), or "" if none.
func (d *Device) PassThruGetLastError() string {
	if e, ok := d.lastErr.Load().(error); ok && e != nil {
		return e.Error()
	}
	return ""
}

// -------- 14. PassThruIoctl --------

// PassThruIoctl dispatches the standard J2534 ioctls. For GET_CONFIG, param is
// the config id and the returned value is the config value; other ioctls ignore
// param and return 0.
func (d *Device) PassThruIoctl(ioctlID, param uint32) (uint32, error) {
	switch ioctlID {
	case GET_CONFIG:
		return d.GetChannelConfig(param)
	case CLEAR_TX_BUFFER:
		return 0, d.control(ctlClearTxBuffer)
	case CLEAR_RX_BUFFER:
		d.drainRxQueue()
		return 0, d.control(ctlClearRxBuffer)
	case CLEAR_PERIODIC_MSGS:
		d.stopAllPeriodic()
		return 0, nil
	case CLEAR_MSG_FILTERS:
		return 0, d.ClearMsgFilters()
	case SET_CONFIG, READ_VBATT, FIVE_BAUD_INIT, FAST_INIT, READ_PROG_VOLTAGE,
		CLEAR_FUNCT_MSG_LOOKUP_TABLE, ADD_TO_FUNCT_MSG_LOOKUP_TABLE, DELETE_FROM_FUNCT_MSG_LOOKUP:
		return 0, fmt.Errorf("ioctl 0x%X: %w", ioctlID, ErrNotSupported)
	default:
		return 0, fmt.Errorf("ioctl 0x%X: %w", ioctlID, ErrNotSupported)
	}
}

// -------- 15. PassThruFirmwareUpdate --------

// PassThruFirmwareUpdate is intentionally not implemented. The reflash images
// live encrypted in the DLL's RCDATA resources and the streaming protocol is
// untested here; a wrong write can brick the dongle. See PROTOCOL.md.
func (d *Device) PassThruFirmwareUpdate(image []byte) error {
	return fmt.Errorf("PassThruFirmwareUpdate: %w", ErrNotSupported)
}

// -------- 16. PassThruReadDetails --------

// PassThruReadDetails returns the cached version/serial details of the open
// device (there is no separate wire query; PassThruOpen already fetched them).
func (d *Device) PassThruReadDetails() VersionInfo {
	return d.PassThruReadVersion()
}

// -------- 17. PassThruSetIncomingMsgCallback --------

// PassThruSetIncomingMsgCallback registers a callback invoked for every RX
// frame (in addition to the FIFO and any WithCANChannel). Safe to call while
// the read loop is running.
func (d *Device) PassThruSetIncomingMsgCallback(cb func(*CANFrame)) {
	d.mu.Lock()
	d.onCANFrame = cb
	d.mu.Unlock()
}

// -------- 18. PassThruGetNextCarDAQ --------

// PassThruGetNextCarDAQ enumerates candidate device serial ports. The J2534 DLL
// discovers Mongoose units over USB/SetupDi; here we return the OS serial-port
// list for the caller to pick from.
func PassThruGetNextCarDAQ() ([]string, error) {
	return serial.GetPortsList()
}

// -------- config + filter helpers (back Ioctl) --------

// GetChannelConfig reads a config param from the connected channel. The reply
// echoes the param at [16:20] and returns the value at [20:24]. Only DATA_RATE
// (0x04), BIT_SAMPLE_POINT (0x14) and SYNC_JUMP_WIDTH (0x15) are supported;
// there is no runtime set (0x0C is get-only). See PROTOCOL.md.
func (d *Device) GetChannelConfig(param uint32) (uint32, error) {
	payload := make([]byte, 12)
	payload[0] = SubCmdGetConfig
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], param)
	resp, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second)
	if err != nil {
		return 0, err
	}
	if len(resp.Payload) >= 24 {
		return binary.LittleEndian.Uint32(resp.Payload[20:24]), nil
	}
	return 0, nil
}

// ClearMsgFilters removes all installed filters (wire op 0x10 subtype 2).
func (d *Device) ClearMsgFilters() error {
	payload := make([]byte, 12)
	payload[0] = SubCmdClearFilters
	payload[1] = FlagRequestFinal
	binary.LittleEndian.PutUint16(payload[2:4], d.nextSeq())
	binary.LittleEndian.PutUint32(payload[8:12], filClearAll)
	if _, err := d.sendAndWait(cmd(d.channel, payload), 2*time.Second); err != nil {
		return err
	}
	d.mu.Lock()
	d.filters = nil
	d.mu.Unlock()
	return nil
}

// BecomeMaster issues the "become master" control command (0x11 subtype 4).
func (d *Device) BecomeMaster(pollID byte) error {
	return d.control(ctlBecomeMaster, pollID)
}

func (d *Device) drainRxQueue() {
	for {
		select {
		case <-d.rxQueue:
		default:
			return
		}
	}
}
