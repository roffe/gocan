//go:build ftdi

package gocan

import (
	"context"
	"fmt"
	"log"
	"time"
)

func (stn *ScantoolFTDI) recvManager(ctx context.Context) {
	defer log.Println("exit recvManager")

	// Prealloc working buffers and reuse them forever.
	// lineBuf holds the incoming ASCII line (one CAN frame or status message).
	lineBuf := make([]byte, 0, 256)

	// buf is our raw read buffer from FTDI.
	buf := make([]byte, 256)

	for ctx.Err() == nil {
		n, err := stn.port.Read(buf)
		if err != nil {
			if !stn.closed {
				stn.Error(fmt.Errorf("failed to read: %w", err))
			}
			return
		}
		if n == 0 {
			time.Sleep(300 * time.Microsecond)
			continue
		}

		// Parse incoming bytes into lines, split by '\r'
		// and handle '>' prompts inline.
		for _, b := range buf[:n] {

			switch b {
			case '>':
				// adapter prompt -> release send semaphore
				select {
				case <-stn.sendSem:
				default:
				}
				// don't include '>' in lineBuf
				continue

			case 0x0D: // CR = end of line
				if len(lineBuf) == 0 {
					// empty line, just reset
					lineBuf = lineBuf[:0]
					continue
				}

				if stn.cfg.Debug {
					// Only here do we pay for a string alloc, in debug builds.
					stn.Debug("<i> " + string(lineBuf))
				}

				switch {
				case equalBytesString(lineBuf, "CAN ERROR"):
					stn.sendEvent(EventTypeError, "CAN ERROR")
					lineBuf = lineBuf[:0]

				case equalBytesString(lineBuf, "STOPPED"):
					stn.sendEvent(EventTypeInfo, "STOPPED")
					lineBuf = lineBuf[:0]

				case equalBytesString(lineBuf, "?"):
					stn.sendEvent(EventTypeWarning, "UNKNOWN COMMAND")
					lineBuf = lineBuf[:0]

				case equalBytesString(lineBuf, "NO DATA"),
					equalBytesString(lineBuf, "OK"):
					// just ignore
					lineBuf = lineBuf[:0]

				default:
					// Decode CAN frame from ASCII without copying.
					f, err := scantoolDecodeFrame(lineBuf)
					if err != nil {
						// We will only allocate here to build the debug string.
						stn.Error(fmt.Errorf("failed to decode frame: %s %w", string(lineBuf), err))
						lineBuf = lineBuf[:0]
						continue
					}

					select {
					case stn.recvChan <- f:
						// ok
					default:
						stn.Error(ErrDroppedFrame)
					}
					lineBuf = lineBuf[:0]
				}
				continue

			default:
				// append byte to current line
				lineBuf = append(lineBuf, b)
			}
		}
	}
}
