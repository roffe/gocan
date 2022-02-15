package obdlink

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/roffe/gocan/pkg/model"
	"go.bug.st/serial"
)

/*
ATZ
STP33
ATCAF0
ATH1

ATSH sätt header, CAN ID som saker ska skickas till

ATCF200 // acceptance Code
ATCM781 // acceptance Mask
STPX h:220, d:3F81001102400000
STPX h:240, d:40A1021A99000000

*/

type SX struct {
	port       serial.Port
	portName   string
	portRate   int
	canRate    string
	send, recv chan model.CANFrame
	close      chan struct{}
}

func NewSX() *SX {
	return &SX{
		send:  make(chan model.CANFrame, 100),
		recv:  make(chan model.CANFrame, 100),
		close: make(chan struct{}, 10),
	}
}

func (cu *SX) Init(ctx context.Context) error {
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

	p.Write([]byte("ATZ\r"))
	time.Sleep(1100 * time.Millisecond)
	p.ResetInputBuffer()
	var cmds = []string{
		"ATE0", // turn off echo
		"ATS0", // turn of spaces
		"STIX", // show extended firmware info
		"ATR0",
		cu.canRate, // Set canbus protocol 33
		"ATCAF0",   // Automatic formatting of
		"ATH1",     // Headers on
		"ATAT0",    // Set adaptive timing mode, Adaptive timing on, aggressive mode. This option may increase throughput on slower connections, at the expense of slightly increasing the risk of missing frames.
		"ATCF200",  // code
		"ATCM781",  // mask
	}
	p.SetReadTimeout(5 * time.Millisecond)
	for _, c := range cmds {
		_, err := p.Write([]byte(c + "\r"))
		if err != nil {
			log.Println(err)
		}
		time.Sleep(100 * time.Millisecond)
		p.ResetInputBuffer()
	}

	time.Sleep(10 * time.Millisecond)
	p.ResetInputBuffer()

	go cu.recvManager(ctx)
	go cu.sendManager(ctx)

	return nil
}

func (cu *SX) SetPort(port string) error {
	cu.portName = port
	return nil
}

func (cu *SX) SetPortRate(rate int) error {
	cu.portRate = rate
	return nil
}

func (cu *SX) SetCANrate(rate float64) error {
	switch rate {
	case 500:
		cu.canRate = "STP33"
	default:
		log.Fatalf("unhandled canbus rate: %f", rate)
	}
	return nil
}

func (cu *SX) SetCANfilter(ids ...uint32) {
	log.Println(ids)
}

func (cu *SX) Chan() <-chan model.CANFrame {
	return cu.recv
}

func (cu *SX) Send(frame model.CANFrame) error {
	select {
	case cu.send <- frame:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("failed to send frame")
	}
}

func (cu *SX) Close() error {
	cu.close <- token{}
	return cu.port.Close()
}

type token struct{}

var sendMutex = make(chan token, 1)

func (cu *SX) sendManager(ctx context.Context) {
outer:
	for {
		select {
		case v := <-cu.send:
			sendMutex <- token{}
			var out string
			switch v.(type) {
			case *model.RawCommand:
				out = fmt.Sprintf("%s\r", v.String())
			default:
				if v.Type() == model.Outgoing {
					out = fmt.Sprintf("STPX h:%X,d:%X,r:0\r", v.Identifier(), v.Data())
					break
				}
				out = fmt.Sprintf("STPX h:%X,d:%X,r:1\r", v.Identifier(), v.Data())
			}
			_, err := cu.port.Write([]byte(out))
			if err != nil {
				log.Printf("failed to write to com port: %q, %v\n", out, err)
			}
		case <-ctx.Done():
			break outer
		case <-cu.close:
			break outer
		}
	}
	if err := cu.Close(); err != nil {
		log.Println("port close error: ", err)
	}
}

func (cu *SX) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 19)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			if strings.Contains(err.Error(), "Port has been closed") && ctx.Err() != nil {
				break
			}
			log.Fatalf("failed to read com port: %v", err)
		}
		if n == 0 {
			continue
		}

		for _, b := range readBuffer[:n] {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if b == '>' {
				select {
				case <-sendMutex:
				default:
				}
				continue
			}

			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				switch buff.String() {
				case "STOPPED":
					buff.Reset()
				case "?":
					log.Println("UNKNOWN COMMAND")
					buff.Reset()
				case "NO DATA":
					buff.Reset()
				case "OK":
					buff.Reset()
				default:
					f, err := cu.decodeFrame(buff.Bytes())
					if err != nil {
						log.Printf("%v: %q\n", err, buff.String())
						continue
					}
					select {
					case cu.recv <- f:
					default:
						log.Println("dropped frame")
					}
					buff.Reset()
				}
				continue
			}
			buff.WriteByte(b)
		}
	}
}

func (*SX) decodeFrame(buff []byte) (model.CANFrame, error) {
	//received := time.Now()
	idBytes, err := hex.DecodeString(fmt.Sprintf("%08s", buff[0:3]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}
	data, err := hex.DecodeString(string(buff[3:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame body: %v", err)
	}

	//length := uint8(len(data))

	return model.NewFrame(
		binary.BigEndian.Uint32(idBytes),
		data,
		model.Incoming,
	), nil

	//return &model.Frame{
	//	Time:       received,
	//	Identifier: binary.BigEndian.Uint32(idBytes),
	//	Len:        length,
	//	Data:       data,
	//}, nil
}
