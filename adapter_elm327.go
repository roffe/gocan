package gocan

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"go.bug.st/serial"
)

/*
THIS IS EXPERIMENTAL DRIVER AND MAY NOT WORK AS EXPECTED
*/

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "ELM327",
		Description:        "ELM327 CANBus Adapter",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: false,
		},
		New: NewELM327,
	}); err != nil {
		panic(err)
	}
}

type ELM327 struct {
	*BaseAdapter
	port serial.Port

	currentID uint32
	response  bool
}

const (
	ELM327_OK      = "OK\r\r"
	ELM327_UNKNOWN = "?\r\r"
)

func NewELM327(cfg *AdapterConfig) (Adapter, error) {
	el := &ELM327{
		BaseAdapter: NewBaseAdapter("ELM327", cfg),
	}
	return el, nil
}

func (el *ELM327) Open(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: el.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(el.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", el.cfg.Port, err)
	}
	el.port = p

	p.SetReadTimeout(10 * time.Millisecond)
	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	if err := el.init(); err != nil {
		el.port.Close()
		return fmt.Errorf("failed to init ELM327: %w", err)
	}

	go el.run(ctx)

	return nil
}

func (el *ELM327) Close() error {
	log.Println("Close ELM327")
	el.BaseAdapter.Close()
	if el.port != nil {
		el.port.Write([]byte("ATZ\r"))
		time.Sleep(100 * time.Millisecond)
		el.port.Write([]byte("ATZ\r"))
		time.Sleep(100 * time.Millisecond)
		el.port.Close()
		el.port = nil
	}
	return nil
}

func (el *ELM327) init() error {
	el.writePort("ATZ")
	time.Sleep(1 * time.Second)
	el.port.ResetInputBuffer()

	commands := []string{
		"ATE0", // Echo off
		"ATS0", // Spaces off
		"ATL0", // Linefeeds off
		// "ATI",    // Identify
		// "AT@1",   // Show device description
		"ATAL",    // Allow long messages
		"ATSP6",   // Set protocol to CAN 11 bit ID 500 kbps
		"ATH1",    // Headers on
		"ATAT2",   // Adaptive Timing
		"ATV1",    // Variable DLC on
		"ATR0",    // Responses off
		"ATAR",    // Automatic receive
		"ATCAF0",  // Automatic formatting off
		"ATCFC0",  // CAN flow control off
		"ATBRT28", // Set baud rate switch timeout to 40 ms
		"ATST32",  // Set read timeout to 200ms (hh*4ms) (32h = 50)
	}

	for _, cmd := range commands {
		resp, err := el.sendCommand(cmd)
		if err != nil {
			return fmt.Errorf("error sending command %q: %w", cmd, err)
		}
		if !strings.HasSuffix(resp, ELM327_OK) {
			return fmt.Errorf("error sending command %q: %q", cmd, resp)
		}
	}

	if err := el.setFilter(el.cfg.CANFilter); err != nil {
		return fmt.Errorf("failed to set filter: %w", err)
	}

	if el.cfg.PortBaudrate != 500000 {
		if err := el.changeDeviceBaudrate(el.cfg.PortBaudrate, 500000); err != nil {
			return fmt.Errorf("failed to change speed: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	el.port.ResetInputBuffer()
	el.port.ResetOutputBuffer()

	return nil
}

func (el *ELM327) run(ctx context.Context) {
	defer log.Println("Exit ELM327 run")
	for {
		select {
		case <-ctx.Done():
			return
		case <-el.closeChan:
			return
		case frame := <-el.sendChan:
			if el.currentID != frame.Identifier {
				if err := el.setHeader(frame.Identifier); err != nil {
					el.Error(err)
					continue
				}
			}

			if frame.FrameType.Type == 2 { // Response Required
				if !el.response {
					if err := el.setResponse(true); err != nil {
						el.Error(err)
						continue
					}
				}
			} else {
				if el.response {
					if err := el.setResponse(false); err != nil {
						el.Error(err)
						continue
					}
				}
			}

			resp, err := el.sendCommand(fmt.Sprintf("%02X", frame.Data))
			if err != nil {
				log.Printf("failed to send frame %v: %v", frame, err)
				continue
			}
			if resp == "\r" {
				continue
			}
			resp = strings.TrimSuffix(resp, "\r\r")
			for msg := range strings.SplitSeq(resp, "\r") {
				switch msg {
				case "NO DATA":
					el.sendEvent(EventTypeError, "CAN ERROR")
					continue
				case "STOPPED":
					el.sendEvent(EventTypeInfo, "STOPPED")
					continue
				case "?":
					el.sendEvent(EventTypeWarning, "UNKNOWN COMMAND")
					continue
				case "OK":
					continue
				}
				if len(msg) < 4 {
					el.Error(fmt.Errorf("message invalid: %s", msg))
					continue
				}
				idStr := msg[0:3]
				id, err := strconv.ParseUint(idStr, 16, 32)
				if err != nil {
					el.Error(fmt.Errorf("invalid id in message %q: %w", msg, err))
					continue
				}
				data, err := hex.DecodeString(msg[3:])
				if err != nil {
					el.Error(fmt.Errorf("invalid data in message %q: %w", msg, err))
					continue
				}
				frame := &CANFrame{
					Identifier: uint32(id),
					Data:       data,
					FrameType:  Incoming,
				}
				select {
				case el.recvChan <- frame:
				default:
					el.Error(errors.New("recvChan full, frame dropped"))
				}
			}
		}
	}
}

func (el *ELM327) setHeader(id uint32) error {
	if el.cfg.Debug {
		log.Printf("setHeader: %03X", id)
	}
	resp, err := el.sendCommand(fmt.Sprintf("ATSH%03X", id))
	if err != nil {
		return fmt.Errorf("failed to set header for ID %03X: %v", id, err)
	}
	if resp != ELM327_OK {
		return fmt.Errorf("failed to set header for ID %03X: %q", id, resp)
	}
	el.currentID = id
	return nil
}

func (el *ELM327) setResponse(enabled bool) error {
	if el.cfg.Debug {
		log.Printf("setResponse: %v", enabled)
	}
	var enableStr string = "0"
	if enabled {
		enableStr = "1"
	}
	resp, err := el.sendCommand("ATR" + enableStr)
	if err != nil {
		return fmt.Errorf("failed to set response required: %w", err)
	}
	if resp != ELM327_OK {
		return fmt.Errorf("failed to set response required: %q", resp)
	}
	el.response = enabled
	return nil
}

func (el *ELM327) setFilter(ids []uint32) error {
	filt, mask := el.calculateFilter(ids)

	if el.cfg.Debug {
		log.Printf("calculating filters for: %03X filt=%03X mask=%03X", ids, filt, mask)
	}

	filtResp, err := el.sendCommand(fmt.Sprintf("ATCF%03X", filt))
	if err != nil {
		return fmt.Errorf("error setting filter: %w", err)
	}
	if filtResp != ELM327_OK {
		return fmt.Errorf("error setting filter: %q", filtResp)
	}

	maskResp, err := el.sendCommand(fmt.Sprintf("ATCM%03X", mask))
	if err != nil {
		return fmt.Errorf("error setting mask: %w", err)
	}
	if maskResp != ELM327_OK {
		return fmt.Errorf("error setting mask: %q", maskResp)
	}

	return nil
}

func (el *ELM327) calculateFilter(ids []uint32) (uint32, uint32) {
	filt := uint32(0xFFF)
	mask := uint32(0x000)
	if len(ids) == 0 {
		return filt, mask
	}
	for _, id := range ids {
		filt &= id
		mask |= id
	}
	mask = (^mask & 0x7FF) | filt
	return filt, mask
}

func (el *ELM327) sendCommand(cmd string) (string, error) {
	if err := el.writePort(cmd); err != nil {
		return "", err
	}
	return el.readUntilPrompt(1 * time.Second)
}

func (el *ELM327) writePort(cmd string) error {
	n, err := el.port.Write([]byte(cmd + "\r"))
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}
	if n != len(cmd)+1 {
		return fmt.Errorf("failed to send full command, sent %d of %d bytes", n, len(cmd)+1)
	}
	return nil
}

func (el *ELM327) readUntilPrompt(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	buffer := make([]byte, 1024*4)
	cmdSize := 0

	var readBuf [1]byte
	for {
		if time.Now().After(deadline) {
			return "", errors.New("timeout waiting for '>' prompt")
		}
		// Use non-blocking-ish small read; Reader will still block, but we have timeout above.
		n, err := el.port.Read(readBuf[:])
		if err != nil {
			return "", fmt.Errorf("read from port: %w", err)
		}
		if n == 0 {
			continue
		}
		b := readBuf[0]
		if b == '>' {
			break
		}
		// Skip raw CR/LF in the assembled response.
		//if b == '\r' || b == '\n' {
		//	continue
		//}
		buffer[cmdSize] = b
		cmdSize++
	}
	return string(buffer[:cmdSize]), nil
}

func (el *ELM327) changeDeviceBaudrate(from, to int) error {
	log.Printf("changing to %d bps", to)
	elm := []byte("ELM327")
	mode := &serial.Mode{
		BaudRate: from,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}
	if err := el.port.SetMode(mode); err != nil {
		return err
	}

	divider := int(math.Round(4000000.0 / float64(to)))
	cmd := fmt.Sprintf("ATBRD%02X", divider)
	el.writePort(cmd)
	time.Sleep(50 * time.Millisecond)

	// Switch to new baud
	if err := el.port.ResetInputBuffer(); err != nil {
		return err
	}
	mode.BaudRate = int(to)
	if err := el.port.SetMode(mode); err != nil {
		return err
	}

	readBuf := make([]byte, 64)
	lineBuf := make([]byte, 0, 128)

	for range 10 {
		n, err := el.port.Read(readBuf)
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		for _, b := range readBuf[:n] {
			if b == '\r' {
				if len(lineBuf) == 0 {
					continue
				}
				if bytes.Contains(lineBuf, elm) {
					el.Info(string(lineBuf))
					if _, err := el.port.Write([]byte{'\r'}); err != nil {
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
