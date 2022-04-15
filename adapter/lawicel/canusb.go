package lawicel

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/albenik/bcd"
	"github.com/roffe/gocan"
	"go.bug.st/serial"
)

var debug bool

func init() {
	if strings.ToLower(os.Getenv("DEBUG")) == "true" {
		debug = true
	}
}

type Canusb struct {
	cfg          *gocan.AdapterConfig
	port         serial.Port
	canRate      string
	filter, mask string
	send, recv   chan gocan.CANFrame
	close        chan struct{}
	closed       bool
}

func NewCanusb(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	cu := &Canusb{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 100),
		recv:  make(chan gocan.CANFrame, 100),
		close: make(chan struct{}, 1),
	}
	if err := cu.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}
	cu.filter, cu.mask = calcAcceptanceFilters(cfg.CANFilter)
	return cu, nil
}

func (cu *Canusb) Init(ctx context.Context) error {
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

	var cmds = []string{
		"\r", "\r", // Empty buffer
		"V", // Get Version number of both CANUSB hardware and software
		//"N",        // Get Serial number of the CANUSB
		"Z0",       // Sets Time Stamp OFF for received frames
		cu.canRate, // Setup CAN bit-rates
		cu.filter,
		cu.mask,
		"O", // Open the CAN channel
	}

	delay := time.Duration(5000000000000 / mode.BaudRate)
	if delay > (100 * time.Millisecond) {
		delay = 100 * time.Millisecond
	}

	go cu.recvManager(ctx)

	for _, c := range cmds {
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			p.Close()
			return err
		}
		time.Sleep(delay)
	}

	go cu.sendManager(ctx)

	return nil
}

func (cu *Canusb) setCANrate(rate float64) error {
	switch rate {
	case 10:
		cu.canRate = "S0"
	case 20:
		cu.canRate = "S1"
	case 33:
		cu.canRate = "s0e1c"
	case 47.619:
		cu.canRate = "scb9a"
	case 50:
		cu.canRate = "S2"
	case 100:
		cu.canRate = "S3"
	case 125:
		cu.canRate = "S4"
	case 250:
		cu.canRate = "S5"
	case 500:
		cu.canRate = "S6"
	case 615.384:
		cu.canRate = "s4037"
	case 800:
		cu.canRate = "S7"
	case 1000:
		cu.canRate = "S8"
	default:
		return fmt.Errorf("unknown rate: %f", rate)

	}
	return nil
}

func (cu *Canusb) Recv() <-chan gocan.CANFrame {
	return cu.recv
}

func (cu *Canusb) Send() chan<- gocan.CANFrame {
	return cu.send
}

func (cu *Canusb) Close() error {
	cu.closed = true
	cu.close <- struct{}{}
	time.Sleep(50 * time.Millisecond)
	cu.port.Write([]byte("C\r"))
	cu.port.Write([]byte("\r\r\r"))
	time.Sleep(10 * time.Millisecond)
	return cu.port.Close()
}

func calcAcceptanceFilters(idList []uint32) (string, string) {
	if len(idList) == 1 && idList[0] == 0 {
		return "\r", "\r"
	}
	var code uint32 = ^uint32(0)
	var mask uint32 = 0
	if len(idList) == 0 {
		code = 0
		mask = ^uint32(0)
	} else {
		for _, canID := range idList {
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16

	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

type token struct{}

var sendMutex = make(chan token, 1)

func (cu *Canusb) sendManager(ctx context.Context) {
	t := time.NewTicker(600 * time.Millisecond)
	f := bytes.NewBuffer(nil)
	for {
		select {
		case <-t.C:
			if !cu.closed {
				sendMutex <- token{}
				cu.port.Write([]byte{'F', '\r'})
			}

		case v := <-cu.send:
			idb := make([]byte, 4)
			binary.BigEndian.PutUint32(idb, v.Identifier())
			f.WriteString("t" + hex.EncodeToString(idb)[5:] +
				strconv.Itoa(v.Length()) +
				hex.EncodeToString(v.Data()) + "\r")
			sendMutex <- token{}
			_, err := cu.port.Write(f.Bytes())
			if err != nil {
				log.Printf("failed to write to com port: %q, %v", f.String(), err)
			}
			if debug {
				fmt.Fprint(os.Stderr, ">> "+f.String()+"\n")
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-cu.close:
			return
		}
	}
}

func (cu *Canusb) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 8)
	for {
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
		cu.parse(ctx, readBuffer[:n], buff)
	}
}

func (cu *Canusb) parse(ctx context.Context, readBuffer []byte, buff *bytes.Buffer) {
	for _, b := range readBuffer {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if b == 0x0D {
			select {
			case <-sendMutex:
			default:
			}
			if buff.Len() == 0 {
				continue
			}
			by := buff.Bytes()
			switch by[0] {
			case 'F':
				if err := decodeStatus(by); err != nil {
					log.Println("CAN status error", err)
				}
			case 't':
				if debug {
					fmt.Fprint(os.Stderr, "<< "+buff.String()+"\n")
				}
				f, err := cu.decodeFrame(by)
				if err != nil {
					log.Printf("failed to decode frame: %q\n", buff.String())
					continue
				}
				select {
				case cu.recv <- f:
				default:
					log.Println("dropped frame")
				}

				buff.Reset()

			case 'z': // last command ok
			case 0x07: // bell, last command was error
			case 'V':
				log.Println("H/W version", buff.String())
			case 'N':
				log.Println("H/W serial ", buff.String())
			default:
				log.Printf("COM>> %q\n", buff.String())
			}
			buff.Reset()
			continue
		}
		buff.WriteByte(b)
	}
}

/*
Bit 0 CAN receive FIFO queue full
Bit 1 CAN transmit FIFO queue full
Bit 2 Error warning (EI), see SJA1000 datasheet
Bit 3 Data Overrun (DOI), see SJA1000 datasheet
Bit 4 Not used.
Bit 5 Error Passive (EPI), see SJA1000 datasheet
Bit 6 Arbitration Lost (ALI), see SJA1000 datasheet *
Bit 7 Bus Error (BEI), see SJA1000 datasheet **
* Arbitration lost doesn’t generate a blinking RED light!
** Bus Error generates a constant RED ligh
*/

func decodeStatus(b []byte) error {
	bs := int(bcd.ToUint16(b[1:]))
	//log.Printf("%08b\n", bs)
	switch true {
	case checkBitSet(bs, 1):
		return errors.New("CAN receive FIFO queue full")
	case checkBitSet(bs, 2):
		return errors.New("CAN transmit FIFO queue full")
	case checkBitSet(bs, 3):
		return errors.New("error warning (EI), see SJA1000 datasheet")
	case checkBitSet(bs, 4):
		return errors.New("data Overrun (DOI), see SJA1000 datasheet")
	case checkBitSet(bs, 5):
		return errors.New("not used")
	case checkBitSet(bs, 6):
		return errors.New("error Passive (EPI), see SJA1000 datasheet")
	case checkBitSet(bs, 7):
		return errors.New("arbitration Lost (ALI), see SJA1000 datasheet *")
	case checkBitSet(bs, 8):
		return errors.New("bus Error (BEI), see SJA1000 datasheet **")

	}
	return nil
}

func checkBitSet(n, k int) bool {
	v := n & (1 << (k - 1))
	return v == 1
}

func (*Canusb) decodeFrame(buff []byte) (gocan.CANFrame, error) {
	idBytes, err := hex.DecodeString(fmt.Sprintf("%08s", buff[1:4]))
	if err != nil {
		return nil, fmt.Errorf("filed to decode identifier: %v", err)
	}
	recvBytes := len(buff[5:])

	leng, err := strconv.ParseUint(string(buff[4:5]), 0, 8)
	if err != nil {
		return nil, err
	}

	if uint64(recvBytes/2) != leng {
		return nil, errors.New("frame received bytes does not match header")
	}

	var data = make([]byte, hex.DecodedLen(recvBytes))
	if _, err := hex.Decode(data, buff[5:]); err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}

	return gocan.NewFrame(
		binary.BigEndian.Uint32(idBytes),
		data,
		gocan.Incoming,
	), nil
}

/*
func (*Canusb) decodeFrame(buff []byte) (*gocan.Frame, error) {
	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}

	if int(buff[4]-0x30) != len(buff[5:])/2 {
		return nil, errors.New("frame received bytes does not match header")
	}

	id := uint32(buff[1]-0x30)<<8 | uint32(buff[2]-0x30)<<4 | uint32(buff[3]-0x30)

	return gocan.NewFrame(
		id,
		data,
		gocan.Incoming,
	), nil
}
*/
