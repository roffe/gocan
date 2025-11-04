package gocan

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"go.bug.st/serial"
)

type SLCan struct {
	BaseAdapter
	port   serial.Port
	closed bool
}

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "SLCan",
		Description:        "Canable SLCan adapter",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewSLCan,
	}); err != nil {
		panic(err)
	}
}

func NewSLCan(cfg *AdapterConfig) (Adapter, error) {
	sl := &SLCan{
		BaseAdapter: NewBaseAdapter("SLCan", cfg),
	}
	return sl, nil
}

func (sl *SLCan) Open(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: sl.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(sl.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", sl.cfg.Port, err)
	}
	p.SetReadTimeout(3 * time.Millisecond)
	sl.port = p

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	go sl.sendManager(ctx)
	go sl.recvManager(ctx)

	switch sl.cfg.CANRate {
	case 10.0:
		p.Write([]byte("S0\r"))
	case 20.0:
		p.Write([]byte("S1\r"))
	case 50.0:
		p.Write([]byte("S2\r"))
	case 100.0:
		p.Write([]byte("S3\r"))
	case 125.0:
		p.Write([]byte("S4\r"))
	case 250.0:
		p.Write([]byte("S5\r"))
	case 500.0:
		log.Println("set 500")
		p.Write([]byte("S6\r"))
	case 750.0:
		p.Write([]byte("S7\r"))
	case 1000.0:
		p.Write([]byte("S8\r"))
	case 615.384:
		p.Write([]byte("S9\r"))
	}
	time.Sleep(10 * time.Millisecond)
	p.Write([]byte("O\r"))
	return nil
}

func (sl *SLCan) SetFilter(filters []uint32) error {
	return nil
}

func (sl *SLCan) Close() error {
	sl.BaseAdapter.Close()
	sl.closed = true
	time.Sleep(10 * time.Millisecond)
	sl.port.Write([]byte("C\r"))
	time.Sleep(10 * time.Millisecond)
	return sl.port.Close()
}

func (sl *SLCan) recvManager(ctx context.Context) {
	buf := make([]byte, 0, 1024)
	readBuf := make([]byte, 8)
	for ctx.Err() == nil {
		n, err := sl.port.Read(readBuf)
		if err != nil {
			if !sl.closed {
				sl.setError(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if n == 0 {
			continue
		}
		buf = sl.parse(ctx, buf, readBuf[:n])
	}
}

func (sl *SLCan) sendManager(ctx context.Context) {
	var outBuf = make([]byte, 0, 4096) // reused scratch buffer for frames
	for {
		select {
		case <-ctx.Done():
			return
		case <-sl.closeChan:
			return
		case frame := <-sl.sendChan:
			if err := sl.handleSend(frame, &outBuf); err != nil {
				sl.cfg.OnMessage(fmt.Sprintf("send error: %v", err))
			}
		}
	}
}

func (sl *SLCan) handleSend(frame *CANFrame, outBuf *[]byte) error {
	// System / control messages (like "O", "C", bitrate stuff)
	if id := frame.Identifier; id >= SystemMsg {
		if id == SystemMsg {
			if sl.cfg.Debug {
				log.Println(">> " + string(frame.Data))
			}
			if _, err := sl.port.Write(append(frame.Data, '\r')); err != nil {
				return fmt.Errorf("failed to write to com port: %w", err)
			}
		}
		return nil
	}

	// Reset the reusable buffer without realloc
	buf := (*outBuf)[:0]

	// SLCAN frame format for standard ID:
	// 't' + 3-hex-digit ID + len-nibble + data-as-hex + '\r'
	//
	// Example: t1238A1B2C3D4E5F6\r
	//
	// We currently only send 11-bit IDs, same as the old code.

	buf = append(buf, 't')

	// Encode 11-bit ID as 3 hex chars.
	// Old code did:
	//   idb := make([]byte,4)
	//   binary.BigEndian.PutUint32(idb, frame.Identifier)
	//   f.WriteString(hex.EncodeToString(idb)[5:])
	//
	// We'll do it without heap churn.

	id := frame.Identifier & 0x7FF // 11-bit
	// We want exactly 3 hex nybbles (padded): high -> low
	// nibble2 nibble1 nibble0
	n2 := byte((id >> 8) & 0xF)
	n1 := byte((id >> 4) & 0xF)
	n0 := byte(id & 0xF)

	buf = append(buf, nybbleToHex(n2), nybbleToHex(n1), nybbleToHex(n0))

	// DLC (single hex digit)
	dlc := frame.Length()
	buf = append(buf, nybbleToHex(byte(dlc)&0xF))

	for i := range dlc {
		buf = append(buf, nybbleToHex(frame.Data[i]>>4), nybbleToHex(frame.Data[i]&0xF))
	}

	// Terminate with CR
	buf = append(buf, '\r')
	// Send it
	if _, err := sl.port.Write(buf); err != nil {
		sl.sendErrorEvent(fmt.Errorf("failed to write to com port: %w", err))
	}
	if sl.cfg.Debug {
		// Safe to turn the slice into string for debug only
		log.Println(">> " + string(buf))
	}
	// Store the grown buffer back so capacity is kept/reused
	*outBuf = buf
	return nil
}

// helper converts a 0..15 value to its ASCII hex nibble
func nybbleToHex(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + (n - 10)
}

// parse processes the read data and returns any remaining partial data.
func (sl *SLCan) parse(ctx context.Context, buf, readBuf []byte) []byte {
	for _, b := range readBuf {
		if b == '\r' {
			if len(buf) == 0 {
				continue
			}
			switch buf[0] {
			case 't':
				if sl.cfg.Debug {
					log.Printf("<< %s", string(buf))
				}
				f, err := sl.decodeFrame(buf)
				if err != nil {
					sl.cfg.OnMessage(fmt.Sprintf("%v: %X", err, buf))
					buf = buf[:0]
					continue
				}

				select {
				case sl.recvChan <- f:
				case <-ctx.Done():
					return buf[:0]
				default:
					sl.sendErrorEvent(ErrDroppedFrame)
				}
			default:
				sl.sendWarningEvent("Unknown>> " + string(buf))
			}
			// Reset buffer after a full message
			buf = buf[:0]
		} else {
			buf = append(buf, b)
		}
	}
	return buf
}

func (*SLCan) decodeFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}
	dataLen, err := strconv.ParseUint(string(buff[4]), 16, 8)
	if dataLen > 16 {
		return nil, fmt.Errorf("invalid data length: %d", dataLen)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to decode data length: %v", err)
	}
	data, err := hex.DecodeString(string(buff[5 : 5+(dataLen*2)]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}
	return NewFrame(
		uint32(id),
		data,
		Incoming,
	), nil
}
