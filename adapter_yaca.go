package gocan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/albenik/bcd"
	"go.bug.st/serial"
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "YACA",
		Description:        "Yet Another CANBus Adapter",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewYACA,
	}); err != nil {
		panic(err)
	}
}

type YACA struct {
	BaseAdapter
	port   serial.Port
	closed bool
}

func NewYACA(cfg *AdapterConfig) (Adapter, error) {
	ya := &YACA{
		BaseAdapter: NewBaseAdapter("YACA", cfg),
	}
	return ya, nil
}

func (ya *YACA) SetFilter(filters []uint32) error {
	code, mask := ya.calculateCanFilterCodeAndMask(filters)
	ya.Send() <- NewFrame(SystemMsg, []byte{'C'}, Outgoing)
	ya.Send() <- NewFrame(SystemMsg, []byte(code), Outgoing)
	ya.Send() <- NewFrame(SystemMsg, []byte(mask), Outgoing)
	ya.Send() <- NewFrame(SystemMsg, []byte{'O'}, Outgoing)

	return nil
}

func (ya *YACA) Open(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: ya.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(ya.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", ya.cfg.Port, err)
	}
	p.SetReadTimeout(1 * time.Millisecond)
	ya.port = p

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	switch ya.cfg.CANRate {
	case 33.3:
		p.Write([]byte("S0\r"))
	case 47.619:
		p.Write([]byte("S1\r"))
	case 500:
		p.Write([]byte("S2\r"))
	case 615.384:
		p.Write([]byte("S3\r"))

	}
	time.Sleep(5 * time.Millisecond)

	code, mask := ya.calculateCanFilterCodeAndMask(ya.cfg.CANFilter)

	p.Write([]byte(code + "\r"))
	time.Sleep(5 * time.Millisecond)
	p.Write([]byte(mask + "\r"))
	time.Sleep(5 * time.Millisecond)

	p.Write([]byte("O\r"))

	go ya.recvManager(ctx)
	go ya.sendManager(ctx)

	return nil
}

func (*YACA) calculateCanFilterCodeAndMask(data []uint32) (string, string) {
	var min uint32 = 0xffffffff
	var max uint32 = 0x0
	for _, val := range data {
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}
	bitcount := make([]uint8, 32)
	for _, id := range data {
		for p, bit := range fmt.Sprintf("%032b", id) {
			if bit == '1' {
				bitcount[p]++
			}
		}
	}
	noIds := uint8(len(data))
	var mask uint32
	for i, bit := range bitcount {
		if bit == 0 {
			continue
		}
		if bit > 0 && bit < noIds {
			mask |= 1 << (31 - i)
		}
	}
	code := min<<21 | 0x0000FFFF
	mask = mask<<21 | 0x0000FFFF
	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

func (ya *YACA) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 8)
	for {
		select {
		case <-ya.closeChan:
			return
		case <-ctx.Done():
			return
		default:
		}
		n, err := ya.port.Read(readBuffer)
		if err != nil {
			ya.setError(fmt.Errorf("failed to read from com port: %w", err))
			return
		}
		if n == 0 {
			continue
		}
		ya.parse(ctx, buff, readBuffer[:n])
	}
}

func (ya *YACA) parse(ctx context.Context, buff *bytes.Buffer, readBuffer []byte) {
	for _, b := range readBuffer {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if b == '\n' {
			if buff.Len() == 0 {
				continue
			}
			by := buff.Bytes()
			switch by[0] {
			case 'F':
				if err := ya.decodeStatus(by); err != nil {
					ya.cfg.OnMessage(fmt.Sprintf("CAN status error: %v", err))
				}
			case 't':
				//if cu.cfg.Debug {
				//	fmt.Fprint(os.Stderr, "<< "+buff.String()+"\n")
				//}
				f, err := ya.decodeFrame(by)
				if err != nil {
					ya.cfg.OnMessage(fmt.Sprintf("failed to decode frame: %X", buff.Bytes()))
					continue
				}
				select {
				case ya.recvChan <- f:
				default:
					ya.sendEvent(EventTypeWarning, ErrDroppedFrame.Error())
				}
				buff.Reset()
			case 0x07: // bell, last command was unknown
				ya.cfg.OnMessage("unknown command")
			default:
				ya.cfg.OnMessage("Unknown>> " + buff.String())
			}
			buff.Reset()
			continue
		}
		buff.WriteByte(b)
	}
}

func (ya *YACA) decodeStatus(b []byte) error {
	bs := int(bcd.ToUint16(b[1:]))
	//log.Printf("%08b\n", bs)
	switch {
	case ya.checkBitSet(bs, 1):
		return errors.New("CAN receive FIFO queue full")
	case ya.checkBitSet(bs, 2):
		return errors.New("CAN transmit FIFO queue full")
	case ya.checkBitSet(bs, 3):
		return errors.New("error warning (EI)")
	case ya.checkBitSet(bs, 4):
		return errors.New("data overrun (DOI)") // see SJA1000 datasheet
	case ya.checkBitSet(bs, 5):
		return errors.New("not used")
	case ya.checkBitSet(bs, 6):
		return errors.New("error passive (EPI)") // see SJA1000 datasheet
	case ya.checkBitSet(bs, 7):
		return errors.New("arbitration lost (ALI)") // see SJA1000 datasheet *
	case ya.checkBitSet(bs, 8):
		return errors.New("bus error (BEI)") // see SJA1000 datasheet **"

	}
	return nil
}

func (ya *YACA) checkBitSet(n, k int) bool {
	v := n & (1 << (k - 1))
	return v == 1
}

func (ya *YACA) sendManager(ctx context.Context) {
	f := bytes.NewBuffer(nil)
	for {
		select {
		case v := <-ya.sendChan:
			if id := v.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if ya.cfg.Debug {
						ya.cfg.OnMessage(">> " + string(v.Data))
					}
					if _, err := ya.port.Write(append(v.Data, '\r')); err != nil {
						ya.setError(fmt.Errorf("failed to write to com port: %w", err))
						return
					}
				}
				continue
			}
			idb := make([]byte, 4)
			binary.BigEndian.PutUint32(idb, v.Identifier)
			f.WriteString("t" + hex.EncodeToString(idb)[5:] +
				strconv.Itoa(v.Length()) +
				hex.EncodeToString(v.Data) + "\x0D")
			if _, err := ya.port.Write(f.Bytes()); err != nil {
				ya.cfg.OnMessage(fmt.Sprintf("failed to write to com port: %s, %v", f.String(), err))
			}
			if ya.cfg.Debug {
				fmt.Fprint(os.Stderr, ">> "+f.String()+"\n")
			}
			f.Reset()

		case <-ctx.Done():
			return
		case <-ya.closeChan:
			return
		}
	}
}

func (*YACA) decodeFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("filed to decode identifier: %v", err)
	}
	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}
	return NewFrame(
		uint32(id),
		data,
		Incoming,
	), nil
}

func (ya *YACA) Close() error {
	ya.BaseAdapter.Close()
	ya.closed = true
	time.Sleep(10 * time.Millisecond)
	ya.port.Write([]byte("C\r"))
	time.Sleep(10 * time.Millisecond)
	return ya.port.Close()
}
