package adapter

import (
	"context"
	"os"

	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/roffe/gocan"
	"go.bug.st/serial"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               "Just4Trionic",
		Description:        "STM32F103C8T6 based CAN adapter",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewJust4Trionic,
	}); err != nil {
		panic(err)
	}
}

type Just4Trionic struct {
	BaseAdapter
	port    serial.Port
	canRate string
	closed  bool
}

func NewJust4Trionic(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	adapter := &Just4Trionic{
		BaseAdapter: NewBaseAdapter("Just4Trionic", cfg),
	}

	/*
		for _, f := range cfg.CANFilter {
			switch f {
			case 0x05:
				adapter.filter = "t5"
			case 0x220:
				adapter.filter = "f7"
			case 0x7E0:
				adapter.filter = "f8"
			default:
				adapter.filter = "f0"
			}
				if f == 0x05 {
					adapter.filter = "t5"
					break
				}
				if f == 0x220 {
					adapter.filter = "f7"
					break
				}
				if f == 0x7E0 {
					adapter.filter = "f8"
					break
				}
			}
	*/

	if err := adapter.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}

	return adapter, nil
}

func (a *Just4Trionic) SetFilter(filters []uint32) error {
	filter, mask := a.calcAcceptanceFilters(filters)
	a.send <- gocan.NewRawCommand("C")
	a.send <- gocan.NewRawCommand(filter)
	a.send <- gocan.NewRawCommand(mask)
	a.send <- gocan.NewRawCommand("O")
	return nil
}

func (*Just4Trionic) calcAcceptanceFilters(idList []uint32) (string, string) {
	if len(idList) == 1 && idList[0] == 0 {
		return "\r", "\r"
	}
	var code = ^uint32(0)
	var mask uint32 = 0

	if len(idList) == 0 {
		code = 0
		mask = ^uint32(0)
	} else {
		code = 0
		for _, canID := range idList {
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16

	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

func (a *Just4Trionic) Connect(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(a.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", a.cfg.Port, err)
	}
	p.SetReadTimeout(1 * time.Millisecond)
	a.port = p

	p.ResetOutputBuffer()

	code, mask := a.calcAcceptanceFilters(a.cfg.CANFilter)

	var cmds = []string{
		"\x1B", // Empty buffer
		"O",    // enter canbus mode
		//a.filter,
		code,
		mask,
		a.canRate, // Setup CAN bit-rates
		//a.mask,
	}

	delay := time.Duration(5 * time.Millisecond)

	for n, c := range cmds {
		if n == 3 {
			p.ResetInputBuffer()
		}
		if a.cfg.Debug {
			a.cfg.OnMessage("sending: " + c)
		}
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			p.Close()
			return err
		}
		time.Sleep(delay)
	}

	go a.recvManager(ctx)
	go a.sendManager(ctx)

	return nil
}

func (a *Just4Trionic) Close() error {
	a.BaseAdapter.Close()
	a.closed = true
	time.Sleep(50 * time.Millisecond)
	a.port.Write([]byte("\x1B"))
	time.Sleep(10 * time.Millisecond)
	return a.port.Close()
}

func (a *Just4Trionic) setCANrate(rate float64) error {
	switch rate {
	case 10:
		a.canRate = "S0"
	case 20:
		a.canRate = "S1"
	case 50:
		a.canRate = "S2"
	case 100:
		a.canRate = "S3"
	case 125:
		a.canRate = "S4"
	case 250:
		a.canRate = "S5"
	case 500:
		a.canRate = "S6"
	case 615.384:
		a.canRate = "s2"
	case 800:
		a.canRate = "S7"
	case 1000:
		a.canRate = "S8"
	default:
		return fmt.Errorf("unknown rate: %f", rate)
	}
	return nil
}

func (a *Just4Trionic) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := a.port.Read(readBuffer)
		if err != nil {
			if !a.closed {
				a.err <- fmt.Errorf("failed to read com port: %w", err)
			}
			return
		}
		if n == 0 {
			continue
		}
		a.parse(ctx, readBuffer[:n], buff)
	}
}

func (a *Just4Trionic) parse(ctx context.Context, readBuffer []byte, buff *bytes.Buffer) {
	for _, b := range readBuffer {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if b == 0x0D || b == 0x0A {
			if buff.Len() == 0 {
				continue
			}
		}
		if b == 0x0A {
			by := buff.Bytes()
			switch by[0] {
			case 'w':
				f, err := a.decodeFrame(by[1 : buff.Len()-1])
				if err != nil {
					a.cfg.OnError(fmt.Errorf("failed to decode frame: %w %X", err, by))
					continue
				}
				select {
				case a.recv <- f:
				default:
					a.cfg.OnError(ErrDroppedFrame)
				}
				buff.Reset()
			default:
				a.cfg.OnMessage("<< " + buff.String())
			}
			buff.Reset()
			continue
		}
		buff.WriteByte(b)
	}
}

func (*Just4Trionic) decodeFrame(buff []byte) (gocan.CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[0:3]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %w", err)
	}
	data := make([]byte, hex.DecodedLen(int(buff[3]-0x30)*2))
	if _, err := hex.Decode(data, buff[4:]); err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %w", err)
	}
	return gocan.NewFrame(
		uint32(id),
		data,
		gocan.Incoming,
	), nil
}

func (a *Just4Trionic) sendManager(ctx context.Context) {
	var f string
	for {
		select {
		case v := <-a.send:
			switch v.(type) {
			case *gocan.RawCommand:
				if _, err := a.port.Write(append(v.Data(), '\r')); err != nil {
					a.err <- fmt.Errorf("failed to write to com port: %q, %w", f, err)
				}
				if a.cfg.Debug {
					fmt.Fprint(os.Stderr, ">> "+v.String()+"\n")
				}
			default:
				f = "t" + strconv.FormatUint(uint64(v.Identifier()), 16) +
					strconv.Itoa(v.Length()) +
					hex.EncodeToString(v.Data())

				for i := v.Length(); i < 8; i++ {
					f += "00"
				}
				f += "\r"
				if _, err := a.port.Write([]byte(f)); err != nil {
					a.cfg.OnError(fmt.Errorf("failed to write to com port: %q, %w", f, err))
				}
				if a.cfg.Debug {
					a.cfg.OnMessage(">> " + f)
				}
				f = ""
			}

		case <-ctx.Done():
			return
		case <-a.close:
			return
		}
	}
}
