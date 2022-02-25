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

	"github.com/roffe/gocan/pkg/frame"
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

//var canIDS = []uint32{0x005, 0x006, 0x00c, 0x00}

type SX struct {
	port         serial.Port
	portName     string
	portRate     int
	canrate      string
	protocol     string
	send, recv   chan frame.CANFrame
	close        chan struct{}
	filter, mask string
}

func NewSX() *SX {
	return &SX{
		send:  make(chan frame.CANFrame, 100),
		recv:  make(chan frame.CANFrame, 100),
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
	var initCmds = []string{
		"ATE0",      // turn off echo
		"ATS0",      // turn of spaces
		cu.protocol, // Set canbus protocol
		"ATH1",      // Headers on
		"ATAT2",     // Set adaptive timing mode, Adaptive timing on, aggressive mode. This option may increase throughput on slower connections, at the expense of slightly increasing the risk of missing frames.
		"ATCAF0",    // Automatic formatting of
		cu.canrate,  // Set CANrate
		"ATAL",      // Allow long messages
		"ATCFC0",    //Turn automatic CAN flow control off
		//"ATAR",      // Automatically set the receive address.
		//"ATCSM0",    //Turn CAN silent monitoring off
		cu.filter, // code
		cu.mask,   // mask
	}
	p.Write([]byte("ATI\r"))
	p.ResetInputBuffer()
	p.Write([]byte("ATZ\r"))
	readbuff := make([]byte, 1)
	for read := 0; read < 8; {
		n, err := p.Read(readbuff)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		read++

	}

	p.SetReadTimeout(5 * time.Millisecond)
	for _, c := range initCmds {
		if c == "" {
			continue
		}
		//log.Println(c)
		out := []byte(c + "\r")
		_, err := p.Write(out)
		if err != nil {
			log.Println(err)
		}
		time.Sleep(150 * time.Millisecond)
		p.ResetInputBuffer()
	}

	time.Sleep(150 * time.Millisecond)
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
		cu.protocol = "STP33"
	case 615.384:
		cu.protocol = "STP33"
		cu.canrate = "STCTR8101FC"
	default:
		return fmt.Errorf("unhandled canbus rate: %f", rate)
	}
	return nil
}

func (cu *SX) SetCANfilter(ids ...uint32) {
	var filt uint32 = 0xFFF
	var mask uint32 = 0x000
	for _, id := range ids {
		filt &= id
		mask |= id
	}
	mask = (^mask & 0x7FF) | filt
	cu.filter = fmt.Sprintf("ATCF%03X", filt)
	cu.mask = fmt.Sprintf("ATCM%03X", mask)
}

func (cu *SX) Chan() <-chan frame.CANFrame {
	return cu.recv
}

func (cu *SX) Send(frame frame.CANFrame) error {
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
	for {
		select {
		case v := <-cu.send:
			sendMutex <- token{}
			var out string
			switch v.(type) {
			case *frame.RawCommand:
				out = fmt.Sprintf("%s\r", v.String())
			default:
				if v.Type() == frame.Outgoing {
					out = fmt.Sprintf("STPX h:%03X,d:%X,r:0\r", v.Identifier(), v.Data())
					break
				}
				if v.GetTimeout() != 0 {
					out = fmt.Sprintf("STPX h:%03X,d:%X,t:%d,r:1\r", v.Identifier(), v.Data(), v.GetTimeout().Milliseconds())
				} else {
					out = fmt.Sprintf("STPX h:%03X,d:%X,r:1\r", v.Identifier(), v.Data())
				}
			}
			//log.Println(out)
			_, err := cu.port.Write([]byte(out))
			if err != nil {
				log.Printf("failed to write to com port: %q, %v\n", out, err)
			}
			// log.Println(">>", v.String()) // debug
		case <-ctx.Done():
			return
		case <-cu.close:
			return
		}
	}
	//if err := cu.Close(); err != nil {
	//	log.Println("port close error: ", err)
	//}
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
			log.Printf("failed to read com port: %v", err)
			return
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
				case "CAN ERROR":
					log.Println("CAN ERROR")
					buff.Reset()
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

func (*SX) decodeFrame(buff []byte) (frame.CANFrame, error) {
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

	return frame.New(
		binary.BigEndian.Uint32(idBytes),
		data,
		frame.Incoming,
	), nil

	//return &frame.Frame{
	//	Time:       received,
	//	Identifier: binary.BigEndian.Uint32(idBytes),
	//	Len:        length,
	//	Data:       data,
	//}, nil
}
