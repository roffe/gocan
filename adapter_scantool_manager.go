package gocan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
)

type scantoolManager struct {
	*BaseAdapter
	port io.ReadWriter
}

func (scm *scantoolManager) run(ctx context.Context) {
	defer log.Println("exit scantoolManager")
	var (
		cmdBuf  bytes.Buffer // reused for every command
		idBytes [4]byte      // stack-allocated, reused
		idHex   [8]byte      // hex(idBytes) => 8 chars
		dataHex [16]byte     // up to 8 data bytes => 16 chars
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-scm.closeChan:
			return
		case frame := <-scm.sendChan:
			// System / control messages
			if id := frame.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if scm.cfg.Debug {
						// log the payload instead of the old cmdBuf contents
						scm.cfg.OnMessage("<o> " + string(frame.Data))
					}

					// Avoid append (which may allocate / mutate caller's slice):
					if _, err := scm.port.Write(frame.Data); err != nil {
						scm.setError(fmt.Errorf("failed to write: %q %w", string(frame.Data), err))
						return
					}
					if _, err := scm.port.Write([]byte{'\r'}); err != nil {
						scm.setError(fmt.Errorf("failed to write CR: %q %w", string(frame.Data), err))
						return
					}
				}
				continue
			}

			// Build STPX command in a reusable buffer
			cmdBuf.Reset()

			// Identifier -> "STPXh:xxx"
			binary.BigEndian.PutUint32(idBytes[:], frame.Identifier)
			hex.Encode(idHex[:], idBytes[:]) // no allocations

			cmdBuf.WriteString("STPXh:")
			// original code used hex.EncodeToString(idb)[5:], i.e. 3 hex chars
			cmdBuf.Write(idHex[5:]) // same effect as that [5:]

			// Data -> ",d:..."
			cmdBuf.WriteString(",d:")
			if len(frame.Data) <= 8 {
				// CAN classic payload fits here, avoid allocation
				n := hex.Encode(dataHex[:], frame.Data)
				cmdBuf.Write(dataHex[:n])
			} else {
				// fallback if you ever send more than 8 bytes
				cmdBuf.WriteString(hex.EncodeToString(frame.Data))
			}

			// Timeout -> ",t:N" (avoid strconv.Itoa allocations)
			if frame.Timeout != 0 && frame.Timeout != 200 {
				cmdBuf.WriteString(",t:")
				var numBuf [8]byte
				n := strconv.AppendInt(numBuf[:0], int64(frame.Timeout), 10)
				cmdBuf.Write(numBuf[:len(n)])
			}

			// Response count -> ",r:N"
			if respCount := frame.FrameType.Responses; respCount > 0 {
				cmdBuf.WriteString(",r:")
				var numBuf [8]byte
				n := strconv.AppendInt(numBuf[:0], int64(respCount), 10)
				cmdBuf.Write(numBuf[:len(n)])
			}

			cmd := cmdBuf.String() // single string allocation per command

			if scm.cfg.Debug {
				scm.cfg.OnMessage("<o> " + cmd)
			}

			resp, err := scm.sendCommand(cmd)
			if err != nil {
				scm.sendErrorEvent(fmt.Errorf("failed to send command: %w", err))
				continue
			}
			if scm.cfg.Debug {
				scm.cfg.OnMessage("<i> " + resp)
			}

			if resp == "\r" {
				continue
			}

			for msg := range strings.SplitSeq(strings.TrimSuffix(resp, "\r\r"), "\r") {
				switch msg {
				case "CAN ERROR":
					scm.sendEvent(EventTypeError, "CAN ERROR")
				case "STOPPED":
					scm.sendEvent(EventTypeInfo, "STOPPED")
				case "?":
					scm.sendEvent(EventTypeWarning, "UNKNOWN COMMAND")
				case "NO DATA", "OK":
					// nothing
				default:
					frm, err := scantoolDecodeFrame([]byte(msg)) // still alloc; see notes below
					if err != nil {
						scm.sendErrorEvent(fmt.Errorf("failed to decode frame: %q %w", msg, err))
						continue
					}
					select {
					case scm.recvChan <- frm:
					default:
						scm.sendErrorEvent(ErrDroppedFrame)
					}
				}
			}
		}
	}
}

func (scm *scantoolManager) sendCommand(cmd string) (string, error) {
	if err := scm.writePort(cmd); err != nil {
		return "", err
	}
	return scm.readUntilPrompt(1 * time.Second)
}

func (scm *scantoolManager) writePort(cmd string) error {
	n, err := scm.port.Write([]byte(cmd + "\r"))
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}
	if n != len(cmd)+1 {
		return fmt.Errorf("failed to send full command, sent %d of %d bytes", n, len(cmd)+1)
	}
	return nil
}

func (scm *scantoolManager) readUntilPrompt(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	buffer := make([]byte, 1024*4)
	cmdSize := 0

	var readBuf [1]byte
	for {
		if time.Now().After(deadline) {
			return "", errors.New("timeout waiting for '>' prompt")
		}
		// Use non-blocking-ish small read; Reader will still block, but we have timeout above.
		n, err := scm.port.Read(readBuf[:])
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
