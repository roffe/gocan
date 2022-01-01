package canusb

import (
	"bytes"
	"context"
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
	"go.uber.org/ratelimit"
)

const (
	//CR = "\r"
	CR           = 0x0d
	SaabIBUSRate = 47.619
	SaabPBUSRate = 500
)

type Canusb struct {
	//debug                bool
	canrate    string
	c          context.CancelFunc
	port       serial.Port
	recv, send chan *Frame
	//send                 chan []byte
	recvBytes, sentBytes uint64
	errors               uint64
	sync.Mutex
}

type Config struct {
	Com ComConfig
	Can CanConfig
}

type ComConfig struct {
	Port     string
	BaudRate int
}

type CanConfig struct {
	Rate string // S0..
}

type B []byte

func New(opts ...Opts) (*Canusb, error) {
	//c := new(Canusb)
	c := &Canusb{
		recv: make(chan *Frame, 1000),
		send: make(chan *Frame, 100),
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	go c.run()
	return c, nil
}

func (c *Canusb) run() {
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
			if readBuffer[n-1] == 0x0D {
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
					c.recv <- f
				case 'z':
					// previous command was ok
				case 0x07:
					atomic.AddUint64(&c.errors, 1)
					log.Println("received error response")
				default:
					log.Printf("COM>> %q || %X\n", string(b), b)
				}

				frame.Reset()
			}
		}
	}()
	go func() {
		rl := ratelimit.New(50)
		defer cancel()
		for f := range c.send {
			if ctx.Err() != nil {
				break
			}
			rl.Take()
			out := fmt.Sprintf("t%s%d%x", f.Identifier, f.Len, f.Data)
			LogOut(f)
			n, err := c.writeCom(B(out))
			if err != nil {
				log.Fatal(err)
			}
			c.sentBytes += uint64(n)
			atomic.AddUint64(&c.sentBytes, uint64(n))
		}
	}()

	go func() {
		for {
			time.Sleep(1 * time.Second)
			c.writeCom(B("F\r")) // Read Status Flags
		}
	}()

	c.initCom()
}

func LogOut(f *Frame) {
	var out strings.Builder
	out.WriteString(strings.ToUpper(f.Identifier) + "h [")
	for i, b := range f.Data {
		out.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			out.WriteString(" ")

		}
	}
	out.WriteString("]")
	log.Println(out.String())
	//log.Printf("%sh %d %X\n", strings.ToUpper(f.Identifier), f.Len, f.Data)
}

func (c *Canusb) initCom() {
	var init = []B{
		{CR, CR, CR},        // Empty buffer
		{'V', CR},           // Get Version number of both CANUSB hardware and software
		{'N', CR},           // Get Serial number of the CANUSB
		B(c.canrate + "\r"), // Setup CAN bit-rates
		//B("M" + "\r"),
		{'O', CR}, // Open the CAN channel
	}

	for _, i := range init {
		if _, err := c.writeCom(i); err != nil {
			log.Fatal(err)
		}
	}
}

func parseFrame(buff *bytes.Buffer) *Frame {
	p := strings.ReplaceAll(buff.String(), "\r", "")

	addr := string(p[1:4])

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
		Len:        len,
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

func (c *Canusb) writeCom(data []byte) (int, error) {
	n, err := c.port.Write(data)
	if err != nil {
		return 0, fmt.Errorf("error writing to port: %v", err)
	}
	return n, nil
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
