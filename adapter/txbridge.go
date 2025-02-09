package adapter

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"go.bug.st/serial"
	"golang.org/x/mod/semver"
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
	BaseAdapter
	port      serial.Port
	closeOnce sync.Once
}

func NewTxbridge(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	tx := &Txbridge{
		BaseAdapter: NewBaseAdapter(cfg),
	}
	return tx, nil
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
	p.SetReadTimeout(10 * time.Millisecond)
	tx.port = p

	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	tx.port.Write([]byte("ccc"))

	if tx.cfg.MinimumFirmwareVersion != "" {
		cmd := gocan.NewSerialCommand('v', []byte{0x10})
		buf, err := cmd.MarshalBinary()
		if err != nil {
			p.Close()
			return err
		}

		if _, err := p.Write(buf); err != nil {
			p.Close()
			return err
		}

		cmd, err = readSerialCommand(p, 5*time.Second)
		if err != nil {
			p.Close()
			return err
		}

		if err := checkErr(cmd); err != nil {
			p.Close()
			return fmt.Errorf("version check failed: %w", err)
		}

		if cmd.Command != 'v' {
			p.Close()
			return fmt.Errorf("unexpected response: %X %X", cmd.Command, cmd.Data)
		}

		tx.cfg.OnMessage("txbridge firmware version: " + string(cmd.Data))

		if ver := semver.Compare("v"+string(cmd.Data), "v"+tx.cfg.MinimumFirmwareVersion); ver != 0 {
			p.Close()
			return fmt.Errorf("txbridge firmware %s is required, please update the dongle", tx.cfg.MinimumFirmwareVersion)
		}
	}

	canRate := uint16(tx.cfg.CANRate)

	cmd := &gocan.SerialCommand{
		Command: 'o',
		Data:    []byte{uint8(canRate), uint8(canRate >> 8)},
	}
	openCmd, err := cmd.MarshalBinary()
	if err != nil {
		p.Close()
		return err
	}

	tx.port.Write(openCmd)

	go tx.recvManager(ctx)
	go tx.sendManager(ctx)

	return nil
}

func (tx *Txbridge) Close() error {
	tx.BaseAdapter.Close()
	tx.closeOnce.Do(func() {
		time.Sleep(200 * time.Millisecond)
		if tx.port != nil {
			tx.port.Write([]byte("c"))
			tx.port.Drain()
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

	//defer tx.Close()

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
				case 'R':
					frame = gocan.NewFrame(
						SystemMsgDataRequest,
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
					// log.Printf("WBLReading: % X", cmd.Data[:commandSize])
					frame = gocan.NewFrame(
						SystemMsgWBLReading,
						cmd.Data[:commandSize],
						gocan.Incoming,
					)
				case 'W':
					frame = gocan.NewFrame(
						SystemMsgWriteResponse,
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

func checkErr(cmd *gocan.SerialCommand) error {
	if cmd.Command == 'e' {
		return fmt.Errorf("error: %X %X", cmd.Command, cmd.Data)
	}
	return nil
}

// readSerialCommand reads a single command from the serial port with timeout
func readSerialCommand(port io.Reader, timeout time.Duration) (*gocan.SerialCommand, error) {
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

				data := make([]byte, cmdbuffPtr)
				copy(data, cmdbuff[:cmdbuffPtr])

				return &gocan.SerialCommand{
					Command: command,
					Data:    data,
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
func writeSerialCommand(port serial.Port, command byte, data []byte) error {
	cmd := &gocan.SerialCommand{
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
