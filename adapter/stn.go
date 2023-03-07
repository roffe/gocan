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

var stnAdapterSpeeds = []int{115200, 230400, 921600, 2000000, 1000000, 57600, 38400}

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
	sendMutex    chan token
}

func NewSTN(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	sx := &STN{
		cfg:       cfg,
		send:      make(chan gocan.CANFrame, 10),
		recv:      make(chan gocan.CANFrame, 10),
		close:     make(chan struct{}, 1),
		sendMutex: make(chan token, 1),
	}

	if err := sx.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}

	sx.setCANfilter(cfg.CANFilter)

	return sx, nil
}

func (cu *STN) Name() string {
	return "STN"
}

func (cu *STN) Init(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: cu.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(cu.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", cu.cfg.Port, err)
	}

	p.SetReadTimeout(1 * time.Millisecond)

	cu.port = p

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	err = retry.Do(func() error {
		to := cu.cfg.PortBaudrate
		for _, from := range stnAdapterSpeeds {
			if err := cu.setSpeed(p, mode, from, to); err == nil {
				cu.cfg.OutputFunc(fmt.Sprintf("Switched adapter baudrate from %d to %d bps", from, to))
				return nil
			} else {
				cu.cfg.ErrorFunc(fmt.Errorf("failed to switch adapter baudrate from %d to %d bps: %w", from, to, err))
			}
		}
		return errors.New("/!\\ Could not init adapter")
	},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.OnRetry(func(n uint, err error) {
			cu.cfg.ErrorFunc(fmt.Errorf("retry #%d: %w", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		p.Close()
		return err
	}

	var initCmds = []string{
		"ATE0",      // turn off echo
		"ATS0",      // turn of spaces
		cu.protocol, // Set canbus protocol
		"ATH1",      // Headers on
		"ATAT2",     // Set adaptive timing mode, Adaptive timing on, aggressive mode. This option may increase throughput on slower connections, at the expense of slightly increasing the risk of missing frames.
		"ATCAF0",    // Automatic formatting of
		cu.canrate,  // Set CANrate
		"ATAL",      // Allow long messages
		"ATCFC0",    //Turn automatic CAN flow control off
		//"ATAR",      // Automatically set the receive address.
		//"ATCSM1",  //Turn CAN silent monitoring off
		cu.mask,   // mask
		cu.filter, // code
	}

	delay := 20 * time.Millisecond

	for _, c := range initCmds {
		if c == "" {
			continue
		}
		out := []byte(c + "\r")
		if debug {
			cu.cfg.OutputFunc(c)
		}
		_, err := p.Write(out)
		if err != nil {
			cu.cfg.ErrorFunc(err)
		}
		time.Sleep(delay)
	}
	p.ResetInputBuffer()

	go cu.recvManager(ctx)
	go cu.sendManager(ctx)

	return nil
}

func (cu *STN) setSpeed(p serial.Port, mode *serial.Mode, from, to int) error {
	mode.BaudRate = from
	p.SetMode(mode)
	start := time.Now()
	for i := 0; i < 2; i++ {
		p.Write([]byte("ATI\r"))
		time.Sleep(20 * time.Millisecond)
		p.ResetInputBuffer()
	}
	errg, _ := errgroup.WithContext(context.Background())
	errg.Go(func() error {
		readbuff := make([]byte, 4)
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
					if strings.Contains(buff.String(), "ELM327") || strings.Contains(buff.String(), "STN") {
						cu.cfg.OutputFunc(buff.String())
						return nil
					}
					buff.Reset()
					continue
				}
				buff.WriteByte(b)
			}
		}
		return fmt.Errorf("/!\\ Init timeout: %q", buff.String())
	})

	p.Write([]byte("STBR" + strconv.Itoa(to) + "\r"))
	time.Sleep(15 * time.Millisecond)
	mode.BaudRate = to
	p.SetMode(mode)

	if err := errg.Wait(); err != nil {
		return err
	}
	p.Write([]byte("\r"))
	p.ResetInputBuffer()
	return nil
}

func (cu *STN) setCANrate(rate float64) error {
	switch rate {
	case 33.3: //MX ony feature
		cu.protocol = "STP61"
		cu.canrate = "STCSWM2"
	case 500:
		cu.protocol = "STP33"
	case 615.384:
		cu.protocol = "STP33"
		cu.canrate = "STCTR8101FC"
	default:
		return fmt.Errorf("/!\\ Unhandled CANBus rate: %f", rate)
	}
	return nil
}

func (cu *STN) setCANfilter(ids []uint32) {
	var filt uint32 = 0xFFF
	var mask uint32 = 0x000
	for _, id := range ids {
		filt &= id
		mask |= id
	}
	mask = (^mask & 0x7FF) | filt
	cu.filter = fmt.Sprintf("ATCF%03X", filt)
	cu.mask = fmt.Sprintf("ATCM%03X", mask)
}

func (cu *STN) Recv() <-chan gocan.CANFrame {
	return cu.recv
}

func (cu *STN) Send() chan<- gocan.CANFrame {
	return cu.send
}

func (cu *STN) Close() error {
	cu.closed = true
	//cu.close <- token{}
	time.Sleep(100 * time.Millisecond)
	cu.port.Write([]byte("ATZ\r"))
	time.Sleep(100 * time.Millisecond)
	cu.port.ResetInputBuffer()

	return cu.port.Close()
}

func (cu *STN) sendManager(ctx context.Context) {
	f := bytes.NewBuffer(nil)
	for {
		select {
		case v := <-cu.send:
			switch v.(type) {
			default:
				idb := make([]byte, 4)
				binary.BigEndian.PutUint32(idb, v.Identifier())

				t := v.Type()
				r := t.GetResponseCount()

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
				f.WriteString("r:" + strconv.Itoa(r) + "\r")
			}
			cu.sendMutex <- token{}
			if debug {
				cu.cfg.OutputFunc("<o> " + f.String())
			}
			_, err := cu.port.Write(f.Bytes())
			if err != nil {
				cu.cfg.ErrorFunc(fmt.Errorf("failed to write to com port: %q, %w", f.String(), err))
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-cu.close:
			return
		}
	}
}

func (cu *STN) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 21)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			if !cu.closed {
				cu.cfg.ErrorFunc(fmt.Errorf("failed to read com port: %w", err))
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
				case <-cu.sendMutex:
				default:
				}
				continue
			}

			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if debug {
					cu.cfg.OutputFunc("<i> " + buff.String())
				}
				switch buff.String() {
				case "CAN ERROR":
					cu.cfg.ErrorFunc(errors.New("CAN ERROR"))
					buff.Reset()
				case "STOPPED":
					buff.Reset()
				case "?":
					cu.cfg.ErrorFunc(errors.New("UNKNOWN COMMAND"))
					buff.Reset()
				case "NO DATA":
					buff.Reset()
				case "OK":
					buff.Reset()
				default:
					f, err := cu.decodeFrame(buff.Bytes())
					if err != nil {
						cu.cfg.ErrorFunc(fmt.Errorf("failed to decode frame: %s %w", buff.String(), err))
						continue
					}
					select {
					case cu.recv <- f:
					default:
						cu.cfg.ErrorFunc(fmt.Errorf("dropped frame: %s", f.String()))
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
	id, err := strconv.ParseUint(string(buff[0:3]), 16, 32)
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
