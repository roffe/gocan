package gocan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/albenik/bcd"
)

func canusbDecodeFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	/* leng, err := hex.DecodeString("0" + string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}
	msgLen := int(leng[0])
	if msgLen > 8 {
		log.Println("msgLen", msgLen)
	} */

	//data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))

	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return NewFrame(
		uint32(id),
		data,
		Incoming,
	), nil
}

// T 00000180 8 2D 12 09 DF 87 56 91 06
func canusbDecodeExtendedFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:9]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	/* leng, err := hex.DecodeString("0" + string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}
	msgLen := int(leng[0])
	if msgLen > 8 {
		log.Println("msgLen", msgLen)
	} */

	//data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))

	data, err := hex.DecodeString(string(buff[10:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return NewExtendedFrame(
		uint32(id),
		data,
		Incoming,
	), nil
}

/*
Bit 0 CAN receive FIFO queue full
Bit 1 CAN transmit FIFO queue full
Bit 2 Error warning (EI), see SJA1000 datasheet
Bit 3 Data Overrun (DOI), see SJA1000 datasheet
Bit 4 Not used.
Bit 5 Error Passive (EPI), see SJA1000 datasheet
Bit 6 Arbitration Lost (ALI), see SJA1000 datasheet *
Bit 7 Bus Error (BEI), see SJA1000 datasheet **
* Arbitration lost doesn’t generate a blinking RED light!
** Bus Error generates a constant RED ligh
*/

func canusbDecodeStatus(b []byte) error {
	bs := int(bcd.ToUint16(b[1:]))
	//log.Printf("%08b\n", bs)
	switch {
	case checkBitSet(bs, 1):
		return errors.New("CAN receive FIFO queue full")
	case checkBitSet(bs, 2):
		return errors.New("CAN transmit FIFO queue full")
	case checkBitSet(bs, 3):
		return errors.New("error warning (EI)")
	case checkBitSet(bs, 4):
		return errors.New("data overrun (DOI)") // see SJA1000 datasheet
	case checkBitSet(bs, 5):
		return errors.New("not used")
	case checkBitSet(bs, 6):
		return errors.New("error passive (EPI)") // see SJA1000 datasheet
	case checkBitSet(bs, 7):
		return errors.New("arbitration lost (ALI)") // see SJA1000 datasheet *
	case checkBitSet(bs, 8):
		return errors.New("bus error (BEI)") // see SJA1000 datasheet **"

	}
	return nil
}

func checkBitSet(n, k int) bool {
	v := n & (1 << (k - 1))
	return v == 1
}

func canusbClose(port io.Writer) error {
	if _, err := port.Write([]byte("C\r")); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func canusbInit(port io.Writer, canRate, filter, mask string) error {
	var cmds = []string{
		"C", "", "", // Empty buffer
		"V", // Get Version number of both CANUSB hardware and software
		//"N",        // Get Serial number of the CANUSB
		"Z0",    // Sets Time Stamp OFF for received frames
		canRate, // Setup CAN bit-rates
		filter,
		mask,
		//"O", // Open the CAN channel
	}

	for _, c := range cmds {
		_, err := port.Write([]byte(c + "\r"))
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func canusbSetFilter(cu *BaseAdapter, filters []uint32) error {
	filter, mask := canusbAcceptanceFilters(filters)
	cu.Send() <- NewFrame(SystemMsg, []byte{'C'}, Outgoing)
	cu.Send() <- NewFrame(SystemMsg, []byte(filter), Outgoing)
	cu.Send() <- NewFrame(SystemMsg, []byte(mask), Outgoing)
	cu.Send() <- NewFrame(SystemMsg, []byte{'O'}, Outgoing)
	return nil
}

func canusbCANrate(rate float64) (string, error) {
	switch rate {
	case 10:
		return "S0", nil
	case 20:
		return "S1", nil
	case 33.3:
		return "s0e1c", nil
	case 47.619:
		return "scb9a", nil
	case 50:
		return "S2", nil
	case 100:
		return "S3", nil
	case 125:
		return "S4", nil
	case 250:
		return "S5", nil
	case 500:
		return "S6", nil
	case 615.384:
		return "s4037", nil
	case 800:
		return "S7", nil
	case 1000:
		return "S8", nil
	default:
		return "", fmt.Errorf("unknown rate: %f", rate)
	}
}

func canusbAcceptanceFilters(idList []uint32) (string, string) {
	if len(idList) == 1 && idList[0] == 0 {
		return "\r", "\r"
	}
	var code = ^uint32(0)
	var mask uint32 = 0

	if len(idList) == 0 {
		code = 0
		mask = ^uint32(0)
	} else {
		code = 0
		for _, canID := range idList {
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16

	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

func canusbCreateParser(debug, printVersion bool, buff *bytes.Buffer, sendSem <-chan struct{}, recvChan chan<- *CANFrame, errorFunc func(error), logFunc func(string)) func([]byte) {
	return func(data []byte) {
		for _, b := range data {
			if b == 0x07 { // BELL
				errorFunc(errors.New("command error"))
				select {
				case <-sendSem:
				default:
				}
				continue
			}
			if b != 0x0D { // CR
				buff.WriteByte(b)
				continue
			}
			if buff.Len() == 0 {
				continue
			}
			by := buff.Bytes()
			if debug {
				logFunc("<< " + buff.String())
			}
			switch by[0] {
			case 'F':
				select {
				case <-sendSem:
				default:
				}
				if err := canusbDecodeStatus(by); err != nil {
					errorFunc(fmt.Errorf("CAN status error: %w", err))
				}
			case 't':
				f, err := canusbDecodeFrame(by)
				if err != nil {
					errorFunc(fmt.Errorf("failed to decode frame: %w", err))
					buff.Reset()
					continue
				}
				select {
				case recvChan <- f:
				default:
					errorFunc(ErrDroppedFrame)
				}
				buff.Reset()
			case 'T':
				f, err := canusbDecodeExtendedFrame(by)
				if err != nil {
					errorFunc(fmt.Errorf("failed to decode frame: %w", err))
					buff.Reset()
					continue
				}
				select {
				case recvChan <- f:
				default:
					errorFunc(ErrDroppedFrame)
				}
				buff.Reset()
			case 'z': // last command ok
				select {
				case <-sendSem:
				default:
				}
			case 'V':
				if printVersion {
					logFunc("H/W version " + buff.String())
				}
			case 'N':
				if printVersion {
					logFunc("H/W serial " + buff.String())
				}
			default:
				logFunc("Unknown>> " + buff.String())
			}
			buff.Reset()
		}
	}
}

func canusbSendManager(
	ctx context.Context,
	closeChan <-chan struct{},
	sendSem chan struct{},
	port io.Writer,
	sendChan <-chan *CANFrame,
	onError func(error),
	onMessage func(string),
	debug bool,
) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Scratch for ID byte packing (reused)
	var idBuff [4]byte

	// Nibble to ASCII hex (lowercase to match hex.EncodeToString)
	hexNib := func(n byte) byte {
		n &= 0xF
		if n < 10 {
			return '0' + n
		}
		return 'a' + (n - 10)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-closeChan:
			return
		case <-ticker.C:
			sendSem <- struct{}{}
			_, _ = port.Write([]byte{'F', '\r'})
		case msg := <-sendChan:
			// System messages pass-through
			if id := msg.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if debug {
						onMessage(">> " + string(msg.Data))
					}
					if _, err := port.Write(msg.Data); err != nil {
						onError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
						return
					}
					if _, err := port.Write([]byte{'\r'}); err != nil {
						onError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
						return
					}
				}
				continue
			}

			sendSem <- struct{}{}

			// Build CAN frame into a fixed scratch buffer.
			// Max classic CAN frame length: 27 bytes as explained above.
			var out [27]byte
			i := 0

			dlc := msg.Length() // assume classic CAN 0..8
			data := msg.Data[:dlc]

			if msg.Extended {
				// 'T' + 8 hex of 29-bit ID
				out[i] = 'T'
				i++

				binary.BigEndian.PutUint32(idBuff[:], msg.Identifier&0x1FFFFFFF)
				// hex.Encode uses lowercase like hex.EncodeToString
				i += hex.Encode(out[i:i+8], idBuff[:])

			} else {
				// 't' + 3 hex of 11-bit ID
				out[i] = 't'
				i++

				id := uint16(msg.Identifier & 0x7FF)
				// three hex nibbles: high->low (11 bits => 3 hex digits)
				out[i+0] = hexNib(byte(id >> 8))
				out[i+1] = hexNib(byte(id >> 4))
				out[i+2] = hexNib(byte(id))
				i += 3
			}

			// DLC (single ASCII digit for classic CAN)
			out[i] = '0' + byte(dlc) // safe for 0..8
			i++

			// Data bytes as hex
			i += hex.Encode(out[i:i+2*len(data)], data)

			// Terminator
			out[i] = '\r'
			i++

			if _, err := port.Write(out[:i]); err != nil {
				onError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
				return
			}

			if debug {
				onMessage(">> " + string(out[:i]))
			}
		}
	}
}

/*
func canusbSendManager(
	ctx context.Context,
	closeChan <-chan struct{},
	sendSem chan struct{},
	port io.Writer,
	sendChan <-chan *CANFrame,
	onError func(error),
	onMessage func(string),
	debug bool,

) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	idBuff := make([]byte, 4)

	for {
		select {
		case <-ctx.Done():
			return
		case <-closeChan:
			return
		case <-ticker.C:
			sendSem <- struct{}{}
			port.Write([]byte{'F', '\r'})
		case msg := <-sendChan:
			if id := msg.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if debug {
						onMessage(">> " + string(msg.Data))
					}
					if _, err := port.Write(append(msg.Data, '\r')); err != nil {
						onError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
						return
					}
				}
				continue
			}

			sendSem <- struct{}{}

			var out []byte
			if msg.Extended {
				binary.BigEndian.PutUint32(idBuff, msg.Identifier&0x1FFFFFFF)
				out = []byte("T" + hex.EncodeToString(idBuff) + strconv.Itoa(msg.Length()) + hex.EncodeToString(msg.Data) + "\r")
				if _, err := port.Write(out); err != nil {
					onError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
					return
				}
			} else {
				binary.BigEndian.PutUint32(idBuff, msg.Identifier&0x7FF)
				out = []byte("t" + hex.EncodeToString(idBuff)[5:] + strconv.Itoa(msg.Length()) + hex.EncodeToString(msg.Data) + "\r")
				if _, err := port.Write(out); err != nil {
					onError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
					return
				}
			}

			if debug {
				onMessage(">> " + string(out))
			}

		}
	}
}
*/
