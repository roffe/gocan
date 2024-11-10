package adapter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"go.bug.st/serial"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               "txbridge",
		Description:        "txbridge",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewTxbridge,
	}); err != nil {
		panic(err)
	}
}

type Txbridge struct {
	cfg        *gocan.AdapterConfig
	port       serial.Port
	send, recv chan gocan.CANFrame
	close      chan struct{}
	closeOnce  sync.Once
}

func NewTxbridge(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Txbridge{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 40),
		close: make(chan struct{}),
	}, nil
}

func (tx *Txbridge) SetFilter(filters []uint32) error {
	return nil
}

func (tx *Txbridge) Name() string {
	return "txbridge"
}

func (tx *Txbridge) Init(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: tx.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(tx.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", tx.cfg.Port, err)
	}
	p.SetReadTimeout(20 * time.Millisecond)
	tx.port = p

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	tx.port.Write([]byte("ccc"))
	tx.port.Write([]byte("o"))

	go tx.recvManager(ctx)
	go tx.sendManager(ctx)

	return nil
}

func (tx *Txbridge) Recv() <-chan gocan.CANFrame {
	return tx.recv
}

func (tx *Txbridge) Send() chan<- gocan.CANFrame {
	return tx.send
}

func (tx *Txbridge) Close() error {
	tx.closeOnce.Do(func() {
		tx.port.Write([]byte("c"))
		time.Sleep(200 * time.Millisecond)
		close(tx.close)
		if tx.port != nil {
			tx.port.Close()
		}
	})
	return nil
}

func (tx *Txbridge) sendManager(_ context.Context) {
	defer tx.Close()
	for {
		select {
		case <-tx.close:
			return
		case frame := <-tx.send:
			if frame.Identifier() == SystemMsg {
				_, err := tx.port.Write(frame.Data())
				if err != nil {
					tx.cfg.OnError(err)
				}
				continue
			}

			cmd := &gocan.SerialCommand{
				Command: 't',
				Data:    append([]byte{uint8(frame.Identifier() >> 8), uint8(frame.Identifier()), byte(frame.Length())}, frame.Data()...),
			}
			buf, err := cmd.MarshalBinary()
			if err != nil {
				tx.cfg.OnError(err)
				continue
			}
			_, err = tx.port.Write(buf)
			if err != nil {
				tx.cfg.OnError(err)
				continue
			}
		}
	}
}

func (tx *Txbridge) recvManager(ctx context.Context) {
	var parsingCommand bool
	var command uint8
	var commandSize uint8
	var commandChecksum uint8

	cmdbuff := make([]byte, 256)
	var cmdbuffPtr uint8

	defer tx.Close()

	readbuf := make([]byte, 16)
	for {
		select {
		case <-tx.close:
			return
		case <-ctx.Done():
			return
		default:
		}

		n, err := tx.port.Read(readbuf)
		if err != nil {
			tx.cfg.OnError(err)
			return
		}
		if n == 0 {
			continue
		}

		for _, b := range readbuf[:n] {
			if !parsingCommand {
				switch b {
				case 'e', 't', 'r', 'w':
					parsingCommand = true
					command = b
					commandSize = 0
					commandChecksum = 0
					cmdbuffPtr = 0
					continue
				default:
					continue
				}
			}

			if commandSize == 0 {
				commandSize = b
				continue
			}

			if cmdbuffPtr == commandSize {
				db := make([]byte, len(cmdbuff[:cmdbuffPtr]))
				copy(db, cmdbuff[:cmdbuffPtr])
				cmd := &gocan.SerialCommand{
					Command: command,
					Data:    db,
				}
				if commandChecksum != b {
					tx.cfg.OnError(fmt.Errorf("checksum error %q %02X != %02X", cmd.Command, commandChecksum, b))
					parsingCommand = false
					commandSize = 0
					commandChecksum = 0
					cmdbuffPtr = 0
					continue
				}
				var frame gocan.CANFrame

				switch command {
				case 'T', 't':
					frame = gocan.NewFrame(
						uint32(cmd.Data[0])<<8|uint32(cmd.Data[1]),
						cmd.Data[2:],
						gocan.Incoming,
					)
				case 'e':
					frame = gocan.NewFrame(
						SystemMsgError,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				case 'r':
					frame = gocan.NewFrame(
						SystemMsgDataResponse,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				case 'w':
					frame = gocan.NewFrame(
						SystemMsgWBLReading,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				}

				select {
				case tx.recv <- frame:
				default:
					tx.cfg.OnError(ErrDroppedFrame)
				}

				cmdbuffPtr = 0
				commandChecksum = 0
				commandSize = 0
				parsingCommand = false
				continue
			}

			if cmdbuffPtr < commandSize {
				cmdbuff[cmdbuffPtr] = b
				cmdbuffPtr++
				commandChecksum += b
				continue
			}
		}
	}
}
