package adapter

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/roffe/gocan"
	"golang.org/x/mod/semver"
)

const (
	TxbridgeWiFiAdapterName = "txbridge wifi"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               TxbridgeWiFiAdapterName,
		Description:        "txbridge over wifi",
		RequiresSerialPort: false,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewTxbridgeWiFi,
	}); err != nil {
		panic(err)
	}
}

type TxbridgeWiFi struct {
	BaseAdapter
	port net.Conn
}

func NewTxbridgeWiFi(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &TxbridgeWiFi{
		BaseAdapter: NewBaseAdapter(cfg),
	}, nil
}

func (tx *TxbridgeWiFi) SetFilter(filters []uint32) error {
	return nil
}

func (tx *TxbridgeWiFi) Name() string {
	return TxbridgeWiFiAdapterName
}

func (tx *TxbridgeWiFi) Init(ctx context.Context) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	port, err := d.Dial("tcp", "192.168.4.1:1337")
	if err != nil {
		return err
	}
	tx.port = port

	if t, ok := tx.port.(*net.TCPConn); ok {
		t.SetNoDelay(true)
	}

	tx.port.Write([]byte("ccc"))

	if tx.cfg.MinimumFirmwareVersion != "" {
		cmd := gocan.NewSerialCommand('v', []byte{0x10})
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

	cmd := &gocan.SerialCommand{
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

func (tx *TxbridgeWiFi) Close() error {
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

func (tx *TxbridgeWiFi) sendManager(_ context.Context) {
	for {
		select {
		case <-tx.close:
			return
		case frame := <-tx.send:
			if frame.Identifier() == SystemMsg {
				_, err := tx.port.Write(frame.Data())
				if err != nil {
					tx.err <- err
				}
				continue
			}

			cmd := &gocan.SerialCommand{
				Command: 't',
				Data:    append([]byte{uint8(frame.Identifier() >> 8), uint8(frame.Identifier()), byte(frame.Length())}, frame.Data()...),
			}
			buf, err := cmd.MarshalBinary()
			if err != nil {
				tx.err <- err
				continue
			}
			_, err = tx.port.Write(buf)
			if err != nil {
				tx.err <- err
				continue
			}
		}
	}
}

func (tx *TxbridgeWiFi) recvManager(ctx context.Context) {
	log.Println("recvManager start")
	defer log.Println("recvManager exited")

	var parsingCommand bool
	var command uint8
	var commandSize uint8
	var commandChecksum uint8

	cmdbuff := make([]byte, 256)
	var cmdbuffPtr uint8

	readbuf := make([]byte, 256)
	for {
		select {
		case <-tx.close:
			log.Println("recvManager adapter closed")
			return
		case <-ctx.Done():
			log.Println("recvManager ctx done")
			return
		default:
		}

		n, err := tx.port.Read(readbuf)
		if err != nil {
			log.Println("recvManager read error", err)
			tx.err <- err
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
