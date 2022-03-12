package obdlink

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
	"go.bug.st/serial"
	"golang.org/x/sync/errgroup"
)

var debug bool

func init() {
	if strings.ToLower(os.Getenv("DEBUG")) == "true" {
		debug = true
	}
}

type SX struct {
	cfg          *gocan.AdapterConfig
	port         serial.Port
	canrate      string
	protocol     string
	send, recv   chan gocan.CANFrame
	close        chan struct{}
	closed       bool
	filter, mask string
}

func NewSX(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	sx := &SX{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 100),
		recv:  make(chan gocan.CANFrame, 100),
		close: make(chan struct{}, 10),
	}

	if err := sx.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}

	sx.setCANfilter(cfg.CANFilter)

	return sx, nil
}

var adapterSpeeds = []int{115200, 230400, 921600, 2000000, 1000000, 57600, 38400}

func (cu *SX) Init(ctx context.Context) error {
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
		for _, from := range adapterSpeeds {
			if err := setSpeed(p, mode, from, to); err == nil {
				log.Printf("Switched adapter baudrate from %d to %d bps", from, to)
				return nil
			} else {
				log.Println(err)
			}
		}
		return errors.New("/!\\ Could not init adapter")
	},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry #%d: %v", n, err)
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
		cu.filter, // code
		cu.mask,   // mask
	}

	delay := time.Duration(5000000000000 / mode.BaudRate)
	if delay > (100 * time.Millisecond) {
		delay = 100 * time.Millisecond
	}

	time.Sleep(delay)

	for _, c := range initCmds {
		if c == "" {
			continue
		}
		out := []byte(c + "\r")
		if debug {
			log.Println(c)
		}
		_, err := p.Write(out)
		if err != nil {
			log.Println(err)
		}
		time.Sleep(delay)
	}
	p.ResetInputBuffer()

	go cu.recvManager(ctx)
	go cu.sendManager(ctx)

	return nil
}

func setSpeed(p serial.Port, mode *serial.Mode, from, to int) error {
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
						log.Printf("%q, %s", buff.String(), time.Since(start).String())
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

func (cu *SX) setCANrate(rate float64) error {
	switch rate {
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

func (cu *SX) setCANfilter(ids []uint32) {
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

func (cu *SX) Recv() <-chan gocan.CANFrame {
	return cu.recv
}

func (cu *SX) Send() chan<- gocan.CANFrame {
	return cu.send
}

func (cu *SX) Close() error {
	cu.closed = true
	cu.close <- token{}
	time.Sleep(100 * time.Millisecond)
	cu.port.Write([]byte("ATZ\r"))
	time.Sleep(100 * time.Millisecond)
	cu.port.ResetInputBuffer()

	return cu.port.Close()
}

type token struct{}

var sendMutex = make(chan token, 1)

func (cu *SX) sendManager2(ctx context.Context) {
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

				//a := v.Identifier()
				// write cmd and header
				//f.Write([]byte{'S','T','P','X','h', ':',byte(a>>8) + 0x30, (byte(a) >> 4) + 0x30, ((byte(a) << 4) >> 4) + 0x30, ','})
				//addr := strings.ToUpper(hex.EncodeToString(idb)[5:])
				data := strings.ToUpper(hex.EncodeToString(v.Data()))
				f.WriteString(data + " " + fmt.Sprintf("%X", r) + "\r")
			}

			sendMutex <- token{}
			if debug {
				fmt.Fprint(os.Stderr, "<o> "+f.String()+"\n")
			}
			_, err := cu.port.Write(f.Bytes())
			if err != nil {
				log.Printf("failed to write to com port: %q, %v", f.String(), err)
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-cu.close:
			return
		}
	}
}

func (cu *SX) sendManager(ctx context.Context) {
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

			sendMutex <- token{}
			if debug {
				fmt.Fprint(os.Stderr, "<o> "+f.String()+"\n")
			}
			_, err := cu.port.Write(f.Bytes())
			if err != nil {
				log.Printf("failed to write to com port: %q, %v", f.String(), err)
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-cu.close:
			return
		}
	}
}

func (cu *SX) recvManager(ctx context.Context) {
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
				log.Printf("failed to read com port: %v", err)
			}
			return
		}
		if n == 0 {
			continue
		}
		//		log.Printf("%q", readBuffer[:n])
		for _, b := range readBuffer[:n] {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if b == '>' {
				select {
				case <-sendMutex:
				default:
				}
				continue
			}

			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if debug {
					fmt.Fprint(os.Stderr, "<i> ", buff.String()+"\n")
				}
				switch buff.String() {
				case "CAN ERROR":
					log.Println("CAN ERROR")
					buff.Reset()
				case "STOPPED":
					buff.Reset()
				case "?":
					log.Println("UNKNOWN COMMAND")
					buff.Reset()
				case "NO DATA":
					//log.Println("NO DATA")
					buff.Reset()
				case "OK":
					buff.Reset()
				default:
					f, err := cu.decodeFrame(buff.Bytes())
					if err != nil {
						log.Printf("%v: %q\n", err, buff.String())
						continue
					}
					select {
					case cu.recv <- f:
					default:
						log.Println("dropped frame")
					}
					buff.Reset()
				}
				continue
			}
			buff.WriteByte(b)
		}
	}
}

func (*SX) decodeFrame(buff []byte) (gocan.CANFrame, error) {
	idBytes, err := hex.DecodeString(fmt.Sprintf("%08s", buff[0:3]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	data, err := hex.DecodeString(string(buff[3:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}

	return gocan.NewFrame(
		binary.BigEndian.Uint32(idBytes),
		data,
		gocan.Incoming,
	), nil
}

/*func (*SX) decodeFrame(buff []byte) (gocan.CANFrame, error) {
	data, err := hex.DecodeString(string(buff[3:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}

	id := uint32(buff[0]-0x30)<<8 | uint32(buff[1]-0x30)<<4 | uint32(buff[2]-0x30)

	return gocan.NewFrame(
		id,
		data,
		gocan.Incoming,
	), nil
}
*/
