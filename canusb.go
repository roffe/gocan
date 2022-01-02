package canusb

import (
	"bytes"
	"fmt"
	"log"
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
	send                 chan Outgoing
	recvBytes, sentBytes uint64
	errors               uint64
	dropped              uint64
	hub                  *Hub
}

type Outgoing interface {
	Send(c *Canusb) error
}

func New(opts ...Opts) (*Canusb, error) {
	c := &Canusb{
		send: make(chan Outgoing, 100),
		hub:  newHub(),
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	c.initCom()
	go c.run()
	return c, nil
}

func (c *Canusb) run() {
	go c.hub.run()
	go c.recvManager()
	go c.sendManager()
	go func() {
		for {
			time.Sleep(5 * time.Second)
			c.SendString("F")
		}
	}()
}

func (c *Canusb) recvManager() {
	buff := bytes.NewBuffer([]byte{})
	readBuffer := make([]byte, 16)
	for {
		n, err := c.port.Read(readBuffer)
		if err != nil {
			log.Println(err)
			break
		}
		if n == 0 {
			log.Println("stopped consuming due to 0 byte read")
			break
		}
		atomic.AddUint64(&c.recvBytes, uint64(n))

		for _, b := range readBuffer[:n] {
			buff.WriteByte(b)
			if b == 0x0D {
				if buff.Len() == 1 {
					buff.Reset()
					continue
				}
				b := buff.Bytes()
				switch b[0] {
				case 'F':
					if err := checkStatus(b); err != nil {
						log.Fatal("can status error", err)
					}
				case 't':
					f := parseFrame(buff)
					select {
					case c.hub.incoming <- f:
					default:
						atomic.AddUint64(&c.dropped, 1)
					}
				case 'z':
					//fmt.Println("ok")
				case 0x07:
					atomic.AddUint64(&c.errors, 1)
					log.Println("received error response")
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

func (c *Canusb) sendManager() {
	for v := range c.send {
		if err := v.Send(c); err != nil {
			log.Printf("failed to send: %v\n", err)
		}
	}
}

func (c *Canusb) initCom() {
	var init = []B{
		{CR, CR, CR},        // Empty buffer
		{'V', CR},           // Get Version number of both CANUSB hardware and software
		{'N', CR},           // Get Serial number of the CANUSB
		B("Z0\r"),           // Sets Time Stamp OFF for received frames
		B(c.canrate + "\r"), // Setup CAN bit-rates
		//B("M00004000\r"),
		//B("m00000FF0\r"),
		{'O', CR}, // Open the CAN channel
	}

	for _, i := range init {
		if _, err := c.writeCom(i); err != nil {
			log.Fatal(err)
		}
	}
}

func (c *Canusb) writeCom(data []byte) (int, error) {
	n, err := c.port.Write(data)
	if err != nil {
		return 0, fmt.Errorf("error writing to port: %v", err)
	}
	return n, nil
}

func (c *Canusb) Stop() error {
	c.port.Write(B("C\r"))
	time.Sleep(100 * time.Millisecond)
	return c.port.Close()
}

// Returns a channel subscribed to all identifiers
func (c *Canusb) Read() <-chan *Frame {
	callbackChan := make(chan *Frame, 1)
	c.hub.register <- &Poll{identifier: 0, callback: callbackChan}
	return callbackChan
}

// Shortcommand to send a standard 11bit frame
func (c *Canusb) SendFrame(identifier uint16, data []byte) {
	waitChan := make(chan struct{})
	err := c.Send(&Frame{
		Identifier: identifier,
		Len:        uint8(len(data)),
		Data:       data,
		processed:  waitChan,
	})
	if err != nil {
		log.Fatal(err)
	}
	<-waitChan
}

// SendString is used to bypass the frame parser and send raw commands to the CANUSB adapter
func (c *Canusb) SendString(str string) {
	waitChan := make(chan struct{})
	err := c.Send(&rawCommand{
		data:      str,
		processed: waitChan,
	})
	if err != nil {
		log.Fatal(err)
	}
	<-waitChan
}

func (c *Canusb) Send(msg Outgoing) error {
	select {
	case c.send <- msg:
		return nil
	default:
		return fmt.Errorf("send channel full")
	}
}
