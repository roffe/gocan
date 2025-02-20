package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/albenik/bcd"
	"github.com/roffe/gocan"
	"go.bug.st/serial"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               "CANUSB VCP",
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
	BaseAdapter
	port         serial.Port
	canRate      string
	filter, mask string
	buff         *bytes.Buffer
	// mu        sync.Mutex
}

func NewCanusb(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	cu := &Canusb{
		BaseAdapter: NewBaseAdapter("CANUSB VCP", cfg),
		buff:        bytes.NewBuffer(nil),
		//sendMutex: make(chan token, 1),
	}
	if err := cu.setCANrate(cfg.CANRate); err != nil {
		return nil, err
	}
	cu.filter, cu.mask = cu.calcAcceptanceFilters(cfg.CANFilter)
	return cu, nil
}

func (cu *Canusb) SetFilter(filters []uint32) error {
	filter, mask := cu.calcAcceptanceFilters(filters)
	cu.Send() <- gocan.NewFrame(gocan.SystemMsg, []byte{'C'}, gocan.Outgoing)
	cu.Send() <- gocan.NewFrame(gocan.SystemMsg, []byte(filter), gocan.Outgoing)
	cu.Send() <- gocan.NewFrame(gocan.SystemMsg, []byte(mask), gocan.Outgoing)
	cu.Send() <- gocan.NewFrame(gocan.SystemMsg, []byte{'O'}, gocan.Outgoing)
	return nil
}

func (cu *Canusb) Connect(ctx context.Context) error {
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
	p.SetReadTimeout(4 * time.Millisecond)
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

	for _, c := range cmds {
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			p.Close()
			return err
		}
		time.Sleep(10 * time.Millisecond)
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

func (cu *Canusb) Close() error {
	cu.BaseAdapter.Close()
	if cu.port != nil {
		cu.port.Write([]byte("C\r"))
		cu.port.ResetInputBuffer()
		cu.port.ResetOutputBuffer()
		if err := cu.port.Close(); err != nil {
			return fmt.Errorf("failed to close com port: %w", err)
		}
		cu.port = nil
	}
	return nil
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
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	fb := bytes.NewBuffer(nil)
	for {
		select {
		case <-ctx.Done():
			return
		case <-cu.closeChan:
			return
		case <-ticker.C:
			// cu.mu.Lock()
			cu.port.Write([]byte{'F', '\r'})
		case v := <-cu.sendChan:
			if id := v.Identifier(); id >= gocan.SystemMsg {
				if id == gocan.SystemMsg {
					if cu.cfg.Debug {
						cu.cfg.OnMessage(">> " + string(v.Data()))
					}
					if _, err := cu.port.Write(append(v.Data(), '\r')); err != nil {
						cu.SetError(gocan.Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
						return
					}
				}
				continue
			}
			fb.Reset()
			// cu.mu.Lock()
			idb := make([]byte, 4)
			binary.BigEndian.PutUint32(idb, v.Identifier())
			fb.WriteString("t" + hex.EncodeToString(idb)[5:] +
				strconv.Itoa(v.Length()) +
				hex.EncodeToString(v.Data()) + "\r")
			if _, err := cu.port.Write(fb.Bytes()); err != nil {
				cu.SetError(gocan.Unrecoverable(fmt.Errorf("failed to write to com port: %s, %w", fb.String(), err)))
				return
			}
			if cu.cfg.Debug {
				cu.cfg.OnMessage(">> " + fb.String())
			}

		}
	}
}

func (cu *Canusb) recvManager(ctx context.Context) {
	readBuffer := make([]byte, 32)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			//var portError *serial.PortError
			//if errors.As(err, &portError) {
			//	log.Println(portError.EncodedErrorString())
			//	return
			//}
			cu.SetError(fmt.Errorf("failed to read com port: %w", err))
			return
		}
		select {
		case <-cu.closeChan:
			return
		default:
			if n == 0 {
				continue
			}
			cu.parse(readBuffer[:n])
		}

	}
}

func (cu *Canusb) parse(data []byte) {
	for _, b := range data {
		if b == 0x07 { // BELL
			cu.cfg.OnMessage("command error")
			// cu.mu.Unlock()
			continue
		}
		if b != 0x0D { // CR
			cu.buff.WriteByte(b)
			continue
		}
		if cu.buff.Len() == 0 {
			continue
		}
		by := cu.buff.Bytes()
		if cu.cfg.Debug {
			cu.cfg.OnMessage("<< " + cu.buff.String())
		}
		switch by[0] {
		case 'F':
			if err := decodeStatus(by); err != nil {
				cu.cfg.OnMessage(fmt.Sprintf("CAN status error: %v", err))
				cu.SetError(err)
			}
			//cu.mu.Unlock()
		case 't':
			f, err := cu.decodeFrame(by)
			if err != nil {
				cu.cfg.OnMessage(fmt.Sprintf("failed to decode frame: %v", err))
				cu.buff.Reset()
				continue
			}
			select {
			case cu.recvChan <- f:
			default:
				cu.SetError(ErrDroppedFrame)
			}
			cu.buff.Reset()
		case 'T':
			f, err := cu.decodeFrame29bit(by)
			if err != nil {
				cu.cfg.OnMessage("failed to decode frame: " + err.Error())
				cu.buff.Reset()
				continue
			}
			select {
			case cu.recvChan <- f:
			default:
				cu.cfg.OnMessage(ErrDroppedFrame.Error())
			}
			cu.buff.Reset()
		case 'z': // last command ok
			//cu.mu.Unlock()
		case 'V':
			if cu.cfg.PrintVersion {
				cu.cfg.OnMessage("H/W version " + cu.buff.String())
			}
		case 'N':
			if cu.cfg.PrintVersion {
				cu.cfg.OnMessage("H/W serial " + cu.buff.String())
			}
		default:
			cu.cfg.OnMessage("Unknown>> " + cu.buff.String())
			/*
				if cu.mu.TryLock() {
					log.Println("was unlocked")
					cu.mu.Unlock()
				} else {
					log.Println("was locked")
					cu.mu.Unlock()
				}
			*/

		}
		cu.buff.Reset()
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
		return errors.New("error warning (EI)")
	case checkBitSet(bs, 4):
		return errors.New("data overrun (DOI)") // see SJA1000 datasheet
	case checkBitSet(bs, 5):
		return errors.New("not used")
	case checkBitSet(bs, 6):
		return errors.New("error passive (EPI)") // see SJA1000 datasheet
	case checkBitSet(bs, 7):
		return errors.New("arbitration lost (ALI)") // see SJA1000 datasheet *
	case checkBitSet(bs, 8):
		return errors.New("bus error (BEI)") // see SJA1000 datasheet **"

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

	/* leng, err := hex.DecodeString("0" + string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}
	msgLen := int(leng[0])
	if msgLen > 8 {
		log.Println("msgLen", msgLen)
	} */

	//data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))

	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return gocan.NewFrame(
		uint32(id),
		data,
		gocan.Incoming,
	), nil
}

// T 00000180 8 2D 12 09 DF 87 56 91 06
func (*Canusb) decodeFrame29bit(buff []byte) (gocan.CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:9]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	/* leng, err := hex.DecodeString("0" + string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}
	msgLen := int(leng[0])
	if msgLen > 8 {
		log.Println("msgLen", msgLen)
	} */

	//data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))

	data, err := hex.DecodeString(string(buff[10:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return gocan.NewFrame(
		uint32(id),
		data,
		gocan.Incoming,
	), nil
}
