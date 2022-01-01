package canusb

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
	"go.uber.org/ratelimit"
)

const (
	SaabIBUSRate = "scb9a"
	SaabPBUSRate = "S6"
)

type Canusb struct {
	c    context.CancelFunc
	port serial.Port
	recv chan []byte
	send chan []byte
	sync.Mutex
	recvBytes, sentBytes uint64
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

type Frame struct {
	Identifier string
	Data       interface{}
}

func New(cfg *Config) (*Canusb, error) {
	mode := &serial.Mode{
		BaudRate: 921600,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(cfg.Com.Port, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open com port %q : %v", cfg.Com.Port, err)
	}

	recv := make(chan []byte, 1000)
	send := make(chan []byte, 1000)

	c := &Canusb{
		port: p,
		recv: recv,
		send: send,
	}

	go c.run(cfg.Can.Rate)
	return c, nil
}

func (c *Canusb) run(canrate string) {
	ctx, cancel := context.WithCancel(context.Background())
	c.c = cancel
	go func() {
		defer cancel()
		defer close(c.recv)
		out := bytes.NewBuffer([]byte{})
		buff := make([]byte, 1)
		for {
			if ctx.Err() != nil {
				break
			}
			n, err := c.port.Read(buff)
			if err != nil {
				log.Println(err)
				break
			}
			if n == 0 {
				log.Println("stopped consuming due to 0 byte read")
				break
			}
			atomic.AddUint64(&c.recvBytes, uint64(n))
			out.Write(buff)
			if buff[n-1] == 0x0D {
				if out.Len() == 1 {
					out.Reset()
					continue
				}
				c.recv <- B(strings.ReplaceAll(out.String(), "\r", ""))
				out.Reset()
			}
		}
	}()
	go func() {
		rl := ratelimit.New(50)
		defer cancel()
		for o := range c.send {
			if ctx.Err() != nil {
				break
			}
			rl.Take()
			n, err := c.writeCom(o)
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
			c.Send(B("F\r")) // Read Status Flags
		}
	}()

	c.Send(B("\r\r\r")) // Empty buffer

	c.Send(B("V\r"))          // Get Version number of both CANUSB hardware and software
	c.Send(B("N\r"))          // Get Serial number of the CANUSB
	c.Send(B(canrate + "\r")) // Setup CAN bit-rates
	c.Send(B("O\r"))          // Open the CAN channel

}

func (c *Canusb) Stop() error {
	c.c()
	c.port.Write(B("C\r"))
	time.Sleep(50 * time.Millisecond)
	return c.port.Close()
}

func (c *Canusb) Chan() <-chan []byte {
	return c.recv
}

func (c *Canusb) Send(b []byte) {
	c.send <- b
}

type Stats struct {
	RecvBytes, SentBytes uint64
}

func (c *Canusb) Stats() Stats {
	return Stats{c.recvBytes, c.sentBytes}
}

func (c *Canusb) writeCom(data []byte) (int, error) {
	n, err := c.port.Write(data)
	if err != nil {
		return 0, fmt.Errorf("error writing to port: %v", err)
	}
	return n, nil
}

func portInfo() {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		fmt.Println("No serial ports found!")
		return
	}
	for _, port := range ports {
		fmt.Printf("Found port: %s\n", port.Name)
		if port.IsUSB {
			fmt.Printf("   USB ID     %s:%s\n", port.VID, port.PID)
			fmt.Printf("   USB serial %s\n", port.SerialNumber)
		}
	}
}
