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
	"go.bug.st/serial/enumerator"
)

const (
	//CR = "\r"
	CR           = 0x0d
	SaabIBUSRate = 47.619
	SaabPBUSRate = 500
	Trionic5     = 615
)

type Canusb struct {
	//debug                bool
	canrate string
	c       context.CancelFunc
	port    serial.Port
	recv    chan *Frame
	send    chan interface{}
	//send                 chan []byte
	recvBytes, sentBytes uint64
	errors               uint64
	waiters              []*waiter
	sync.Mutex
}

type waiter struct {
	identifier int
	callback   chan *Frame
}

type rawCommand struct {
	data      string
	processed chan struct{}
}

type B []byte

func New(opts ...Opts) (*Canusb, error) {
	//c := new(Canusb)
	c := &Canusb{
		recv: make(chan *Frame, 1000),
		send: make(chan interface{}, 100),
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	rdy := make(chan struct{})
	go c.run(rdy)
	<-rdy
	c.initCom()
	go func() {
		for {
			time.Sleep(5 * time.Second)
			c.SendString("F")
		}
	}()
	return c, nil
}

func (c *Canusb) run(rdy chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	c.c = cancel
	go func() {
		defer cancel()
		defer close(c.recv)
		frame := bytes.NewBuffer([]byte{})
		readBuffer := make([]byte, 1)
		for {
			if ctx.Err() != nil {
				break
			}
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

			frame.Write(readBuffer)
			if readBuffer[0] == 0x0D {
				if frame.Len() == 1 {
					frame.Reset()
					continue
				}
				b := frame.Bytes()
				switch b[0] {
				case 'F':
					if err := checkStatus(b); err != nil {
						log.Fatal("can status error", err)
					}
				case 't':
					f := parseFrame(frame)
					c.Lock()
					var newList []*waiter
					for _, w := range c.waiters {
						if w.identifier == int(f.Identifier) {
							select {
							case w.callback <- f:
							default:
							}
							continue
						}
						newList = append(newList, w)
					}
					c.waiters = newList
					c.recv <- f
					c.Unlock()
				case 'z':
					//fmt.Println("ok")
				case 0x07:
					atomic.AddUint64(&c.errors, 1)
					log.Println("received error response")
				default:
					log.Printf("COM>> %q\n", string(b))
				}
				frame.Reset()
			}
		}
	}()

	go func() {
		defer cancel()
		for v := range c.send {
			if ctx.Err() != nil {
				break
			}
			switch f := v.(type) {
			case *Frame:
				out := fmt.Sprintf("t%x%d%x", f.Identifier, f.Len, f.Data)
				n, err := c.port.Write(B(out + "\r"))
				close(f.processed)
				if err != nil {
					log.Fatal(err)
				}
				c.sentBytes += uint64(n)
				atomic.AddUint64(&c.sentBytes, uint64(n))
			case *rawCommand:
				cmd := f.data + "\r"
				n, err := c.port.Write(B(cmd))
				close(f.processed)
				if err != nil {
					log.Fatal(err)
				}
				c.sentBytes += uint64(n)
				atomic.AddUint64(&c.sentBytes, uint64(n))
			}

		}
	}()

	close(rdy)

}

func (c *Canusb) WaitForFrame(identifier int, timeout time.Duration) (*Frame, error) {
	callbackChan := make(chan *Frame, 1)
	c.Lock()
	c.waiters = append(c.waiters, &waiter{
		identifier: identifier,
		callback:   callbackChan,
	})
	c.Unlock()
	select {
	case f := <-callbackChan:
		return f, nil
	case <-time.After(timeout):
		c.Lock()
		var newList []*waiter
		for _, w := range c.waiters {
			if w.identifier == identifier {
				continue
			}
			newList = append(newList, w)
		}
		c.waiters = newList
		c.Unlock()
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", identifier)
	}
}

func LogOut(f *Frame) {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("0x%x", f.Identifier) + " [")
	for i, b := range f.Data {
		out.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			out.WriteString(" ")
		}
	}
	out.WriteString("]")
	log.Println(out.String())
}

func (c *Canusb) initCom() {
	var init = []B{
		{CR, CR},            // Empty buffer
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

func parseFrame(buff *bytes.Buffer) *Frame {
	p := strings.ReplaceAll(buff.String(), "\r", "")
	b, err := hex.DecodeString(fmt.Sprintf("%04s", p[1:4]))
	if err != nil {
		log.Fatal(err)
	}
	addr := binary.BigEndian.Uint16(b)

	len, err := strconv.ParseUint(string(p[4:5]), 0, 8)
	if err != nil {
		log.Fatal(err)
	}

	data, err := hex.DecodeString(p[5:])
	if err != nil {
		log.Fatal(err)
	}

	return &Frame{
		Identifier: addr,
		Len:        uint8(len),
		Data:       data,
	}

}

func checkBitSet(n, k int) bool {
	v := n & (1 << (k - 1))
	return v == 1
}

func (c *Canusb) Stop() error {
	c.c()
	c.port.Write(B("C\r"))
	time.Sleep(50 * time.Millisecond)
	return c.port.Close()
}

func (c *Canusb) Chan() <-chan *Frame {
	return c.recv
}

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

func (c *Canusb) SendString(str string) {
	waitChan := make(chan struct{})
	select {
	case c.send <- &rawCommand{
		data:      str,
		processed: waitChan,
	}:
	default:
		log.Fatal(fmt.Errorf("send channel full"))
	}
	<-waitChan
}

func (c *Canusb) Send(f *Frame) error {
	select {
	case c.send <- f:
		return nil
	default:
		return fmt.Errorf("send channel full")
	}
}

type Stats struct {
	RecvBytes uint64
	SentBytes uint64
	Errors    uint64
}

func (c *Canusb) Stats() Stats {
	return Stats{c.recvBytes, c.sentBytes, c.errors}
}

func portInfo(portName string) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		log.Fatal("No serial ports found!")
		return
	}
	for _, port := range ports {
		if port.Name == portName {
			log.Printf("Using port: %s\n", port.Name)
			if port.IsUSB {
				log.Printf("   USB ID     %s:%s\n", port.VID, port.PID)
				log.Printf("   USB serial %s\n", port.SerialNumber)
			}
		}
	}
}
