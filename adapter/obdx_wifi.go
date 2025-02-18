package adapter

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/dvi"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               "OBDX Pro Wifi",
		Description:        "OBDX Pro Wifi",
		RequiresSerialPort: false,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: true,
		},
		New: NewOBDXProWifi,
	}); err != nil {
		panic(err)
	}
}

type OBDXProWifi struct {
	BaseAdapter
	conn      net.Conn
	filters   []*dvi.Command
	closeOnce sync.Once
}

func NewOBDXProWifi(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &OBDXProWifi{
		BaseAdapter: NewBaseAdapter("OBDX Pro Wifi", cfg),
	}, nil
}

func (a *OBDXProWifi) SetFilter(filters []uint32) error {
	for i, id := range filters {
		log.Printf("creating filter: %X", id)
		filterCMD := dvi.New(0x34, []byte{0x00, byte(i), dvi.FRAME_TYPE_11BIT, dvi.FILTER_TYPE_PASS, dvi.FILTER_STATUS_ON, byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id), 0x00, 0x00, 0x07, 0xFF, 0x00, 0x00, 0x00, 0x00})
		a.filters = append(a.filters, filterCMD)
	}
	return nil
}

func (a *OBDXProWifi) Connect(ctx context.Context) error {

	if err := a.SetFilter(a.cfg.CANFilter); err != nil {
		return err
	}

	d := net.Dialer{Timeout: 5 * time.Second}

	var err error
	a.conn, err = d.Dial("tcp", "192.168.4.1:23")
	if err != nil {
		return err
	}

	a.conn.Write([]byte("ATAR\r"))
	time.Sleep(20 * time.Millisecond)

	a.conn.Write([]byte("DXDP1\r"))
	time.Sleep(50 * time.Millisecond)

	slask := make([]byte, 1024)
	n, err := a.conn.Read(slask)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("Slask: %q", slask[:n])
	}

	var initCommands = []*dvi.Command{
		dvi.New(0x31, []byte{0x01, 0x02}), // set HS CAN
		dvi.New(0x34, []byte{0x15, 0x06}), // set CAN speed to 500kbit
		dvi.New(0x34, []byte{0x0F, 0x00}), // disable automatic formating for writing network frames
		dvi.New(0x34, []byte{0x0B, 0x00}), // disable Automatic Frame Processing for Received Network Messages
		dvi.New(0x34, []byte{0x0E, 0x00}), // disable padding
		//dvi.New(0x34, []byte{0x10, 0x00, 0xFA}), // delay between writing frames
		dvi.New(0x24, []byte{0x01, 0x00}), // disable network write responses status

	}

	for _, cmd := range initCommands {
		if _, err := a.conn.Write(cmd.Bytes()); err != nil {
			a.cfg.OnMessage(fmt.Sprintf("failed to send init command: %v", err))
		}
		time.Sleep(20 * time.Millisecond)
	}

	for _, f := range a.filters {
		if _, err := a.conn.Write(f.Bytes()); err != nil {
			a.cfg.OnMessage(fmt.Sprintf("failed to set filter: %v", err))
		}
		time.Sleep(20 * time.Millisecond)
	}

	// enable networking
	if _, err := a.conn.Write(dvi.New(0x31, []byte{0x02, 0x01}).Bytes()); err != nil {
		return fmt.Errorf("failed to enable networking: %v", err)
	}

	go a.recvManager()
	go a.sendManager()

	return nil
}

func (a *OBDXProWifi) sendManager() {
	for {
		select {
		case <-a.closeChan:
			log.Println("sendManager closed")
			return
		case frame := <-a.sendChan:
			id := frame.Identifier()
			if id >= gocan.SystemMsg {
				continue
			}
			sendCmd := dvi.New(dvi.CMD_SEND_TO_NETWORK_NORMAL, append([]byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}, frame.Data()...))
			if a.cfg.Debug {
				log.Println("dvi out:", sendCmd.String())
			}
			if _, err := a.conn.Write(sendCmd.Bytes()); err != nil {
				log.Printf("Failed to send DVI: %v", err)
			}

		}
	}
}

func (a *OBDXProWifi) recvManager() {
	parser := dvi.NewCommandParser(func(cmd *dvi.Command) {
		if a.cfg.Debug {
			log.Println("dvi in: ", cmd.String())
		}
		switch cmd.Command() {
		case 0x08:
			id := binary.BigEndian.Uint32(cmd.Data()[:4])
			frame := gocan.NewFrame(
				id,
				cmd.Data()[4:],
				gocan.Incoming,
			)
			select {
			case a.recvChan <- frame:
			default:
				log.Printf("dropped frame: %s", frame.String())
			}
			return
		}
	})
	buf := make([]byte, 16)
	for {
		select {
		case <-a.closeChan:
			log.Println("recvManager closed")
			return
		default:
			n, err := a.conn.Read(buf)
			if err != nil {
				//if errors.Is(err, os.ErrDeadlineExceeded) {
				//	continue
				//}
				log.Println("recv error:", err)
				return
			}
			if n == 0 {
				continue
			}
			parser.AddData(buf[:n])
		}
	}

}

func (a *OBDXProWifi) Close() error {
	a.BaseAdapter.Close()
	a.closeOnce.Do(func() {
		time.Sleep(80 * time.Millisecond)
		cmds := []*dvi.Command{
			dvi.New(0x31, []byte{0x02, 0x00}), // disable networking
			dvi.New(0x25, []byte{}),           // reset
		}
		for _, cmd := range cmds {
			if _, err := a.conn.Write(cmd.Bytes()); err != nil {
				log.Printf("Failed to send command: %v", err)
			}
			time.Sleep(20 * time.Millisecond)
		}

		a.conn.Close()
		a.cfg.OnMessage("Disconnected from OBDX Pro")
	})
	return nil
}
