package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/serialcommand"
	"go.bug.st/serial"
	"golang.org/x/mod/semver"
)

func init() {
	if err := gocan.RegisterAdapter(&gocan.AdapterInfo{
		Name:               "txbridge wifi",
		Description:        "txbridge over wifi",
		RequiresSerialPort: false,
		Capabilities: gocan.AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewTxbridge("txbridge wifi"),
	}); err != nil {
		panic(err)
	}

	if err := gocan.RegisterAdapter(&gocan.AdapterInfo{
		Name:               "txbridge bluetooth",
		Description:        "txbridge over bluetooth",
		RequiresSerialPort: false,
		Capabilities: gocan.AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewTxbridge("txbridge bluetooth"),
	}); err != nil {
		panic(err)
	}
}

type Txbridge struct {
	BaseAdapter
	port io.ReadWriteCloser
}

func NewTxbridge(name string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		return &Txbridge{
			BaseAdapter: NewBaseAdapter(name, cfg),
		}, nil
	}
}

func (tx *Txbridge) Open(ctx context.Context) error {
	switch tx.name {
	case "txbridge wifi":
		d := net.Dialer{Timeout: 2 * time.Second}
		port, err := d.Dial("tcp", "192.168.4.1:1337")
		if err != nil {
			return err
		}
		//if t, ok := tx.port.(*net.TCPConn); ok {
		//	t.SetNoDelay(true)
		//}
		tx.port = port
	case "txbridge bluetooth":
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
		p.SetReadTimeout(10 * time.Millisecond)
		tx.port = p

		p.ResetOutputBuffer()
		p.ResetInputBuffer()
	default:
		return fmt.Errorf("unknown txbridge type: %s", tx.name)
	}

	tx.port.Write([]byte("ccc"))

	if tx.cfg.MinimumFirmwareVersion != "" {
		cmd := serialcommand.NewSerialCommand('v', []byte{0x10})
		buf, err := cmd.MarshalBinary()
		if err != nil {
			tx.port.Close()
			return err
		}

		if _, err := tx.port.Write(buf); err != nil {
			tx.port.Close()
			return err
		}

		cmd, err = readSerialCommand(tx.port, 5*time.Second)
		if err != nil {
			tx.port.Close()
			return err
		}

		if err := checkErr(cmd); err != nil {
			tx.port.Close()
			return fmt.Errorf("version check failed: %w", err)
		}

		if cmd.Command != 'v' {
			tx.port.Close()
			return fmt.Errorf("unexpected response: %X %X", cmd.Command, cmd.Data)
		}

		tx.cfg.OnMessage("txbridge firmware version: " + string(cmd.Data))

		if ver := semver.Compare("v"+string(cmd.Data), "v"+tx.cfg.MinimumFirmwareVersion); ver != 0 {
			tx.port.Close()
			return fmt.Errorf("txbridge firmware %s is required, please update the dongle", tx.cfg.MinimumFirmwareVersion)
		}
	}

	canRate := uint16(tx.cfg.CANRate)

	cmd := &serialcommand.SerialCommand{
		Command: 'o',
		Data:    []byte{uint8(canRate), uint8(canRate >> 8)},
	}
	openCmd, err := cmd.MarshalBinary()
	if err != nil {
		tx.port.Close()
		return err
	}
	tx.port.Write(openCmd)

	go tx.recvManager(ctx)
	go tx.sendManager(ctx)

	return nil
}

func (tx *Txbridge) SetFilter(filters []uint32) error {
	return nil
}

func (tx *Txbridge) Close() error {
	tx.BaseAdapter.Close()
	if tx.port != nil {
		if _, err := tx.port.Write([]byte("c")); err != nil {
			return fmt.Errorf("failed to write txbridge: %w", err)
		}
		if err := tx.port.Close(); err != nil {
			return fmt.Errorf("failed to close txbridge: %w", err)
		}
		tx.port = nil
	}
	return nil
}

func (tx *Txbridge) sendManager(_ context.Context) {
	if tx.cfg.Debug {
		log.Println("sendManager start")
		defer log.Println("sendManager exited")
	}
	for {
		select {
		case <-tx.closeChan:
			return
		case frame := <-tx.sendChan:
			if frame.Identifier == gocan.SystemMsg {
				_, err := tx.port.Write(frame.Data)
				if err != nil {
					tx.SetError(err)
				}
				continue
			}

			cmd := &serialcommand.SerialCommand{
				Command: 't',
				Data:    append([]byte{uint8(frame.Identifier >> 8), uint8(frame.Identifier), byte(frame.Length())}, frame.Data...),
			}
			buf, err := cmd.MarshalBinary()
			if err != nil {
				tx.SetError(err)
				continue
			}
			_, err = tx.port.Write(buf)
			if err != nil {
				tx.SetError(err)
				continue
			}
		}
	}
}

func (tx *Txbridge) recvManager(ctx context.Context) {
	if tx.cfg.Debug {
		log.Println("recvManager start")
		defer log.Println("recvManager exited")
	}
	var parsingCommand bool
	var command uint8
	var commandSize uint8
	var commandChecksum uint8

	cmdbuff := make([]byte, 256)
	var cmdbuffPtr uint8

	readbuf := make([]byte, 256)
	for {
		select {
		case <-tx.closeChan:
			log.Println("recvManager adapter closed")
			return
		case <-ctx.Done():
			log.Println("recvManager ctx done")
			return
		default:
		}

		n, err := tx.port.Read(readbuf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			tx.SetError(gocan.Unrecoverable(err))
			return
		}
		if n == 0 {
			continue
		}

		for _, b := range readbuf[:n] {
			if !parsingCommand {
				switch b {
				case 'e', 't', 'r', 'R', 'w', 'W':
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
				cmd := &serialcommand.SerialCommand{
					Command: command,
					Data:    append([]byte(nil), cmdbuff[:cmdbuffPtr]...),
				}
				if commandChecksum != b {
					tx.cfg.OnMessage(fmt.Sprintf("checksum error %q %02X != %02X", cmd.Command, commandChecksum, b))
					parsingCommand = false
					commandSize = 0
					commandChecksum = 0
					cmdbuffPtr = 0
					continue
				}
				var frame *gocan.CANFrame
				switch command {
				case 'T', 't':
					frame = gocan.NewFrame(
						uint32(cmd.Data[0])<<8|uint32(cmd.Data[1]),
						cmd.Data[2:],
						gocan.Incoming,
					)
				case 'e':
					switch cmd.Data[0] {
					case 0x31:
						tx.SetError(fmt.Errorf("read timeout"))
					case 0x06:
						tx.SetError(fmt.Errorf("invalid sequence"))
					default:
						tx.SetError(fmt.Errorf("xerror: %X", cmd.Data))
					}
					cmdbuffPtr = 0
					commandChecksum = 0
					commandSize = 0
					parsingCommand = false
					continue

				case 'R':
					frame = gocan.NewFrame(
						gocan.SystemMsgDataRequest,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				case 'r':
					frame = gocan.NewFrame(
						gocan.SystemMsgDataResponse,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				case 'w':
					// log.Printf("WBLReading: % X", cmd.Data[:commandSize])
					frame = gocan.NewFrame(
						gocan.SystemMsgWBLReading,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				case 'W':
					frame = gocan.NewFrame(
						gocan.SystemMsgWriteResponse,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				default:
					tx.cfg.OnMessage(fmt.Sprintf("unknown command: %q: %x", cmd.Command, cmd.Data))
					cmdbuffPtr = 0
					commandChecksum = 0
					commandSize = 0
					parsingCommand = false
					continue
				}
				select {
				case tx.recvChan <- frame:
				default:
					tx.SetError(gocan.ErrDroppedFrame)
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

func checkErr(cmd *serialcommand.SerialCommand) error {
	if cmd.Command == 'e' {
		return fmt.Errorf("error: %X %X", cmd.Command, cmd.Data)
	}
	return nil
}

// readSerialCommand reads a single command from the serial port with timeout
func readSerialCommand(port io.Reader, timeout time.Duration) (*serialcommand.SerialCommand, error) {
	deadline := time.Now().Add(timeout)

	var (
		parsingCommand  bool
		command         byte
		commandSize     byte
		commandChecksum byte
		cmdbuff         = make([]byte, 256)
		cmdbuffPtr      byte
	)

	readbuf := make([]byte, 16)

	for time.Now().Before(deadline) {
		n, err := port.Read(readbuf)
		if err != nil {
			return nil, fmt.Errorf("read error: %w", err)
		}
		if n == 0 {
			continue
		}

		for _, b := range readbuf[:n] {
			if !parsingCommand {
				parsingCommand = true
				command = b
				continue
			}

			if commandSize == 0 {
				commandSize = b
				continue
			}

			if cmdbuffPtr == commandSize {
				if commandChecksum != b {
					log.Printf("dara: %X", cmdbuff[:cmdbuffPtr])
					return nil, fmt.Errorf("checksum error: expected %02X, got %02X", b, commandChecksum)
				}

				return &serialcommand.SerialCommand{
					Command: command,
					Data:    append([]byte(nil), cmdbuff[:cmdbuffPtr]...),
				}, nil
			}

			if cmdbuffPtr < commandSize {
				cmdbuff[cmdbuffPtr] = b
				cmdbuffPtr++
				commandChecksum += b
			}
		}
	}

	return nil, fmt.Errorf("timeout after %v", timeout)
}

// writeSerialCommand writes a single command to the serial port
func writeSerialCommand(port io.Writer, command byte, data []byte) error {
	cmd := &serialcommand.SerialCommand{
		Command: command,
		Data:    data,
	}

	buf, err := cmd.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	_, err = port.Write(buf)
	if err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}
