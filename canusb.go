package canusb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

const (
	CR = 0x0D
)

type B []byte

type Canusb struct {
	canrate              string
	port                 Port
	send                 chan CANFrame
	recvBytes, sentBytes uint64
	errors               uint64
	dropped              uint64
	hub                  *Hub
	filter               []uint32
	close                chan struct{}
	closed               bool
	logging              bool
}

type Port interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}

type CANFrame interface {
	Byte() []byte
}

func New(ctx context.Context, filters []uint32, opts ...Opts) (*Canusb, error) {
	c := &Canusb{
		send:  make(chan CANFrame, 100),
		hub:   newHub(),
		close: make(chan struct{}, 16),
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	var ready sync.WaitGroup
	ready.Add(3)
	go c.run(ctx, &ready)
	ready.Wait()
	c.initAdapter()
	go func() {
		for ctx.Err() == nil {
			<-time.After(5 * time.Second)
			c.Send(&RawCommand{Data: "F"})
		}
	}()
	return c, nil
}

func (c *Canusb) Close() error {
	c.closed = true
	close(c.close)
	time.Sleep(50 * time.Millisecond)
	return nil
}

func (c *Canusb) run(ctx context.Context, ready *sync.WaitGroup) {
	go c.hub.run(ctx, ready)
	go c.recvManager(ctx, ready)
	go c.sendManager(ctx, ready)
}

func (c *Canusb) recvManager(ctx context.Context, wg *sync.WaitGroup) {
	wg.Done()
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 8)
	c.port.(serial.Port).SetReadTimeout(2 * time.Millisecond)
	//x.SetReadTimeout(10 * time.Millisecond)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := c.port.Read(readBuffer)
		if err != nil {
			if (strings.Contains(err.Error(), "Port has been closed") && ctx.Err() != nil) || c.closed {
				break
			}
			log.Fatalf("failed to read com port: %v", err)
		}
		if n == 0 {
			//log.Println("comport 0 byte read")
			continue
		}

		atomic.AddUint64(&c.recvBytes, uint64(n))

		for _, b := range readBuffer[:n] {
			select {
			case <-ctx.Done():
				return
			default:
			}
			buff.WriteByte(b)
			if b == 0x0D {
				if buff.Len() == 1 {
					buff.Reset()
					continue
				}
				by := buff.Bytes()
				switch by[0] {
				case 'F':
					if err := decodeStatus(by); err != nil {
						log.Fatal("CAN status error", err)
					}
				case 't':
					f, err := c.decodeFrame(buff.Bytes())
					if err != nil {
						log.Fatalf("failed to decode frame: %q\n", buff.String())
						continue
					}
					select {
					case c.hub.incoming <- f:
					default:
						atomic.AddUint64(&c.dropped, 1)
					}
					buff.Reset()

				case 'z':
					//fmt.Println("ok")
				case 0x07:
					atomic.AddUint64(&c.errors, 1)
					//log.Println("received error response")
				case 'V':
					log.Println("   H/W version", string(b))
				case 'N':
					log.Println("   H/W serial ", string(b))
				default:
					log.Printf("COM>> %q\n", string(b))
				}
				buff.Reset()
			}
		}
	}
}

func (*Canusb) decodeFrame(buff []byte) (*Frame, error) {
	received := time.Now()
	p := strings.ReplaceAll(string(buff), "\r", "")
	idBytes, err := hex.DecodeString(fmt.Sprintf("%08s", p[1:4]))
	if err != nil {
		return nil, fmt.Errorf("filed to decode identifier: %v", err)
	}
	len, err := strconv.ParseUint(string(p[4:5]), 0, 8)
	if err != nil {
		log.Fatal(err)
	}
	data, err := hex.DecodeString(p[5:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}
	return &Frame{
		Time:       received,
		Identifier: binary.BigEndian.Uint32(idBytes),
		Len:        uint8(len),
		Data:       data,
	}, nil
}

func (c *Canusb) sendManager(ctx context.Context, wg *sync.WaitGroup) {
	wg.Done()
	//f, _ := os.Create("canlog.log")
outer:
	for {
		select {
		case v := <-c.send:
			//log.Printf("%s\n", v.Byte())
			n, err := c.port.Write(v.Byte())
			if err != nil {
				log.Printf("failed to write to com port: %q, %v\n", string(v.Byte()), err)
			}
			atomic.AddUint64(&c.sentBytes, uint64(n))
			//ff, ok := v.(*Frame)
			//if ok {
			//	f.WriteString(ff.String())
			//	f.WriteString("\n")
			//}
		case <-ctx.Done():
			break outer
		case <-c.close:
			break outer

		}
	}
	c.port.Write(B("C\r"))
	c.port.Write(B("\r\r\r"))
	if err := c.port.Close(); err != nil {
		log.Println("port close error: ", err)
	}
}

func CalcAcceptanceFilters(idList ...uint32) (string, string) {
	var code uint32 = ^uint32(0)
	var mask uint32 = 0
	if len(idList) == 0 {
		code = 0
		mask = ^uint32(0)
	} else {
		for _, canID := range idList {
			if canID == 0x00 {
				log.Println("Found illegal id: ", canID)
				code = 0
				mask = 0
				break
			}
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16

	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

func (c *Canusb) initAdapter() {
	code, mask := CalcAcceptanceFilters(c.filter...) //0x6b1, 0x3A0
	//	log.Println(code, mask)
	var cmds = []string{
		"\r\r",    // Empty buffer
		"V",       // Get Version number of both CANUSB hardware and software
		"N",       // Get Serial number of the CANUSB
		"Z0",      // Sets Time Stamp OFF for received frames
		c.canrate, // Setup CAN bit-rates
		code,
		mask,
		"O", // Open the CAN channel
	}
	log.Println(code, mask)
	for _, cmd := range cmds {
		c.Send(&RawCommand{
			Data: cmd,
		})
	}
	time.Sleep(200 * time.Millisecond)
}

//func (c *Canusb) Monitor(ctx context.Context, identifiers ...uint32) {
//	for {
//		select {
//		case f := <-c.Read():
//			if len(identifiers) == 0 {
//				log.Println(f.String())
//			}
//			for _, id := range identifiers {
//				if f.Identifier == id || id == 0x00 {
//					log.Println(f.String())
//				}
//			}
//		case <-ctx.Done():
//			return
//		}
//	}
//}

// Returns a channel subscribed to all identifiers
//func (c *Canusb) Read() <-chan *Frame {
//	callbackChan := make(chan *Frame, 1)
//	c.hub.register <- &Poll{identifier: 0, callback: callbackChan}
//	return callbackChan
//}

// Send a CAN Frame
func (c *Canusb) Send(msg CANFrame) error {
	select {
	case c.send <- msg:
		return nil
	default:
		log.Println("oofh")
		return fmt.Errorf("outgoing queue full")
	}
}

// Shortcommand to send a standard 11bit frame
func (c *Canusb) SendFrame(identifier uint32, data []byte) error {
	var b = make([]byte, 8)
	copy(b, data)
	return c.Send(&Frame{
		Identifier: identifier,
		Len:        uint8(len(b)),
		Data:       b,
	})
}

// SendString is used to bypass the frame parser and send raw commands to the CANUSB adapter
func (c *Canusb) SendString(str string) error {
	return c.Send(&RawCommand{Data: str})
}
