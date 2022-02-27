package lawicel

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

	"github.com/albenik/bcd"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/frame"
	"go.bug.st/serial"
)

type Canusb struct {
	port             serial.Port
	portName         string
	portBaudrate     int
	canRate          string
	canCode, canMask string
	send, recv       chan frame.CANFrame
	close            chan struct{}
	closed           bool
}

func NewCanusb(cfg *gocan.AdapterConfig) (*Canusb, error) {
	cu := &Canusb{
		portName:     cfg.Port,
		portBaudrate: cfg.PortBaudrate,
		send:         make(chan frame.CANFrame, 100),
		recv:         make(chan frame.CANFrame, 100),
		close:        make(chan struct{}, 1),
	}
	if err := cu.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}
	cu.canCode, cu.canMask = calcAcceptanceFilters(cfg.CANFilter)
	return cu, nil
}

func (cu *Canusb) Init(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: cu.portBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(cu.portName, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", cu.portName, err)
	}
	cu.port = p

	var cmds = []string{
		"\r", "\r", "\r", "\r", // Empty buffer
		"V",        // Get Version number of both CANUSB hardware and software
		"N",        // Get Serial number of the CANUSB
		"Z0",       // Sets Time Stamp OFF for received frames
		cu.canRate, // Setup CAN bit-rates
		cu.canCode,
		cu.canMask,
		"O", // Open the CAN channel
	}
	p.SetReadTimeout(1 * time.Millisecond)

	delay := time.Duration(5000000000000 / mode.BaudRate)
	if delay > (100 * time.Millisecond) {
		delay = 100 * time.Millisecond
	}

	for _, c := range cmds {
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			p.Close()
			return err
		}
	}

	go func() {
		for ctx.Err() == nil {
			<-time.After(700 * time.Millisecond)
			cu.send <- frame.NewRawCommand("F")
		}
	}()

	go cu.recvManager(ctx)
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

func (cu *Canusb) Recv() <-chan frame.CANFrame {
	return cu.recv
}

func (cu *Canusb) Send() chan<- frame.CANFrame {
	return cu.send
}

func (cu *Canusb) Close() error {
	cu.closed = true
	cu.close <- struct{}{}
	time.Sleep(100 * time.Millisecond)
	cu.port.Write([]byte("C\r"))
	cu.port.Write([]byte("\r\r\r"))
	time.Sleep(10 * time.Millisecond)
	cu.port.ResetInputBuffer()
	cu.port.ResetOutputBuffer()
	return cu.port.Close()
}

func calcAcceptanceFilters(idList []uint32) (string, string) {
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
	f := bytes.NewBuffer(nil)
	for {
		select {
		case v := <-cu.send:
			switch v.(type) {
			case *frame.RawCommand:
				f.WriteString(v.String() + "\r")

			default:
				idb := make([]byte, 4)
				binary.BigEndian.PutUint32(idb, v.Identifier())
				f.WriteString("t" + hex.EncodeToString(idb)[5:] +
					strconv.Itoa(v.Len()) +
					hex.EncodeToString(v.Data()) + "\r")
			}

			sendMutex <- token{}
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
			if buff.Len() == 0 {
				continue
			}
			by := buff.Bytes()
			switch by[0] {
			case 'F':
				if err := decodeStatus(by); err != nil {
					log.Println("CAN status error", err)
				}
				select {
				case <-sendMutex:
				default:
				}
			case 't':
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

			case 'z':
				select {
				case <-sendMutex:
				default:
				}
				//fmt.Println("ok")
			case 0x07:
				select {
				case <-sendMutex:
				default:
				}
			case 'V':
				select {
				case <-sendMutex:
				default:
				}
				//log.Println("H/W version", buff.String())
			case 'N':
				select {
				case <-sendMutex:
				default:
				}
				//log.Println("H/W serial ", buff.String())
			default:
				select {
				case <-sendMutex:
				default:
				}
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

func (*Canusb) decodeFrame(buff []byte) (*frame.Frame, error) {
	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}

	if int(buff[4]-0x30) != len(buff[5:])/2 {
		return nil, errors.New("frame received bytes does not match header")
	}

	id := uint32(buff[1]-0x30)<<8 | uint32(buff[2]-0x30)<<4 | uint32(buff[3]-0x30)

	return frame.New(
		id,
		data,
		frame.Incoming,
	), nil
}
