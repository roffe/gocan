package t7

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/roffe/gocan/pkg/frame"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) LoadBinFile(filename string) (int64, []byte, error) {
	var temp byte
	readBytes := 0
	data, err := os.ReadFile(filename)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read bin file: %v", err)
	}
	readBytes = len(data)

	if readBytes == 256*1024 {
		return 0, nil, errors.New("error: is this a Trionic 5 ECU binary?")
	}
	if readBytes == 512*1024 || readBytes == 0x70100 {
		// Convert Motorola byte-order to Intel byte-order (just in RAM)
		if data[0] == 0xFF && data[1] == 0xFF && data[2] == 0xFC && data[3] == 0xEF {
			log.Println("note: Motorola byte-order detected.")
			for i := 0; i < readBytes; i += 2 {
				temp = data[i]
				data[i] = data[i+1]
				data[i+1] = temp
			}
		}
	}

	if readBytes == 512*1024 || readBytes == 0x70100 {
		return int64(readBytes), data, nil
	}

	return int64(readBytes), nil, errors.New("invalid bin size")
}

var t7offsets = []struct {
	binpos int
	offset int
	end    int
	delay  time.Duration
}{
	{0x000000, 0x000000, 0x07B000, 0},
	{0x07FF00, 0x07FF00, 0x080000, 10 * time.Nanosecond},
}

// Flash the ECU
func (t *Client) FlashECU(ctx context.Context, bin []byte, callback model.ProgressCallback) error {
	if bin[0] != 0xFF || bin[1] != 0xFF || bin[2] != 0xEF || bin[3] != 0xFC {
		return fmt.Errorf("error: bin doesn't appear to be for a Trionic 7 ECU! (%02X%02X%02X%02X)",
			bin[0], bin[1], bin[2], bin[3])
	}

	if err := t.DataInitialization(ctx); err != nil {
		return err
	}

	ok, err := t.KnockKnock(ctx, callback)
	if err != nil || !ok {
		return fmt.Errorf("failed to authenticate: %v", err)
	}

	if err := t.EraseECU(ctx, callback); err != nil {
		return err
	}

	if callback != nil {
		callback(-float64(0x80000))
		callback("Flashing ECU")
	}

	start := time.Now()
	for _, o := range t7offsets {
		binPos := o.binpos
		if err := t.writeJump(ctx, o.offset, o.end-binPos); err != nil {
			return err
		}
		for binPos < o.end {
			left := o.end - binPos
			var writeBytes int
			if left >= 0xF0 {
				writeBytes = 0xF0
			} else {
				writeBytes = left
			}
			if err := t.writeRange(ctx, binPos, binPos+writeBytes, bin, o.delay); err != nil {
				return err
			}
			binPos += writeBytes
			left -= writeBytes
			if callback != nil {
				callback(float64(binPos))
			}
		}
		if callback != nil {
			callback(float64(binPos))
		}
	}

	end, err := t.c.SendAndPoll(ctx, frame.New(0x240, []byte{0x40, 0xA1, 0x01, 0x37, 0x00, 0x00, 0x00, 0x00}, frame.ResponseRequired), t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("error waiting for data transfer exit reply: %v", err)
	}
	// Send acknowledgement
	d := end.Data()
	t.Ack(d[0], frame.Outgoing)

	if d[3] != 0x77 {
		return errors.New("exit download mode failed")
	}

	if callback != nil {
		callback(fmt.Sprintf("Done, took: %s", time.Since(start).Round(time.Second)))
	}
	return nil
}

// send request "Download - tool to module" to Trionic"
func (t *Client) writeJump(ctx context.Context, offset, length int) error {
	jumpMsg := []byte{0x41, 0xA1, 0x08, 0x34, 0x00, 0x00, 0x00, 0x00}
	jumpMsg2 := []byte{0x00, 0xA1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	offsetBytes, err := hex.DecodeString(fmt.Sprintf("%06X", offset))
	if err != nil {
		return err
	}

	lengthBytes, err := hex.DecodeString(fmt.Sprintf("%06X", length))
	if err != nil {
		return err
	}

	for k := 4; k < 7; k++ {
		jumpMsg[k] = offsetBytes[k-4]
	}

	for k := 2; k < 5; k++ {
		jumpMsg2[k] = lengthBytes[k-2]
	}

	t.c.SendFrame(0x240, jumpMsg, frame.Outgoing)
	f, err := t.c.SendAndPoll(ctx, frame.New(0x240, jumpMsg2, frame.ResponseRequired), t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("failed to enable request download")
	}
	d := f.Data()
	t.Ack(d[0], frame.Outgoing)

	if d[3] != 0x74 {
		return fmt.Errorf("invalid response enabling download mode")
	}
	return nil
}

func (t *Client) writeRange(ctx context.Context, start, end int, bin []byte, delay time.Duration) error {
	length := end - start
	binPos := start
	rows := int(math.Floor(float64((length + 3)) / 6.0))
	first := true
	for i := rows; i >= 0; i-- {
		var data = make([]byte, 8)
		data[1] = 0xA1
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data[0] = byte(i)
		if first {
			data[0] |= 0x40
			data[2] = byte(length + 1) // length
			data[3] = 0x36             // Data Transfer
			data[4] = bin[binPos]
			binPos++
			data[5] = bin[binPos]
			binPos++
			data[6] = bin[binPos]
			binPos++
			data[7] = bin[binPos]
			binPos++
			first = false
		} else if i == 0 {
			left := end - binPos
			if left > 6 {
				log.Fatal("sequence is fucked, tell roffe") // this should never happend
			}
			for i := 0; i < left; i++ {
				data[2+i] = bin[binPos]
				binPos++
			}
			if left <= 6 {
				fill := 8 - left
				for i := left; i < fill; i++ {
					data[left+2] = 0x00
				}
			}
		} else {
			for k := 2; k < 8; k++ {
				data[k] = bin[binPos]
				binPos++
			}
		}
		if i > 0 {
			//t.c.SendFrame(0x240, data, frame.OptFrameType(frame.Outgoing))
			//if delay > 0 {
			//	time.Sleep(delay)
			//}
			t.c.Send(frame.New(0x240, data, frame.Outgoing))
			continue
		}
		t.c.SendFrame(0x240, data, frame.ResponseRequired)
	}

	resp, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("error writing 0x%X - 0x%X was at pos 0x%X: %v", start, end, binPos, err)
	}
	// Send acknowledgement
	d := resp.Data()
	t.Ack(d[0], frame.Outgoing)
	if d[3] != 0x76 {
		//log.Println(f2.String())
		return fmt.Errorf("ECU did not confirm write")
	}
	return nil
}
