package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
	"go.bug.st/serial"
	"golang.org/x/sync/errgroup"
)

var stnAdapterSpeeds = []int{115200, 38400, 230400, 921600, 2000000, 1000000, 57600}

func init() {
	Register("OBDLink SX", NewSTN)
	Register("OBDLink MX", NewSTN)
}

type STN struct {
	cfg          *gocan.AdapterConfig
	port         serial.Port
	canrate      string
	protocol     string
	send, recv   chan gocan.CANFrame
	close        chan struct{}
	closed       bool
	filter, mask string
	semChan      chan token
}

func NewSTN(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	sx := &STN{
		cfg:     cfg,
		send:    make(chan gocan.CANFrame, 10),
		recv:    make(chan gocan.CANFrame, 20),
		close:   make(chan struct{}),
		semChan: make(chan token, 1),
	}

	if err := sx.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}

	sx.setCANfilter(cfg.CANFilter)

	return sx, nil
}

func (stn *STN) Name() string {
	return "STN"
}

func (stn *STN) Init(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: stn.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(stn.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", stn.cfg.Port, err)
	}

	p.SetReadTimeout(1 * time.Millisecond)

	stn.port = p

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	err = retry.Do(func() error {
		to := stn.cfg.PortBaudrate
		for _, from := range stnAdapterSpeeds {
			if err := stn.setSpeed(p, mode, from, to); err == nil {
				stn.cfg.OutputFunc(fmt.Sprintf("Switched adapter baudrate from %d to %d bps", from, to))
				return nil
			} else {
				stn.cfg.ErrorFunc(err)
			}
		}
		return errors.New("Failed to switch adapter baudrate") //lint:ignore ST1005 ignore this
	},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.OnRetry(func(n uint, err error) {
			stn.cfg.ErrorFunc(fmt.Errorf("retry #%d: %w", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		p.Close()
		return err
	}

	var initCmds = []string{
		"ATE0",       // turn off echo
		"ATS0",       // turn of spaces
		stn.protocol, // Set canbus protocol
		"ATH1",       // Headers on
		"ATAT2",      // Set adaptive timing mode, Adaptive timing on, aggressive mode. This option may increase throughput on slower connections, at the expense of slightly increasing the risk of missing frames.
		"ATCAF0",     // Automatic formatting off
		stn.canrate,  // Set CANrate
		"ATAL",       // Allow long messages
		"ATCFC0",     //Turn automatic CAN flow control off
		//"ATAR",      // Automatically set the receive address.
		//"ATCSM1",  //Turn CAN silent monitoring off
		stn.mask,   // mask
		stn.filter, // code
	}

	delay := 15 * time.Millisecond

	time.Sleep(delay)
	for _, c := range initCmds {
		if c == "" {
			continue
		}
		out := []byte(c + "\r")
		if debug {
			stn.cfg.OutputFunc(c)
		}
		if _, err := p.Write(out); err != nil {
			stn.cfg.ErrorFunc(err)
		}
		time.Sleep(delay)
	}
	p.ResetInputBuffer()

	go stn.recvManager(ctx)
	go stn.sendManager(ctx)

	return nil
}

func (stn *STN) setSpeed(p serial.Port, mode *serial.Mode, from, to int) error {
	mode.BaudRate = from
	if err := p.SetMode(mode); err != nil {
		return err
	}
	start := time.Now()
	for i := 0; i < 2; i++ {
		p.Write([]byte("ATI\r"))
		time.Sleep(20 * time.Millisecond)
		p.ResetInputBuffer()
	}
	errg, _ := errgroup.WithContext(context.Background())
	errg.Go(func() error {
		readbuff := make([]byte, 8)
		buff := bytes.NewBuffer(nil)
		for time.Since(start) < 300*time.Millisecond {
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
					if strings.HasPrefix(buff.String(), "ELM327") || strings.HasPrefix(buff.String(), "STN") {
						stn.cfg.OutputFunc(buff.String())
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

	p.Write([]byte("STBR" + strconv.Itoa(to) + "\r"))
	time.Sleep(5 * time.Millisecond)
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

func (stn *STN) setCANrate(rate float64) error {
	switch rate {
	case 33.3: //MX ony feature
		stn.protocol = "STP61"
		stn.canrate = "STCSWM2"
	case 500:
		stn.protocol = "STP33"
	case 615.384:
		stn.protocol = "STP33"
		stn.canrate = "STCTR8101FC"
	default:
		return fmt.Errorf("unhandled CANBus rate: %f", rate)
	}
	return nil
}

func (stn *STN) setCANfilter(ids []uint32) {
	var filt uint32 = 0xFFF
	var mask uint32 = 0x000
	for _, id := range ids {
		filt &= id
		mask |= id
	}
	mask = (^mask & 0x7FF) | filt
	stn.filter = fmt.Sprintf("ATCF%03X", filt)
	stn.mask = fmt.Sprintf("ATCM%03X", mask)
}

func (stn *STN) Recv() <-chan gocan.CANFrame {
	return stn.recv
}

func (stn *STN) Send() chan<- gocan.CANFrame {
	return stn.send
}

func (stn *STN) Close() error {
	stn.closed = true
	close(stn.close)
	time.Sleep(100 * time.Millisecond)
	stn.port.Write([]byte("ATZ\r"))
	time.Sleep(100 * time.Millisecond)
	stn.port.ResetInputBuffer()

	return stn.port.Close()
}

func (stn *STN) sendManager(ctx context.Context) {
	f := bytes.NewBuffer(nil)
	for {
		select {
		case v := <-stn.send:
			switch v.(type) {
			default:
				idb := make([]byte, 4)
				binary.BigEndian.PutUint32(idb, v.Identifier())

				t := v.Type()
				timeout := v.Timeout().Milliseconds()

				//a := v.Identifier()
				// write cmd and header
				//f.Write([]byte{'S','T','P','X','h', ':',byte(a>>8) + 0x30, (byte(a) >> 4) + 0x30, ((byte(a) << 4) >> 4) + 0x30, ','})
				f.WriteString("STPXh:" + hex.EncodeToString(idb)[5:] + "," +
					"d:" + hex.EncodeToString(v.Data()) + ",",
				)
				// write timeout
				if timeout > 0 {
					f.WriteString("t:" + strconv.Itoa(int(timeout)) + ",")
				}
				// write reply
				f.WriteString("r:" + strconv.Itoa(t.GetResponseCount()) + "\r")
			}
			stn.semChan <- token{}
			if debug {
				stn.cfg.OutputFunc("<o> " + f.String())
			}
			_, err := stn.port.Write(f.Bytes())
			if err != nil {
				stn.cfg.ErrorFunc(fmt.Errorf("failed to write to com port: %q, %w", f.String(), err))
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-stn.close:
			return
		}
	}
}

func (stn *STN) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 21)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := stn.port.Read(readBuffer)
		if err != nil {
			if !stn.closed {
				stn.cfg.ErrorFunc(fmt.Errorf("failed to read com port: %w", err))
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
				case <-stn.semChan:
				default:
				}
				continue
			}

			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if debug {
					stn.cfg.OutputFunc("<i> " + buff.String())
				}
				switch buff.String() {
				case "CAN ERROR":
					stn.cfg.ErrorFunc(errors.New("CAN ERROR"))
					buff.Reset()
				case "STOPPED":
					buff.Reset()
				case "?":
					stn.cfg.ErrorFunc(errors.New("UNKNOWN COMMAND"))
					buff.Reset()
				case "NO DATA":
					buff.Reset()
				case "OK":
					buff.Reset()
				default:
					f, err := stn.decodeFrame(buff.Bytes())
					if err != nil {
						stn.cfg.ErrorFunc(fmt.Errorf("failed to decode frame: %s %w", buff.String(), err))
						continue
					}
					select {
					case stn.recv <- f:
					default:
						stn.cfg.ErrorFunc(fmt.Errorf("dropped frame: %s", f.String()))
					}
					buff.Reset()
				}
				continue
			}
			buff.WriteByte(b)
		}
	}
}

func (*STN) decodeFrame(buff []byte) (gocan.CANFrame, error) {
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
