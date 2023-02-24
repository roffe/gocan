package legion

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/gocan/pkg/model"
)

var EcuByte_MCP byte = 5
var EcuByte_T8 byte = 6

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
	legionRunning  bool
	gm             *gmlan.Client
	canID          uint32
	recvID         []uint32
}

func New(c *gocan.Client, canID uint32, recvID ...uint32) *Client {
	return &Client{
		c:              c,
		defaultTimeout: 150 * time.Millisecond,
		gm:             gmlan.New(c, canID, recvID...),
		canID:          canID,
		recvID:         recvID,
	}
}

func (t *Client) IsRunning() bool {
	return t.legionRunning
}

func (t *Client) StartBootloader(ctx context.Context, startAddress uint32) error {
	return t.gm.Execute(ctx, startAddress)
}

func (t *Client) UploadBootloader(ctx context.Context, callback model.ProgressCallback) error {
	if err := t.gm.RequestDownload(ctx, false); err != nil {
		return err
	}
	startAddress := 0x102400
	Len := 9996 / 238
	seq := byte(0x21)

	start := time.Now()

	if callback != nil {
		callback(-float64(9997))
		callback(float64(0))
		callback("Uploading bootloader " + strconv.Itoa(len(bootloaderBytes)) + " bytes")
	}

	r := bytes.NewReader(bootloaderBytes)
	pp := 0

	progress := 0

	for i := 0; i < Len; i++ {
		pp++
		if pp == 10 {
			t.gm.TesterPresentNoResponseAllowed()
			pp = 0
		}
		if err := t.gm.TransferData(ctx, 0x00, 0xF0, startAddress); err != nil {
			log.Println(err)
			return err
		}
		seq = 0x21
		for j := 0; j < 0x22; j++ {
			payload := make([]byte, 8)
			payload[0] = seq
			for x := 1; x < 8; x++ {
				b, err := r.ReadByte()
				if err != nil && err != io.EOF {
					return err
				}
				payload[x] = b
				progress++
			}

			tt := gocan.Outgoing
			if j == 0x21 {
				tt = gocan.ResponseRequired
			}
			f := gocan.NewFrame(t.canID, payload, tt)
			if err := t.c.Send(f); err != nil {
				return err
			}

			seq++
			if seq > 0x2F {
				seq = 0x20
			}
			if callback != nil {
				callback(float64(progress))
			}
		}
		resp, err := t.c.Poll(ctx, t.defaultTimeout, t.recvID...)
		if err != nil {
			return err
		}
		if err := gmlan.CheckErr(resp); err != nil {
			log.Println(resp.String())
			return err
		}
		d := resp.Data()
		if d[0] != 0x01 || d[1] != 0x76 {
			return errors.New("invalid transfer data response")
		}
		startAddress += 0xEA
	}

	seq = 0x21

	if err := t.gm.TransferData(ctx, 0x00, 0x0A, startAddress); err != nil {
		return err
	}

	payload := make([]byte, 8)
	payload[0] = seq
	for x := 1; x < 8; x++ {
		b, err := r.ReadByte()
		if err != nil && err != io.EOF {
			return err
		}
		payload[x] = b
	}
	f2 := gocan.NewFrame(t.canID, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, f2, t.defaultTimeout, t.recvID...)
	if err != nil {
		return err
	}

	if err := gmlan.CheckErr(resp); err != nil {
		return err
	}

	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x76 {
		return errors.New("invalid transfer data response")
	}
	t.gm.TesterPresentNoResponseAllowed()

	startAddress += 0x06
	if callback != nil {
		callback(float64(9997))
		callback(fmt.Sprintf("Done, took: %s", time.Since(start).String()))
	}

	return nil
}

func (t *Client) Ping(ctx context.Context) error {
	frame := gocan.NewFrame(t.canID, []byte{0xEF, 0xBE, 0x00, 0x00, 0x00, 0x00, 0x33, 0x66}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, t.recvID...)
	if err != nil {
		return errors.New("LegionPing: " + err.Error())
	}
	if err := gmlan.CheckErr(resp); err != nil {
		return errors.New("LegionPing: " + err.Error())
	}
	d := resp.Data()
	if d[0] == 0xDE && d[1] == 0xAD && d[2] == 0xF0 && d[3] == 0x0F {
		return nil
	}
	return errors.New("LegionPing: no response")
}

func (t *Client) Exit(ctx context.Context) error {
	payload := []byte{0x01, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	frame := gocan.NewFrame(t.canID, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, t.recvID...)
	if err != nil {
		return errors.New("LegionExit: " + err.Error())
	}
	if err := gmlan.CheckErr(resp); err != nil {
		return errors.New("LegionExit: " + err.Error())
	}
	d := resp.Data()
	if d[0] != 0x01 || (d[1] != 0x50 && d[1] != 0x60) {
		return errors.New("LegionExit: invalid response")
	}
	return nil
}

// Set inter frame latency to 0x20(32)
func (t *Client) EnableHighSpeed(ctx context.Context) error {
	_, err := t.IDemand(ctx, SetInterFrameLatency, 5)
	if err != nil {
		return err
	}
	return nil
}

// Commands are as follows:
//
// command 00: Configure packet delay.
//
//	wish:
//	Delay ( default is 2000 )
//
// command 01: Full Checksum-32
//
//	wish:
//	00: Trionic 8.
//	01: Trionic 8; MCP.
//
// command 02: Trionic 8; md5.
// wish:
//
//	00: Full md5.
//	01: Partition 1.
//
// ..
// 09: Partition 9.
//
//	Oddballs:
//	10: Read from 0x00000 to last address of binary + 512 bytes
//	11: Read from 0x04000 to last address of binary + 512 bytes
//	12: Read from 0x20000 to last address of binary + 512 bytes
//
// command 03: Trionic 8 MCP; md5.
//
//	wish:
//	00: Full md5.
//	01: Partition 1.
//	..
//	09: Partition 9 aka 'Shadow'.
//
// command 04: Start secondary bootloader
//
//	wish: None, just wish.
//
// command 05: Marry secondary processor
//
//	wish: None, just wish.
//
// Command 06: Read ADC pin
//
//	whish:
//	Which pin to read.
func (t *Client) IDemand(ctx context.Context, command Command, wish uint16) ([]byte, error) {
	//log.Println("Legion i demand", command.String()+"!")
	payload := []byte{0x02, 0xA5, byte(command), 0x00, 0x00, 0x00, byte(wish >> 8), byte(wish)}
	frame := gocan.NewFrame(t.canID, payload, gocan.ResponseRequired)

	var out []byte

	err := retry.Do(func() error {
		resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, t.recvID...)
		if err != nil {
			return err
		}
		d := resp.Data()

		if err := demandErr(command, d); err != nil {
			return err
		}

		switch command {
		case 0:
			// Settings correctly received.
			//log.Println("settings correctly received")
			return nil
		case 1:
			// Crc-32; complete
			//log.Println("crc-32 complete")
			out = d[4:]
			return nil
		case 2, 3:
			// md5; complete
			b, _, err := t.ReadDataByLocalIdentifier(ctx, true, 7, 0, 16)
			if err != nil {
				return err
			}
			out = b
			//log.Println("md5 complete")
			return nil
		case 4:
			// Secondary loader is alive!
			//log.Println("Master!, Secondary loader alive")
			return nil
		case 5:
			// Marriage; Complete
			//log.Println("Master, the marriage has been completed")
			return nil
		case 6:
			// ADC-read; complete
			out = d[4:5]
			return nil
		default:
			return retry.Unrecoverable(errors.New("command unknown to the legion"))
		}
	},
		retry.Context(ctx),
		retry.Attempts(10),
		retry.LastErrorOnly(true),
		//retry.OnRetry(func(n uint, err error) {
		//	log.Printf("#%d: %v", n, err)
		//}),
	)
	if err != nil {
		return out, err
	}

	return out, nil
}

func (t *Client) ReadFlash(ctx context.Context, device byte, lastAddress int, z22se bool, callback model.ProgressCallback) ([]byte, error) {
	if !t.legionRunning {
		if err := t.Bootstrap(ctx, callback); err != nil {
			return nil, err
		}
	}

	buf := make([]byte, lastAddress)
	bufpnt := 0

	// Pre-fill buffer with 0xFF (unprogrammed FLASH chip value)
	buf[0] = 0xFF
	for j := 1; j < len(buf); j *= 2 {
		copy(buf[j:], buf[:j])
	}

	if callback != nil {
		callback("Downloading " + strconv.Itoa(lastAddress) + " bytes")
		callback(-float64(lastAddress))
		callback(float64(0))
	}

	startAddress := 0
	var blockSize byte = 0x80
	retries := 0
	for startAddress < lastAddress && ctx.Err() == nil {
		if callback != nil {
			callback(float64(startAddress + int(blockSize) - 1))
		}
		err := retry.Do(
			func() error {
				b, blocksToSkip, err := t.ReadDataByLocalIdentifier(ctx, true, device, startAddress, blockSize)
				if err != nil {
					// Increase inter frame latency
					if IsInvalidSequenceError(err) {
						retries++

						if retries == 3 {
							r := uint16(retries * 100)
							if r > 2000 {
								r = 2000
							}
							if _, err2 := t.IDemand(ctx, 0, r); err != nil {
								return fmt.Errorf("failed to set frame latency: %d, previous error: %v now: %v", r, err, err2)
							}
						}
					}
					return err
				}
				if blocksToSkip > 0 {
					//callback("Skipping " + strconv.Itoa(blocksToSkip) + " blocks")
					bufpnt += (blocksToSkip * int(blockSize))
					startAddress += (blocksToSkip * int(blockSize))
				} else if len(b) == int(blockSize) {
					for j := 0; j < int(blockSize); j++ {
						buf[bufpnt] = b[j]
						bufpnt++
					}
					startAddress += int(blockSize)
				} else {
					return fmt.Errorf("dropped frame, len: %d bs: %d", len(b), blockSize)
				}
				return nil
			},
			retry.Attempts(3),
			retry.Context(ctx),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return nil, err
		}
	}
	if callback != nil {
		callback(float64(lastAddress))
	}
	return buf, nil
}

const (
	SetInterFrameLatency     Command = 0x00
	GetCRC32                 Command = 0x01
	GetTrionic8MD5           Command = 0x02
	GetTrionic8MCPMD5        Command = 0x03
	StartSecondaryBootloader Command = 0x04
	MarrySecondaryProcessor  Command = 0x05
	ReadADCPin               Command = 0x06
)

type Command byte

func (c Command) String() string {
	switch c {
	case SetInterFrameLatency:
		return "configure inter frame latency"
	case GetCRC32:
		return "full crc-32"
	case GetTrionic8MD5:
		return "Trionic8 md5"
	case GetTrionic8MCPMD5:
		return "Trionic8 MCP md5"
	case StartSecondaryBootloader:
		return "start secondary bootloader"
	case MarrySecondaryProcessor:
		return "marry secondary processor"
	case ReadADCPin:
		return "read ADC pin"
	default:
		return "unknown command"
	}
}

func demandErr(command Command, d []byte) error {
	if command == 0 && d[3] != 1 {
		return errors.New("failed to set inter frame latency")
	}

	if command == 1 && d[3] != 1 {
		time.Sleep(100 * time.Millisecond)
		return errors.New("crc-32 not ready yet :(")
	}

	if (command == 2 || command == 3) && d[3] != 1 {
		time.Sleep(100 * time.Millisecond)
		return errors.New("md5 not ready yet :(")
	}

	if command == 4 && d[3] != 0x01 {
		time.Sleep(100 * time.Millisecond)
		return errors.New("secondary loader not ready yet")
	}

	if command == 5 {
		switch d[3] {
		case 0xFF:
			// Critical error; Could not start the secondary loader!
			return errors.New("failed to start the secondary loader")
		case 0xFD:
			// Failed to write!
			return errors.New("retrying write")
		case 0xFE:
			// Failed to format!
			return errors.New("retrying format")
		default:
			time.Sleep(500 * time.Millisecond)
			return errors.New("busy marrying")
		}

	}

	if command == 6 && d[3] != 0x01 {
		return fmt.Errorf("ADC read not ready yet")
	}

	// Something is wrong or we sent the wrong command.
	if d[3] == 0xFF {
		return errors.New("bootloader did what it could and failed. Sorry")

	}
	return nil
}

func checkErr(f gocan.CANFrame) error {
	d := f.Data()
	switch {
	case d[0] == 0x7E:
		return errors.New("got 0x7E message as response to 0x21, ReadDataByLocalIdentifier command")
	case bytes.Equal(d, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}):
		return errors.New("got blank response message to 0x21, ReadDataByLocalIdentifier")
	case d[0] == 0x03 && d[1] == 0x7F && d[2] == 0x23:
		return errors.New("no security access granted")
	case d[2] != 0x61 && d[1] != 0x61:
		if bytes.Equal(d, []byte{0x01, 0x7E, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) {
			return fmt.Errorf("incorrect response to 0x21, sendReadDataByLocalIdentifier.  Byte 2 was %X", d[2])
		}
	}
	return nil
}

func (t *Client) ReadDataByLocalIdentifier(ctx context.Context, legionMode bool, pci byte, address int, length byte) ([]byte, int, error) {
	retData := make([]byte, length)
	payload := []byte{pci, 0x21, length,
		byte(address >> 24),
		byte(address >> 16),
		byte(address >> 8),
		byte(address),
		0x00,
	}

	frame := gocan.NewFrame(t.canID, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, t.recvID...)
	if err != nil {
		return nil, 0, err
	}

	if err := checkErr(resp); err != nil {
		return nil, 0, err
	}

	rx_cnt := 0
	var seq byte = 0x21
	d := resp.Data()

	if length <= 4 {
		for i := 4; i < int(4+length); i++ {
			if int(length) > rx_cnt {
				retData[rx_cnt] = d[i]
				rx_cnt++
			}
		}
		return retData, 0, nil
	}

	for i := 0; i < 4; i++ {
		retData[i] = d[i+4]
		rx_cnt++
	}

	if !legionMode || d[3] == 0x00 {
		if err := t.c.SendFrame(t.canID, []byte{0x30}, gocan.CANFrameType{Type: 2, Responses: 18}); err != nil {
			return nil, 0, err
		}
		m_nrFrameToReceive := ((length - 4) / 7)
		if (length-4)%7 > 0 {
			m_nrFrameToReceive++
		}

		c := t.c.Subscribe(ctx, t.recvID...)

		for m_nrFrameToReceive > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case resp := <-c:
				d2 := resp.Data()

				if d2[0] != seq {
					return nil, 0, InvalidSequenceError(fmt.Errorf("received invalid sequenced frame 0x%02X, expected 0x%02X", d2[0], seq))
				}
				for i := 1; i < 8; i++ {
					if rx_cnt < int(length) {
						retData[rx_cnt] = d2[i]
						rx_cnt++
					}
				}
				seq++
				m_nrFrameToReceive--
				if seq > 0x2F {
					seq = 0x20
				}

			case <-time.After(t.defaultTimeout):
				return nil, 0, errors.New("timeout waiting for blocks")
			}
		}
	} else {
		// Loader tagged package as filled with FF
		// (Ie it's not necessary to send a go and receive the rest of the frame,
		// we already know what it contains)
		retData[0] = 0xFF
		for j := 1; j < len(retData); j *= 2 {
			copy(retData[j:], retData[:j])
		}
		return retData, int(d[3]), nil
	}

	return retData, 0, nil
}

type invalidSequenceError struct {
	error
}

func InvalidSequenceError(err error) error {
	return invalidSequenceError{err}
}

func IsInvalidSequenceError(err error) bool {
	_, isIndalidSequenceError := err.(invalidSequenceError)
	return !isIndalidSequenceError
}

func (t *Client) GetMCPVersion(ctx context.Context) (string, error) {

	resp, _, err := t.ReadDataByLocalIdentifier(ctx, true, EcuByte_MCP, 0x8100, 0x80)
	if err != nil {
		return "", err
	}
	out := make([]byte, 10)
	for i := 0; i < 10; i++ {
		out[i] = resp[0xC+i]
	}

	return string(out[:]), nil
}
