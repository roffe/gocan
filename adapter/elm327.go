package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/roffe/gocan"
	"go.bug.st/serial"
	"golang.org/x/sync/errgroup"
)

var elm327AdapterSpeeds = []int{38400, 115200, 230400, 285714, 500000, 1000000, 2000000}

/* func init() {
	if err := Register(&AdapterInfo{
		Name:               "ELM327",
		Description:        "ELM327",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewELM327,
	}); err != nil {
		panic(err)
	}
} */

type ELM327 struct {
	BaseAdapter

	port     serial.Port
	canrate  string
	protocol string

	closed       bool
	filter, mask string
	semChan      chan token
}

func NewELM327(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	elm := &ELM327{
		BaseAdapter: NewBaseAdapter(cfg),
		semChan:     make(chan token, 1),
	}

	if err := elm.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}

	elm.setCANfilter(cfg.CANFilter)

	return elm, nil
}

func (elm *ELM327) SetFilter(filters []uint32) error {
	elm.setCANfilter(filters)
	elm.send <- gocan.NewRawCommand("STPC")
	elm.send <- gocan.NewRawCommand(elm.mask)
	elm.send <- gocan.NewRawCommand(elm.filter)
	elm.send <- gocan.NewRawCommand("STPO")
	//elm.send <- gocan.NewRawCommand("ATCM7FF")
	//elm.send <- gocan.NewRawCommand("ATCF258")
	return nil
}

func (elm *ELM327) Name() string {
	return "ELM327"
}

func (elm *ELM327) Init(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: elm.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if p, err := serial.Open(elm.cfg.Port, mode); err != nil {
		return fmt.Errorf("failed to open com port %q : %v", elm.cfg.Port, err)
	} else {
		elm.port = p
	}

	if err := elm.port.SetReadTimeout(1 * time.Millisecond); err != nil {
		return err
	}

	elm.port.ResetOutputBuffer()
	elm.port.ResetInputBuffer()

	setSpeed := func() error {
		to := elm.cfg.PortBaudrate
		for _, from := range elm327AdapterSpeeds {
			if err := elm.setSpeed(elm.port, mode, from, to); err != nil {
				if elm.cfg.Debug {
					elm.cfg.OnError(err)
				}
			} else {
				if elm.cfg.Debug {
					elm.cfg.OnMessage(fmt.Sprintf("Switched adapter baudrate from %d to %d bps", from, to))
				}
				return nil
			}
		}
		return errors.New("Failed to switch adapter baudrate") //lint:ignore ST1005 ignore this
	}
	if err := setSpeed(); err != nil {
		elm.port.Close()
		return err
	}

	var initCmds = []string{
		"ATE0",       // turn off echo
		"ATS0",       // turn of spaces
		elm.protocol, // Set canbus protocol
		"ATH1",       // Headers on
		"ATAT2",      // Set adaptive timing mode, Adaptive timing on, aggressive mode. This option may increase throughput on slower connections, at the expense of slightly increasing the risk of missing frames.
		"ATCAF0",     // Automatic formatting off
		elm.canrate,  // Set CANrate
		"ATAL",       // Allow long messages
		"ATCFC0",     //Turn automatic CAN flow control off
		//"ATAR",      // Automatically set the receive address.
		//"ATCSM1",  //Turn CAN silent monitoring off
		elm.mask,   // mask
		elm.filter, // code
	}

	delay := 15 * time.Millisecond

	time.Sleep(delay)
	for _, c := range initCmds {
		if c == "" {
			continue
		}
		out := []byte(c + "\r")
		if elm.cfg.Debug {
			elm.cfg.OnMessage(c)
		}
		if _, err := elm.port.Write(out); err != nil {
			elm.cfg.OnError(err)
		}
		time.Sleep(delay)
	}
	elm.port.ResetInputBuffer()

	go elm.recvManager(ctx)
	go elm.sendManager(ctx)

	return nil
}

func calculateATBRDCommand(desiredBaudRate int) string {
	divider := int(math.Round(4000000.0 / float64(desiredBaudRate)))
	// Format the command string
	return fmt.Sprintf("ATBRD %02X", divider)

}

func (elm *ELM327) setSpeed(p serial.Port, mode *serial.Mode, from, to int) error {
	mode.BaudRate = from
	if err := p.SetMode(mode); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	start := time.Now()
	for i := 0; i < 2; i++ {
		p.Write([]byte("ATI\r"))
		time.Sleep(300 * time.Millisecond)
		p.ResetInputBuffer()
	}
	errg, _ := errgroup.WithContext(context.Background())
	errg.Go(func() error {
		readbuff := make([]byte, 8)
		buff := bytes.NewBuffer(nil)
		for time.Since(start) < 900*time.Millisecond {
			n, err := p.Read(readbuff)
			if err != nil {
				if err == io.EOF {
					break
				}
				p.Close()
				return err
			}
			if n == 0 {
				continue
			}
			for _, b := range readbuff[:n] {
				if b == '\r' {
					if buff.Len() == 0 {
						continue
					}
					if strings.HasPrefix(buff.String(), "ELM327") {
						if elm.cfg.PrintVersion {
							elm.cfg.OnMessage(buff.String())
						}
						return nil
					}
					buff.Reset()
					continue
				}
				buff.WriteByte(b)
			}
		}
		return fmt.Errorf("failed to change adapter baudrate from %d to %d bps", from, to)
	})

	//	p.Write([]byte("ATBRD" + strconv.Itoa(to) + "\r"))
	cmd := calculateATBRDCommand(to)
	log.Println(cmd)
	p.Write([]byte(cmd + "\r"))
	time.Sleep(20 * time.Millisecond)
	mode.BaudRate = to
	if err := p.SetMode(mode); err != nil {
		return err
	}

	if err := errg.Wait(); err != nil {
		return err
	}
	p.Write([]byte("\r"))
	p.ResetInputBuffer()
	return nil
}

func (elm *ELM327) setCANrate(rate float64) error {
	switch rate {
	case 33.3: // STN1170 & STN2120 feature only
		elm.protocol = "STP61"
		elm.canrate = "STCSWM2"
	case 500:
		elm.protocol = "ATSP6"
	case 615.384:
		elm.protocol = "STP33"
		elm.canrate = "STCTR8101FC"
	default:
		return fmt.Errorf("unhandled CANBus rate: %f", rate)
	}
	return nil
}

func (elm *ELM327) setCANfilter(ids []uint32) {
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
	elm.filter = fmt.Sprintf("AT CF %03X", filt)
	elm.mask = fmt.Sprintf("AT CM %03X", mask)
}

func (elm *ELM327) Close() error {
	elm.BaseAdapter.Close()
	elm.closed = true
	time.Sleep(150 * time.Millisecond)
	elm.port.ResetOutputBuffer()
	elm.port.Write([]byte("ATSP00\r"))
	elm.port.Write([]byte("ATZ\r"))
	time.Sleep(50 * time.Millisecond)
	elm.port.ResetInputBuffer()
	return elm.port.Close()
}

func (elm *ELM327) sendManager(ctx context.Context) {
	f := bytes.NewBuffer(nil)
	for {
		select {
		case v := <-elm.send:
			switch v.(type) {
			case *gocan.RawCommand:
				f.WriteString(v.String() + "\r")
			default:
				idb := make([]byte, 4)
				binary.BigEndian.PutUint32(idb, v.Identifier())

				t := v.Type()
				timeout := v.Timeout().Milliseconds()

				//a := v.Identifier()
				// write cmd and header
				//f.Write([]byte{'S','T','P','X','h', ':',byte(a>>8) + 0x30, (byte(a) >> 4) + 0x30, ((byte(a) << 4) >> 4) + 0x30, ','})
				f.WriteString("STPXh:" + hex.EncodeToString(idb)[5:] + "," +
					"d:" + hex.EncodeToString(v.Data()),
				)
				// write timeout
				if timeout > 300 {
					f.WriteString(",t:" + strconv.Itoa(int(timeout)))
				}
				// write reply
				f.WriteString(",r:" + strconv.Itoa(t.GetResponseCount()) + "\r")

			}
			elm.semChan <- token{}
			if elm.cfg.Debug {
				elm.cfg.OnMessage("<o> " + f.String())
			}
			if _, err := elm.port.Write(f.Bytes()); err != nil {
				elm.cfg.OnError(fmt.Errorf("failed to write to com port: %q, %w", f.String(), err))
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-elm.close:
			return
		}
	}
}

func (elm *ELM327) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 21)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := elm.port.Read(readBuffer)
		if err != nil {
			if !elm.closed {
				elm.cfg.OnError(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if n == 0 {
			continue
		}
		for _, b := range readBuffer[:n] {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if b == '>' {
				select {
				case <-elm.semChan:
				default:
				}
				continue
			}

			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if elm.cfg.Debug {
					elm.cfg.OnMessage("<i> " + buff.String())
				}
				switch buff.String() {
				case "CAN ERROR":
					elm.cfg.OnError(errors.New("CAN ERROR"))
					buff.Reset()
				case "STOPPED":
					buff.Reset()
				case "?":
					elm.cfg.OnError(errors.New("UNKNOWN COMMAND"))
					buff.Reset()
				case "NO DATA", "OK":
					buff.Reset()
				default:
					f, err := elm.decodeFrame(buff.Bytes())
					if err != nil {
						elm.cfg.OnError(fmt.Errorf("failed to decode frame: %s %w", buff.String(), err))
						buff.Reset()
						continue
					}
					select {
					case elm.recv <- f:
					default:
						elm.cfg.OnError(ErrDroppedFrame)
					}
					buff.Reset()
				}
				continue
			}
			buff.WriteByte(b)
		}
	}
}

func (*ELM327) decodeFrame(buff []byte) (gocan.CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[:3]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}
	data, err := hex.DecodeString(string(buff[3:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}
	return gocan.NewFrame(
		uint32(id),
		data,
		gocan.Incoming,
	), nil
}
