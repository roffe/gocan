package t7

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// send request "Download - tool to module" to Trionic"
func (t *Trionic) WriteJump(ctx context.Context, offset, length int) error {
	jumpMsg := []byte{0x41, 0xA1, 0x08, 0x34, 0x00, 0x00, 0x00, 0x00}
	jumpMsg2 := []byte{0x00, 0xA1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	b, err := hex.DecodeString(fmt.Sprintf("%06X", offset))
	if err != nil {
		return err
	}

	b2, err := hex.DecodeString(fmt.Sprintf("%06X", length))
	if err != nil {
		return err
	}

	for k := 4; k < 7; k++ {
		jumpMsg[k] = b[k-4]
	}

	for k := 2; k < 5; k++ {
		jumpMsg2[k] = b2[k-2]
	}

	//log.Printf("%X\n", jumpMsg)
	//log.Printf("%X\n", jumpMsg2)

	t.c.SendFrame(0x240, jumpMsg)
	t.c.SendFrame(0x240, jumpMsg2)
	f, err := t.c.Poll(ctx, 0x258, t.defaultTimeout)
	if err != nil {
		return fmt.Errorf("failed to enable request download")
	}

	t.Ack(f.Data[0])
	if f.Data[3] != 0x74 {
		return fmt.Errorf("invalid response enabling download mode")
	}

	return nil
}

func (t *Trionic) WriteRange(ctx context.Context, start, end int, bin []byte) error {
	length := end - start
	binPos := start
	rows := length / 6
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
			if left > 6 {
				log.Fatal("sequence is fucked, tell roffe") // this should never happend
			}
		} else {
			for k := 2; k < 8; k++ {
				data[k] = bin[binPos]
				binPos++
			}
		}

		t.c.SendFrame(0x240, data)
	}

	f2, err := t.c.Poll(ctx, 0x258, t.defaultTimeout)
	if err != nil {
		return fmt.Errorf("error writing 0x%X - 0x%X was at pos 0x%X: %v", start, end, binPos, err)
	}
	// Send acknowledgement
	t.Ack(f2.Data[0])
	if f2.Data[3] != 0x76 {
		return fmt.Errorf("ECU did not confirm write")
	}
	return nil
}

func (t *Trionic) WriteDataBlock(ctx context.Context, headerId byte, d []byte) error {
	var blockPos int
	var length, rows int
	var i, k int
	var rowTemp float64
	write := []byte{0x40, 0xA1, 0x00, 0x3B, 0x00, 0x00, 0x00, 0x00}

	block := []byte(strings.TrimSpace(string(d)))
	length = len(block)

	rowTemp = math.Floor(float64((length + 3)) / float64(6))
	rows = int(rowTemp)

	// Send "Write data block" to Trionic
	write[2] = byte(length + 2)
	write[4] = headerId

	for i = rows; i >= 0; i-- {
		if i == rows {
			write[0] = byte(i) | 0x40
			write[5] = block[blockPos]
			blockPos++
			write[6] = block[blockPos]
			blockPos++
			write[7] = block[blockPos]
			blockPos++
		} else {
			write[0] = byte(i)
			for k = 2; k < 8; k++ {
				if blockPos < int(length) {
					write[k] = block[blockPos]
					blockPos++
				} else {
					write[k] = 0x00

				}
			}
		}

		t.c.SendFrame(0x240, write)
	}

	// Read response message
	f, err := t.c.Poll(ctx, 0x258, t.defaultTimeout)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	fmt.Println(f.String())
	// Send acknowledgement
	t.Ack(f.Data[0])
	if f.Data[3] != 0x7B && f.Data[4] != byte(headerId) {
		return fmt.Errorf("block write failed: %s", f.String())
	}
	time.Sleep(10 * time.Millisecond)
	return nil

}
