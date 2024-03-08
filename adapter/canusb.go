package adapter

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
	"sync"
	"time"

	"github.com/albenik/bcd"
	"github.com/roffe/gocan"
	"go.bug.st/serial"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               "CANUSB",
		Description:        "Lawicell CANUSB",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: true,
		},
		New: NewCanusb,
	}); err != nil {
		panic(err)
	}
}

type Canusb struct {
	cfg          *gocan.AdapterConfig
	port         serial.Port
	canRate      string
	filter, mask string
	send, recv   chan gocan.CANFrame
	close        chan struct{}

	//sendMutex chan token
	closeOnce sync.Once
	mu        sync.Mutex
}

func NewCanusb(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	cu := &Canusb{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 30),
		close: make(chan struct{}, 1),
		//sendMutex: make(chan token, 1),
	}
	if err := cu.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}
	cu.filter, cu.mask = cu.calcAcceptanceFilters(cfg.CANFilter)
	return cu, nil
}

func (cu *Canusb) Name() string {
	return "CANUSB"
}

func (cu *Canusb) SetFilter(filters []uint32) error {
	filter, mask := cu.calcAcceptanceFilters(filters)
	cu.send <- gocan.NewRawCommand("C")
	cu.send <- gocan.NewRawCommand(filter)
	cu.send <- gocan.NewRawCommand(mask)
	cu.send <- gocan.NewRawCommand("O")
	return nil
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
	p.SetReadTimeout(2 * time.Millisecond)
	cu.port = p

	var cmds = []string{
		"C", "", "", // Empty buffer
		"V", // Get Version number of both CANUSB hardware and software
		//"N",        // Get Serial number of the CANUSB
		"Z0",       // Sets Time Stamp OFF for received frames
		cu.canRate, // Setup CAN bit-rates
		cu.filter,
		cu.mask,
		"O", // Open the CAN channel
	}

	delay := time.Duration(2147483647 / mode.BaudRate)
	if delay > (100 * time.Millisecond) {
		delay = 100 * time.Millisecond
	}

	for _, c := range cmds {
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			p.Close()
			return err
		}
		time.Sleep(delay)
	}

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

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
	case 33.3:
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
	var err error
	cu.closeOnce.Do(func() {
		_, err = cu.port.Write([]byte("C\r"))
		if err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
		close(cu.close)
		time.Sleep(10 * time.Millisecond)
		err = cu.port.ResetInputBuffer()
		if err != nil {
			return
		}
		err = cu.port.ResetOutputBuffer()
		if err != nil {
			return
		}
		err = cu.port.Close()
		if err != nil {
			return
		}
	})
	return err
}

func (*Canusb) calcAcceptanceFilters(idList []uint32) (string, string) {
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

func (cu *Canusb) sendManager(ctx context.Context) {
	t := time.NewTicker(600 * time.Millisecond)
	f := bytes.NewBuffer(nil)
	for {
		select {
		case <-t.C:
			// cu.sendMutex <- token{}
			cu.mu.Lock()
			cu.port.Write([]byte{'F', '\r'})

		case v := <-cu.send:
			switch v.(type) {
			case *gocan.RawCommand:
				//cu.sendMutex <- token{}
				cu.mu.Lock()
				if _, err := cu.port.Write(append(v.Data(), '\r')); err != nil {
					cu.cfg.OnError(fmt.Errorf("failed to write to com port: %s, %w", f.String(), err))
				}
				if cu.cfg.Debug {
					fmt.Fprint(os.Stderr, ">> "+v.String()+"\n")
				}
			default:
				//cu.sendMutex <- token{}
				f.Reset()
				cu.mu.Lock()
				idb := make([]byte, 4)
				binary.BigEndian.PutUint32(idb, v.Identifier())
				f.WriteString("t" + hex.EncodeToString(idb)[5:] +
					strconv.Itoa(v.Length()) +
					hex.EncodeToString(v.Data()) + "\r")
				if _, err := cu.port.Write(f.Bytes()); err != nil {
					cu.cfg.OnError(fmt.Errorf("failed to write to com port: %s, %w", f.String(), err))
				}
				if cu.cfg.Debug {
					fmt.Fprint(os.Stderr, ">> "+f.String()+"\n")
				}
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
	readBuffer := make([]byte, 16)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			var portError *serial.PortError
			if errors.As(err, &portError) {
				log.Println(portError.EncodedErrorString())
				return
			}
			cu.cfg.OnError(fmt.Errorf("failed to read com port: %w", err))
			return
		}
		select {
		case <-cu.close:
			return
		default:
			if n == 0 {
				continue
			}
			cu.parse(ctx, buff, readBuffer[:n])
		}

	}
}

func (cu *Canusb) parse(ctx context.Context, buff *bytes.Buffer, readBuffer []byte) {
	for _, b := range readBuffer {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if b != 0x0D {
			buff.WriteByte(b)
			continue
		}
		//select {
		//case <-cu.sendMutex:
		//default:
		//}
		if buff.Len() == 0 {
			continue
		}
		by := buff.Bytes()
		if cu.cfg.Debug {
			fmt.Fprint(os.Stderr, "<< "+buff.String()+"\n")
		}
		switch by[0] {
		case 'F':
			if err := decodeStatus(by); err != nil {
				cu.cfg.OnError(fmt.Errorf("CAN status error: %w", err))
			}
			cu.mu.Unlock()
		case 't':
			//if cu.cfg.Debug {
			//	fmt.Fprint(os.Stderr, "<< "+buff.String()+"\n")
			//}
			f, err := cu.decodeFrame(by)
			if err != nil {
				cu.cfg.OnError(fmt.Errorf("failed to decode frame: %X", buff.Bytes()))
				continue
			}
			select {
			case cu.recv <- f:
			default:
				cu.cfg.OnError(ErrDroppedFrame)
			}
			buff.Reset()
		case 'z': // last command ok
			//select {
			//case <-cu.sendMutex:
			//default:
			//}
			cu.mu.Unlock()
		case 0x07: // bell, last command was error
		case 'V':
			if cu.cfg.PrintVersion {
				cu.cfg.OnMessage("H/W version " + buff.String())
			}
			//			cu.mu.Unlock()
		case 'N':
			if cu.cfg.PrintVersion {
				cu.cfg.OnMessage("H/W serial " + buff.String())
			}
		default:
			cu.cfg.OnMessage("Unknown>> " + buff.String())
			//if cu.mu.TryLock() {
			//	cu.mu.Unlock()
			//} else {
			//	cu.mu.Unlock()
			//}

		}
		buff.Reset()
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
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	msgLen, err := strconv.Atoi(string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}

	data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return gocan.NewFrame(
		uint32(id),
		data,
		gocan.Incoming,
	), nil
}
