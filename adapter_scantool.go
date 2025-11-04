package gocan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"
)

const (
	OBDLinkSX = "OBDLink SX"
	OBDLinkEX = "OBDLink EX"
	STN1170   = "STN1170"
	STN2120   = "STN2120"
)

var scantoolBaudrates = [...]uint{115200, 38400, 230400, 921600, 2000000, 1000000, 57600}

func scantoolInit(debug bool, port io.Writer, protocolCMD, canrateCMD, filter, mask string, onMessage func(string)) {
	var initCmds = []string{
		"ATE0",      // turn off echo
		"STUFC0",    // Turn on flow control
		"ATS0",      // turn off spaces
		"ATV1",      // variable DLC on
		protocolCMD, // Set canbus protocol
		"ATH1",      // Headers on
		"ATAT0",     // Set adaptive timing mode off
		"ATCAF0",    // Automatic formatting off
		canrateCMD,  // Set CANrate
		//"ATAL",          // Allow long messages
		"ATCFC0", //Turn automatic CAN flow control off
		//"ATAR",      // Automatically set the receive address.
		//"ATCSM0",  //Turn CAN silent monitoring off
		//"STCMM1",   // Set CAN monitor monitor - Normal node – with CAN ACKs
		"ATST32", // Set timeout to 200msec
		"ATR0",   // Turn off replies
		mask,     // mask
		filter,   // code
	}

	delay := 10 * time.Millisecond

	for _, cmd := range initCmds {
		if cmd == "" {
			continue
		}
		out := []byte(cmd + "\r")
		if debug {
			onMessage(">> " + cmd)
		}
		if _, err := port.Write(out); err != nil {
			onMessage(err.Error())
		}
		time.Sleep(delay)
	}
}

func scantoolReset(port io.Writer) {
	time.Sleep(25 * time.Millisecond)
	port.Write([]byte("ATZ\r"))
	time.Sleep(10 * time.Millisecond)
}

func scantoolSetFilter(base *BaseAdapter, mask, filter string) error {
	base.Send() <- NewFrame(SystemMsg, []byte("STPC"), Outgoing)
	base.Send() <- NewFrame(SystemMsg, []byte(mask), Outgoing)
	base.Send() <- NewFrame(SystemMsg, []byte(filter), Outgoing)
	base.Send() <- NewFrame(SystemMsg, []byte("STPO"), Outgoing)
	return nil
}

func scantoolCANfilter(ids []uint32) (filterStr string, maskStr string) {
	var filt uint32 = 0xFFF
	var mask uint32 = 0x000
	for _, id := range ids {
		filt &= id
		mask |= id
	}
	mask = (^mask & 0x7FF) | filt
	if len(ids) == 0 {
		filt = 0
		mask = 0x7FF
	}
	filterStr = fmt.Sprintf("ATCF%03X", filt)
	maskStr = fmt.Sprintf("ATCM%03X", mask)
	return
}

func scantoolDecodeFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[:3]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}
	data, err := hex.DecodeString(string(buff[3:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}
	return NewFrame(uint32(id), data, Incoming), nil
}

func scantoolCalculateCANrate(baseName string, rate float64) (string, string, error) {
	var protocolCMD string
	var canrateCMD string

	switch rate {
	case 33.3: // STN1170 & STN2120 feature only
		protocolCMD = "STP61"
		canrateCMD = "STCSWM2"
	case 500:
		protocolCMD = "STP33"
	case 615.384:
		protocolCMD = "STP33"
		switch baseName {
		case OBDLinkSX, STN1170:
			canrateCMD = "STCTR8101FC"
		case OBDLinkEX, STN2120:
			//canrateCMD = "STCTR42039F" // orig
			canrateCMD = "STCTR82239F" // verkar funka bäst so far

			//canrateCMD = "STCTR42439F" // x
			//canrateCMD = "STCTR82039F" // x
			//canrateCMD = "STCTR82439F" // bästa hittills?
			//canrateCMD = "STCTR82839F" // ännu bättre?
			//canrateCMD = "STCTRC2039F" // x

		default:
			return "", "", fmt.Errorf("unhandled adapter: %s", baseName)
		}
	default:
		return "", "", fmt.Errorf("unhandled CANBus rate: %f", rate)
	}
	return protocolCMD, canrateCMD, nil
}

func scantoolSendManager(
	ctx context.Context,
	debug bool,
	port io.Writer,
	sendChan <-chan *CANFrame,
	sendSem chan<- struct{},
	closeChan chan struct{},
	onError func(error),
	onMessage func(string),

) {
	defer log.Println("exit scantoolSendManager")
	f := bytes.NewBuffer(nil)
	idb := make([]byte, 4)
	for {
		select {
		case <-ctx.Done():
			return
		case <-closeChan:
			return
		case v := <-sendChan:
			if id := v.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if debug {
						onMessage("<o> " + f.String())
					}
					sendSem <- struct{}{}
					if _, err := port.Write(append(v.Data, '\r')); err != nil {
						onError(fmt.Errorf("failed to write: %q %w", f.String(), err))
						return
					}
				}
				continue
			}
			binary.BigEndian.PutUint32(idb, v.Identifier)
			f.WriteString("STPXh:" + hex.EncodeToString(idb)[5:] + ",d:" + hex.EncodeToString(v.Data))
			if v.Timeout != 0 && v.Timeout != 200 {
				f.WriteString(",t:" + strconv.Itoa(int(v.Timeout)))
			}
			// timeout = 0
			respCount := v.FrameType.Responses
			if respCount > 0 {
				f.WriteString(",r:" + strconv.Itoa(respCount))
			}
			f.WriteString("\r")
			if debug {
				onMessage("<o> " + f.String())
			}
			sendSem <- struct{}{}
			if _, err := port.Write(f.Bytes()); err != nil {
				onError(fmt.Errorf("failed to write: %q %w", f.String(), err))
				return
			}
			f.Reset()
		}
	}
}

const (
	sleepAfterBreak = 20 * time.Millisecond
	sleepAfterSTBR  = 10 * time.Millisecond
	sleepWhenIdle   = 4 * time.Millisecond
	readAttempts    = 10
)

func scantoolTrySpeed(
	port io.ReadWriter,
	from, to uint,
	speedSetter func(int) error,
	resetInputBuffer func() error,
	onMessage func(string),
) error {
	cr3 := []byte{'\r', '\r', '\r'}
	stn := []byte("STN")

	// Set initial baud and send triple break
	if err := speedSetter(int(from)); err != nil {
		return err
	}
	if _, err := port.Write(cr3); err != nil {
		return err
	}
	time.Sleep(sleepAfterBreak)

	// Send "STBR<to>\r" with minimal allocs
	cmd := append([]byte("STBR"), strconv.AppendInt(nil, int64(to), 10)...)
	cmd = append(cmd, '\r')
	if _, err := port.Write(cmd); err != nil {
		return err
	}
	time.Sleep(sleepAfterSTBR)

	// Switch to new baud
	if err := resetInputBuffer(); err != nil {
		return err
	}
	if err := speedSetter(int(to)); err != nil {
		return err
	}

	readBuf := make([]byte, 64)
	lineBuf := make([]byte, 0, 128)

	for range readAttempts {
		n, err := port.Read(readBuf)
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(sleepWhenIdle)
			continue
		}

		for _, b := range readBuf[:n] {
			if b == '\r' {
				if len(lineBuf) == 0 {
					continue
				}
				if bytes.Contains(lineBuf, stn) {
					onMessage("Device info: " + string(lineBuf))
					if _, err := port.Write([]byte{'\r'}); err != nil {
						return err
					}
					return nil
				}
				lineBuf = lineBuf[:0]
				continue
			}

			if len(lineBuf) == cap(lineBuf) {
				newCap := max(cap(lineBuf)*2, 64)
				tmp := make([]byte, len(lineBuf), newCap)
				copy(tmp, lineBuf)
				lineBuf = tmp
			}
			lineBuf = append(lineBuf, b)
		}
	}

	return fmt.Errorf("failed to change adapter baudrate from %d to %d bps", from, to)
}

func equalBytesString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}
