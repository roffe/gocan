package gocan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/roffe/gocan/pkg/serialcommand"
	"go.bug.st/serial"
	"golang.org/x/mod/semver"
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "txbridge wifi",
		Description:        "txbridge over wifi",
		RequiresSerialPort: true,
		SerialPortOptional: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewTxbridge("txbridge wifi"),
	}); err != nil {
		panic(err)
	}

	/*
		if err := RegisterAdapter(&AdapterInfo{
			Name:               "txbridge bluetooth",
			Description:        "txbridge over bluetooth",
			RequiresSerialPort: true,
			Capabilities: AdapterCapabilities{
				HSCAN: true,
				KLine: false,
				SWCAN: false,
			},
			New: NewTxbridge("txbridge bluetooth"),
		}); err != nil {
			panic(err)
		}
	*/
}

type Txbridge struct {
	*BaseAdapter
	port io.ReadWriteCloser
}

func NewTxbridge(name string) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		tx := &Txbridge{
			BaseAdapter: NewBaseAdapter(name, cfg),
		}
		tx.syncCapable = true // writeFrame confirms the frame is written to the port
		return tx, nil
	}
}

func (tx *Txbridge) Open(ctx context.Context) error {
	switch tx.name {
	case "txbridge wifi":
		address := "192.168.4.1:1337"
		if strings.HasPrefix(tx.cfg.Port, "tcp://") {
			address = tx.cfg.Port[len("tcp://"):]
		}

		if !strings.HasSuffix(address, ":1337") {
			address += ":1337" // Ensure the port is always set
		}
		d := net.Dialer{Timeout: 2 * time.Second}
		port, err := d.Dial("tcp", address)
		if err != nil {
			return err
		}
		if t, ok := port.(*net.TCPConn); ok {
			t.SetNoDelay(true) // low latency for small log messages
		}
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

	var minVersion string
	if val, found := tx.cfg.AdditionalConfig["minversion"]; found && val != "" {
		minVersion = val
	}

	if minVersion != "" {
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
			return fmt.Errorf("unexpected version response: %X %X", cmd.Command, cmd.Data)
		}

		tx.Info("txbridge firmware version: " + string(cmd.Data))

		if ver := semver.Compare("v"+string(cmd.Data), "v"+minVersion); ver < 0 {
			tx.port.Close()
			return fmt.Errorf("txbridge firmware %s or newer is required (dongle has %s), please update the dongle", minVersion, string(cmd.Data))
		}
	}

	//if err := tx.SetFilter(tx.cfg.CANFilter); err != nil {
	//	tx.port.Close()
	//	return err
	//}

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

// SetFilter installs the adapter's dynamic whitelist: only frames whose ID is
// listed are forwarded over the link (the KWP IDs the firmware handles itself are
// always passed). Can be called at any time. An empty list is a no-op so the
// firmware keeps its boot default whitelist rather than going silent.
func (tx *Txbridge) SetFilter(filters []uint32) error {
	log.Printf("set filters: %03X", filters)
	if len(filters) == 0 {
		return nil
	}
	data := make([]byte, 0, len(filters)*2)
	for _, id := range filters {
		data = append(data, byte(id), byte(id>>8)) // 11-bit IDs, little-endian
	}
	cmd := &serialcommand.SerialCommand{Command: 'f', Data: data}
	buf, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}
	if tx.port == nil {
		return fmt.Errorf("txbridge port not open")
	}
	// Route through sendManager (the sole port writer) so this can't interleave
	// with an in-flight frame and tear the wire framing.
	select {
	case tx.sendChan <- NewFrame(SystemMsg, buf, Outgoing):
		return nil
	case <-time.After(1 * time.Second):
		return fmt.Errorf("SetFilter: send queue full")
	}
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
	defer log.Println("sendManager exited")
	if tx.cfg.Debug {
		log.Println("sendManager start")
	}
	for {
		select {
		case <-tx.closeChan:
			return
		case frame := <-tx.sendChan:
			if frame.Identifier == SystemMsg {
				_, err := tx.port.Write(frame.Data)
				if err != nil {
					tx.Error(err)
				}
				continue
			}
			tx.writeFrame(frame)
		}
	}
}

// writeFrame serializes and writes a single CAN frame to the port. The deferred
// markSent releases a SendSync waiter on every exit path (success or error).
func (tx *Txbridge) writeFrame(frame *CANFrame) {
	defer frame.markSent()
	cmd := &serialcommand.SerialCommand{
		Command: 't',
		Data:    append([]byte{uint8(frame.Identifier >> 8), uint8(frame.Identifier), byte(frame.DLC())}, frame.Data...),
	}
	buf, err := cmd.MarshalBinary()
	if err != nil {
		tx.Error(err)
		return
	}
	if _, err := tx.port.Write(buf); err != nil {
		tx.Error(err)
	}
}

func (tx *Txbridge) recvManager(ctx context.Context) {
	defer log.Println("recvManager exited")
	if tx.cfg.Debug {
		log.Println("recvManager start")
	}
	var parsingCommand bool
	var haveLength bool // length byte read? distinguishes "no len yet" from len==0
	var command uint8
	var commandSize uint8
	var commandChecksum uint8

	cmdbuff := make([]byte, 256)
	var cmdbuffPtr uint8

	readbuf := make([]byte, 4096)
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
			tx.Fatal(err)
			return
		}
		if n == 0 {
			continue
		}

		for _, b := range readbuf[:n] {
			if !parsingCommand {
				switch b {
				case 'e', 't', 'r', 'R', 'w', 'W', 'G':
					parsingCommand = true
					haveLength = false
					command = b
					commandSize = 0
					commandChecksum = 0
					cmdbuffPtr = 0
					continue
				default:
					continue
				}
			}

			if !haveLength { // length byte; zero is a legitimate size, so don't sentinel on it
				commandSize = b
				haveLength = true
				continue
			}

			if cmdbuffPtr == commandSize {
				//cmd := &serialcommand.SerialCommand{
				//	Command: command,
				//	Data:    make([]byte, commandSize),
				//}
				data := make([]byte, commandSize)
				copy(data, cmdbuff[:cmdbuffPtr])
				if commandChecksum != b {
					tx.sendEvent(EventTypeError, fmt.Sprintf("checksum error: expected %02X, got %02X", commandChecksum, b))
					// tx.cfg.OnMessage(fmt.Sprintf("checksum error %q %02X != %02X", command, commandChecksum, b))
					parsingCommand = false
					commandSize = 0
					commandChecksum = 0
					cmdbuffPtr = 0
					continue
				}

				switch command {
				case 'T', 't':
					// Guard: the 1-byte sum checksum is weak enough that a corrupted
					// frame can pass with a short size; don't panic on data[0]/data[1].
					if len(data) < 2 {
						tx.sendEvent(EventTypeError, fmt.Sprintf("short %q frame: %X", command, data))
						break
					}
					tx.recvFrame(&CANFrame{
						Identifier: uint32(data[0])<<8 | uint32(data[1]),
						Data:       data[2:],
						FrameType:  Incoming,
					})
				case 'e':
					if len(data) < 2 {
						tx.sendEvent(EventTypeError, fmt.Sprintf("short %q frame: %X", command, data))
						cmdbuffPtr = 0
						commandChecksum = 0
						commandSize = 0
						parsingCommand = false
						continue
					}
					switch data[1] {
					case 0x31:
						tx.sendEvent(EventTypeError, "read timeout")
					case 0x32:
						tx.sendEvent(EventTypeError, "invalid sequence")
					default:
						tx.sendEvent(EventTypeError, fmt.Sprintf("Unknown: %X", data))
					}
					cmdbuffPtr = 0
					commandChecksum = 0
					commandSize = 0
					parsingCommand = false
					continue

				case 'R':
					tx.recvFrame(&CANFrame{
						Identifier: SystemMsgDataRequest,
						Data:       data,
						FrameType:  Incoming,
					})
				case 'r':
					tx.recvFrame(&CANFrame{
						Identifier: SystemMsgDataResponse,
						Data:       data,
						FrameType:  Incoming,
					})
				case 'w':
					// log.Printf("WBLReading: % X", cmd.Data[:commandSize])
					tx.recvFrame(&CANFrame{
						Identifier: SystemMsgWBLReading,
						Data:       data,
						FrameType:  Incoming,
					})
				case 'W':
					tx.recvFrame(&CANFrame{
						Identifier: SystemMsgWriteResponse,
						Data:       data,
						FrameType:  Incoming,
					})
				default:
					tx.Error(fmt.Errorf("unknown command: %q: %x", command, data))
					cmdbuffPtr = 0
					commandChecksum = 0
					commandSize = 0
					parsingCommand = false
					continue
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

func (tx *Txbridge) recvFrame(frame *CANFrame) {
	select {
	case tx.recvChan <- frame:
	default:
		tx.Error(ErrDroppedFrame)
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
		haveLength      bool // length byte read? distinguishes "no len yet" from len==0
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

			if !haveLength { // length byte; zero is a legitimate size, so don't sentinel on it
				commandSize = b
				haveLength = true
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

/*
// WriteSerialCommand writes a single command to the serial port
func WriteSerialCommand(port io.Writer, command byte, data []byte) error {
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
*/
