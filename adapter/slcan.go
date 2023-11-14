package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/roffe/gocan"
	"go.bug.st/serial"
)

type SLCan struct {
	cfg        *gocan.AdapterConfig
	port       serial.Port
	send, recv chan gocan.CANFrame
	close      chan struct{}
	closed     bool
}

func init() {
	if err := Register(&AdapterInfo{
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

func NewSLCan(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	sl := &SLCan{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 30),
		close: make(chan struct{}, 1),
	}
	return sl, nil
}

func (sl *SLCan) Init(ctx context.Context) error {
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
	p.SetReadTimeout(1 * time.Millisecond)
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
		p.Write([]byte("S6\r"))
	case 750.0:
		p.Write([]byte("S7\r"))
	case 1000.0:
		p.Write([]byte("S8\r"))
	}
	time.Sleep(10 * time.Millisecond)
	p.Write([]byte("O\r"))
	return nil
}

func (sl *SLCan) SetFilter(filters []uint32) error {
	return nil
}

func (sl *SLCan) Close() error {
	sl.closed = true
	time.Sleep(10 * time.Millisecond)
	sl.port.Write([]byte("C\r"))
	time.Sleep(10 * time.Millisecond)
	return sl.port.Close()
}

func (sl *SLCan) Send() chan<- gocan.CANFrame {
	return sl.send
}

func (sl *SLCan) Recv() <-chan gocan.CANFrame {
	return sl.recv
}

func (sl *SLCan) Name() string {
	return "SLCan"
}

func (sl *SLCan) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := sl.port.Read(readBuffer)
		if err != nil {
			if !sl.closed {
				sl.cfg.OnError(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if n == 0 {
			continue
		}
		sl.parse(ctx, buff, readBuffer[:n])
	}
}

func (sl *SLCan) sendManager(ctx context.Context) {
	f := bytes.NewBuffer(nil)
	for {
		select {
		case v := <-sl.send:
			switch v.(type) {
			case *gocan.RawCommand:
				if _, err := sl.port.Write(append(v.Data(), '\r')); err != nil {
					sl.cfg.OnError(fmt.Errorf("failed to write to com port: %s, %w", f.String(), err))
				}
				if sl.cfg.Debug {
					log.Println(">> " + v.String())
				}
			default:
				idb := make([]byte, 4)
				binary.BigEndian.PutUint32(idb, v.Identifier())
				f.WriteString("t" + hex.EncodeToString(idb)[5:] +
					strconv.Itoa(v.Length()) +
					hex.EncodeToString(v.Data()) + "\x0D")
				if _, err := sl.port.Write(f.Bytes()); err != nil {
					sl.cfg.OnError(fmt.Errorf("failed to write to com port: %s, %w", f.String(), err))
				}
				if sl.cfg.Debug {
					log.Println(">> " + f.String())
				}
				f.Reset()
			}

		case <-ctx.Done():
			return
		case <-sl.close:
			return
		}
	}
}

func (sl *SLCan) parse(ctx context.Context, buff *bytes.Buffer, readBuffer []byte) {
	for _, b := range readBuffer {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if b == 0x0D {
			if buff.Len() == 0 {
				continue
			}
			by := buff.Bytes()
			switch by[0] {
			case 'F':
				if err := decodeStatus(by); err != nil {
					sl.cfg.OnError(fmt.Errorf("CAN status error: %w", err))
				}
			case 't':
				if sl.cfg.Debug {
					log.Println("<< " + buff.String())
				}
				f, err := sl.decodeFrame(by)
				if err != nil {
					sl.cfg.OnError(fmt.Errorf("failed to decode frame: %X", buff.Bytes()))
					continue
				}
				select {
				case sl.recv <- f:
				default:
					sl.cfg.OnError(ErrDroppedFrame)
				}
				buff.Reset()
			case 0x07: // bell, last command was unknown
				sl.cfg.OnError(errors.New("unknown command"))
			default:
				sl.cfg.OnMessage("Unknown>> " + buff.String())
			}
			buff.Reset()
			continue
		}
		buff.WriteByte(b)
	}
}

func (*SLCan) decodeFrame(buff []byte) (gocan.CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("filed to decode identifier: %v", err)
	}
	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}
	return gocan.NewFrame(
		uint32(id),
		data,
		gocan.Incoming,
	), nil
}
