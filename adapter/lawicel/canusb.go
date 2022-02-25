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
	"github.com/roffe/gocan/pkg/frame"
	"go.bug.st/serial"
)

type Canusb struct {
	port             serial.Port
	portName         string
	portRate         int
	canRate          string
	canCode, canMask string
	send, recv       chan frame.CANFrame
	close            chan struct{}
	closed           bool
}

func NewCanusb() *Canusb {
	return &Canusb{
		send:  make(chan frame.CANFrame, 100),
		recv:  make(chan frame.CANFrame, 100),
		close: make(chan struct{}, 1),
	}
}

func (cu *Canusb) Init(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: cu.portRate,
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
		"\r\r",     // Empty buffer
		"V",        // Get Version number of both CANUSB hardware and software
		"N",        // Get Serial number of the CANUSB
		"Z0",       // Sets Time Stamp OFF for received frames
		cu.canRate, // Setup CAN bit-rates
		cu.canCode,
		cu.canMask,
		"O", // Open the CAN channel
	}
	p.SetReadTimeout(3 * time.Millisecond)
	for _, c := range cmds {
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			p.Close()
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	go func() {
		for ctx.Err() == nil {
			<-time.After(500 * time.Millisecond)
			cu.Send(frame.NewRawCommand("F"))
		}
	}()

	go cu.recvManager(ctx)
	go cu.sendManager(ctx)

	return nil
}

func (cu *Canusb) SetPort(port string) error {
	cu.portName = port
	return nil
}

func (cu *Canusb) SetPortRate(rate int) error {
	cu.portRate = rate
	return nil
}

func (cu *Canusb) SetCANrate(rate float64) error {
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

func (cu *Canusb) SetCANfilter(ids ...uint32) {
	cu.canCode, cu.canMask = calcAcceptanceFilters(ids...)
}

func (cu *Canusb) Read(data []byte) (int, error) {
	return cu.port.Read(data)
}

func (cu *Canusb) Chan() <-chan frame.CANFrame {
	return cu.recv
}

func (cu *Canusb) Send(frame frame.CANFrame) error {
	select {
	case cu.send <- frame:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("failed to send frame")
	}
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

func calcAcceptanceFilters(idList ...uint32) (string, string) {
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
	for {
		select {
		case v := <-cu.send:
			var b []byte
			switch v.(type) {
			case *frame.RawCommand:
				b = []byte(append(v.Data(), '\r'))
			default:
				b = []byte(fmt.Sprintf("t%03X%d%X\r", v.Identifier(), v.Len(), v.Data()))
			}
			sendMutex <- token{}
			_, err := cu.port.Write(b)
			if err != nil {
				log.Printf("failed to write to com port: %q, %v\n", string(b), err)
			}
		case <-ctx.Done():
			return
		case <-cu.close:
			return
		}
	}
}

func (cu *Canusb) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 10)
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
		cu.parse(ctx, n, readBuffer, buff)

	}
}

func (cu *Canusb) parse(ctx context.Context, n int, readBuffer []byte, buff *bytes.Buffer) {
	for _, b := range readBuffer[:n] {
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
				//log.Println("received error response")
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

	return frame.New(
		binary.BigEndian.Uint32(idBytes),
		data,
		frame.Incoming,
	), nil
}
