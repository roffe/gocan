package t7

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
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
}{
	{0x000000, 0x000000, 0x07B000},
	{0x07FF00, 0x07FF00, 0x080000},
}

// Flash the ECU
func (t *Client) FlashECU(ctx context.Context, bin []byte, callback model.ProgressCallback) error {
	if bin[0] != 0xFF || bin[1] != 0xFF || bin[2] != 0xEF || bin[3] != 0xFC {
		return fmt.Errorf("error: bin doesn't appear to be for a Trionic 7 ECU! (%02X%02X%02X%02X)",
			bin[0], bin[1], bin[2], bin[3])
	}

	if err := t.DataInitialization(ctx, callback); err != nil {
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
		err := retry.Do(func() error {
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
				if err := t.writeRange(ctx, binPos, binPos+writeBytes, bin); err != nil {
					return err
				}
				binPos += writeBytes
				left -= writeBytes
				if callback != nil {
					callback(float64(binPos))
				}
				time.Sleep(5 * time.Millisecond)
			}
			if callback != nil {
				callback(float64(binPos))
			}
			return nil
		},
			retry.Context(ctx),
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return err
		}
	}
	end, err := t.c.SendAndPoll(ctx, gocan.NewFrame(0x240, []byte{0x40, 0xA1, 0x01, 0x37, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired), t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("error waiting for data transfer exit reply: %v", err)
	}
	// Send acknowledgement
	d := end.Data()
	t.Ack(d[0], gocan.Outgoing)

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
	jumpMsg := []byte{0x41, 0xA1, 0x08, 0x34, byte(offset >> 16), byte(offset >> 8), byte(offset), 0x00}
	jumpMsg2 := []byte{0x00, 0xA1, byte(length >> 16), byte(length >> 8), byte(length), 0x00, 0x00, 0x00}

	log.Printf("writeJump: offset=%d, length=%d", offset, length)
	log.Printf("writeJump: jumpMsg=%X", jumpMsg)
	if err := t.c.SendFrame(0x240, jumpMsg, gocan.Outgoing); err != nil {
		return fmt.Errorf("failed to enable request download #1")
	}
	log.Printf("writeJump: jumpMsg2=%X", jumpMsg2)
	time.Sleep(5 * time.Millisecond)

	f, err := t.c.SendAndPoll(ctx, gocan.NewFrame(0x240, jumpMsg2, gocan.ResponseRequired), t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("failed to enable request download #2")
	}

	d := f.Data()
	t.Ack(d[0], gocan.Outgoing)

	if d[3] != 0x74 {
		log.Println(f.String())
		return fmt.Errorf("invalid response enabling download mode")
	}
	return nil
}

func (t *Client) writeRange(ctx context.Context, start, end int, bin []byte) error {
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
			for fq := 4; fq < 8; fq++ {
				data[fq] = bin[binPos]
				binPos++
			}
			first = false
		} else if i == 0 {
			left := end - binPos
			if left > 6 {
				return fmt.Errorf("sequence is fucked, tell roffe") // this should never happend
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
			t.c.SendFrame(0x240, data, gocan.Outgoing)
			continue
		}
		t.c.SendFrame(0x240, data, gocan.ResponseRequired)
	}

	resp, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("error writing 0x%X - 0x%X was at pos 0x%X: %v", start, end, binPos, err)
	}

	// Send acknowledgement
	d := resp.Data()
	t.Ack(d[0], gocan.Outgoing)

	if d[3] != 0x76 {
		return fmt.Errorf("ECU did not confirm write")
	}
	return nil
}
