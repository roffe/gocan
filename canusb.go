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
	CR       = 0x0D
	Trionic5 = 615
)

type B []byte

type Canusb struct {
	canrate              string
	port                 serial.Port
	send                 chan CANFrame
	recvBytes, sentBytes uint64
	errors               uint64
	dropped              uint64
	hub                  *Hub
}

type CANFrame interface {
	Byte() []byte
}

func New(ctx context.Context, opts ...Opts) (*Canusb, error) {
	c := &Canusb{
		send: make(chan CANFrame, 100),
		hub:  newHub(),
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
	return c, nil
}

func (c *Canusb) run(ctx context.Context, ready *sync.WaitGroup) {
	go c.hub.run(ctx, ready)
	go c.recvManager(ctx, ready)
	go c.sendManager(ctx, ready)
	go func() {
		time.Sleep(10 * time.Second)
		for {
			time.Sleep(5 * time.Second)
			c.SendString("F")
		}
	}()
}

func (c *Canusb) recvManager(ctx context.Context, wg *sync.WaitGroup) {
	wg.Done()
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 32)
	for {
		n, err := c.port.Read(readBuffer)
		if err != nil {
			log.Fatal(err)
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		if n == 0 {
			log.Fatal("com-port closed with 0 byte read")
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
				b := buff.Bytes()
				switch b[0] {
				case 'F':
					if err := decodeStatus(b); err != nil {
						log.Fatal("CAN status error", err)
					}
				case 't':
					f, err := c.decodeFrame(buff)
					if err != nil {
						log.Printf("failed to decode frame: %q\n", buff.Bytes())
						buff.Reset()
						continue
					}
					select {
					case c.hub.incoming <- f:
					default:
						atomic.AddUint64(&c.dropped, 1)
					}
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

func (*Canusb) decodeFrame(buff *bytes.Buffer) (*Frame, error) {
	p := strings.ReplaceAll(buff.String(), "\r", "")
	idBytes, err := hex.DecodeString(fmt.Sprintf("%04s", p[1:4]))
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
		Identifier: binary.BigEndian.Uint16(idBytes),
		Len:        uint8(len),
		Data:       data,
	}, nil
}

func (c *Canusb) sendManager(ctx context.Context, wg *sync.WaitGroup) {
	wg.Done()
	for v := range c.send {
		//log.Printf("%s\n", v.Byte())
		n, err := c.port.Write(v.Byte())
		if err != nil {
			log.Printf("failed to write to com port: %q, %v\n", string(v.Byte()), err)
		}
		atomic.AddUint64(&c.sentBytes, uint64(n))
	}
}

func (c *Canusb) initAdapter() {
	var cmds = []string{
		"\r\r\r",         // Empty buffer
		"V\r",            // Get Version number of both CANUSB hardware and software
		"N\r",            // Get Serial number of the CANUSB
		"Z0\r",           // Sets Time Stamp OFF for received frames
		c.canrate + "\r", // Setup CAN bit-rates
		//"M00004000\r",
		//"m00000FF0\r",
		"O\r", // Open the CAN channel
	}
	for _, cmd := range cmds {
		c.Send(&rawCommand{
			data: cmd,
		})
	}
}

func (c *Canusb) Stop() error {
	c.port.Write(B("C\r"))
	c.port.Write(B("\r\r\r"))
	time.Sleep(100 * time.Millisecond)
	return c.port.Close()
}

func (c *Canusb) Monitor(ctx context.Context, identifiers ...uint16) {
	for {
		select {
		case f := <-c.Read():
			for _, id := range identifiers {
				if f.Identifier == id || id == 0x00 {
					log.Println(f.String())
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// Returns a channel subscribed to all identifiers
func (c *Canusb) Read() <-chan *Frame {
	callbackChan := make(chan *Frame, 1)
	c.hub.register <- &Poll{identifier: 0, callback: callbackChan}
	return callbackChan
}

// Send a CAN Frame
func (c *Canusb) Send(msg CANFrame) error {
	select {
	case c.send <- msg:
		return nil
	default:
		return fmt.Errorf("outgoing queue full")
	}
}

// Shortcommand to send a standard 11bit frame
func (c *Canusb) SendFrame(identifier uint16, data []byte) error {
	return c.Send(&Frame{
		Identifier: identifier,
		Len:        uint8(len(data)),
		Data:       data,
	})
}

// SendString is used to bypass the frame parser and send raw commands to the CANUSB adapter
func (c *Canusb) SendString(str string) error {
	return c.Send(&rawCommand{data: str})
}
