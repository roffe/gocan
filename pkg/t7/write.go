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

	"github.com/avast/retry-go"
	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

func (t *Trionic) LoadBinFile(filename string) (int64, []byte, error) {
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

var offsets = []struct {
	binpos int
	offset int
	end    int
}{
	{0x000000, 0x000000, 0x07B000},
	{0x07FF00, 0x07FF00, 0x080000},
}

// Flash the ECU
func (t *Trionic) Flash(ctx context.Context, bin []byte) error {
	if err := t.DataInitialization(ctx); err != nil {
		return err
	}
	if bin[0] != 0xFF || bin[1] != 0xFF || bin[2] != 0xEF || bin[3] != 0xFC {
		return fmt.Errorf("error: bin doesn't appear to be for a Trionic 7 ECU! (%02X%02X%02X%02X)",
			bin[0], bin[1], bin[2], bin[3])
	}

	bar := progressbar.NewOptions(0x7FFF0,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[cyan][2/2][reset] flashing ECU"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	maxRetries := 30
	retries := 0
	start := time.Now()
	for _, o := range offsets {
		binPos := o.binpos
		err := retry.Do(
			func() error {
				return t.writeJump(ctx, o.offset, o.end-binPos)
			},
			retry.Context(ctx),
			retry.Attempts(5),
			retry.OnRetry(func(n uint, err error) {
				fmt.Println()
				log.Println(err)
			}),
		)
		if err != nil {
			return errors.New("write jump failed")
		}
		for binPos < o.end {
			bar.Set(binPos)
			left := o.end - binPos
			var writeBytes int
			if left >= 0xF0 {
				writeBytes = 0xF0
			} else {
				writeBytes = left
			}
			err := t.writeRange(ctx, binPos, binPos+writeBytes, bin)
			if err != nil {
				if retries < maxRetries {
					log.Println()
					log.Println(err)
					time.Sleep(100 * time.Millisecond)
					err := retry.Do(
						func() error {
							return t.writeJump(ctx, binPos, o.end-binPos)
						},
						retry.Context(ctx),
						retry.Attempts(5),
						retry.OnRetry(func(n uint, err error) {
							log.Println()
							log.Println(err)
						}),
					)
					if err != nil {
						return fmt.Errorf("jump failed: %v", err)
					}
					retries++
					continue
				}
				return err
			}
			binPos += writeBytes
			left -= writeBytes
		}
		bar.Set(binPos)
	}

	t.c.SendFrame(0x240, []byte{0x40, 0xA1, 0x01, 0x37, 0x00, 0x00, 0x00, 0x00}) // end data transfer mode
	end, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("error waiting for data transfer exit reply: %v", err)
	}
	// Send acknowledgement
	t.Ack(end.Data[0] & 0xBF)

	if end.Data[3] != 0x77 {
		return errors.New("exit download mode failed")
	}

	bar.Finish()
	log.Println(", flash successfull. took: ", time.Since(start).Round(time.Second))
	return nil
}

// send request "Download - tool to module" to Trionic"
func (t *Trionic) writeJump(ctx context.Context, offset, length int) error {
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

	t.c.SendFrame(0x240, jumpMsg)
	t.c.SendFrame(0x240, jumpMsg2)
	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("failed to enable request download")
	}

	t.Ack(f.Data[0])
	if f.Data[3] != 0x74 {
		return fmt.Errorf("invalid response enabling download mode")
	}

	return nil
}

func (t *Trionic) writeRange(ctx context.Context, start, end int, bin []byte) error {
	length := end - start
	binPos := start
	// length / 6
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

		t.c.SendFrame(0x240, data)
	}

	f2, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
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

/*
func (t *Trionic) writeDataBlock(ctx context.Context, headerId byte, d []byte) error {
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
	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
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

func (t *Trionic) Program2(ctx context.Context, bin []byte) error {
	binCount := 0
	//n := len(bin)
	//bInfo := GetBinInfo(bin)
	// Check that the file begins with FF FF EF FC
	if bin[0] != 0xFF || bin[1] != 0xFF || bin[2] != 0xEF || bin[3] != 0xFC {
		return fmt.Errorf("error: bin doesn't appear to be for a Trionic 7 ECU! (%02X%02X%02X%02X)",
			bin[0], bin[1], bin[2], bin[3])
	}

	jumpMsg1a := []byte{0x41, 0xA1, 0x08, 0x34, 0x00, 0x00, 0x00, 0x00} // 0x000000 length=0x07B000
	jumpMsg1b := []byte{0x00, 0xA1, 0x07, 0xB0, 0x00, 0x00, 0x00, 0x00}
	jumpMsg2a := []byte{0x41, 0xA1, 0x08, 0x34, 0x07, 0xFF, 0x00, 0x00} // 0x07FF00 length=0x000100
	jumpMsg2b := []byte{0x00, 0xA1, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	endDataMsg := []byte{0x40, 0xA1, 0x01, 0x37, 0x00, 0x00, 0x00, 0x00}
	//exitDiagMsg := []byte{0x40, 0xA1, 0x02, 0x31, 0x54, 0x00, 0x00, 0x00}
	//reqdiagResultMsg := []byte{0x3F, 0x81, 0x01, 0x33, 0x02, 0x40, 0x00, 0x00} // 220h

	ack := []byte{0x40, 0xA1, 0x3F, 0x00, 0x00, 0x00, 0x00, 0x00} // 266h

	// send request "Download - tool to module" to Trionic"
	t.c.SendFrame(0x240, jumpMsg1a)
	t.c.SendFrame(0x240, jumpMsg1b)
	f, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to enable request download")
	}

	ack[3] = f.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)

	if f.Data[3] != 0x74 {
		return fmt.Errorf("invalid response to enabling download mode request")
	}

	c := 0
	for binCount < 0x7B000 {
		left := 0x7B000 - binCount
		var leng byte
		if left >= 241 {
			leng = 0xF1
		} else {
			leng = byte(left)
		}

		var data = make([]byte, 8)
		data[1] = 0xA1

		bytesThisRound := 0

		for i := 0x28; i >= 0; i-- {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			data[0] = byte(i)
			if i == 0x28 {
				data[0] |= 0x40
				data[2] = leng // length
				data[3] = 0x36 // Data Transfer
				data[4] = bin[binCount]
				binCount++
				bytesThisRound++
				data[5] = bin[binCount]
				binCount++
				bytesThisRound++
				data[6] = bin[binCount]
				binCount++
				bytesThisRound++
				data[7] = bin[binCount]
				binCount++
				bytesThisRound++
			} else if i == 0 {
				data[2] = bin[binCount]
				binCount++
				bytesThisRound++
				data[3] = bin[binCount]
				binCount++
				bytesThisRound++
				for k := 4; k < 8; k++ {
					data[k] = 0x00
				}
			} else {
				for k := 2; k < 8; k++ {
					data[k] = bin[binCount]
					binCount++
					bytesThisRound++
				}
			}
			// time.Sleep(3*time.Millisecond)
			t.c.SendFrame(0x240, data)
		}
		f2, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
		if err != nil {

			return fmt.Errorf("error writing was at pos 0x%X: %v", binCount, err)
		}
		// Send acknowledgement
		ack[3] = f2.Data[0] & 0xBF
		t.c.SendFrame(0x266, ack)
		if f2.Data[3] != 0x76 {

			return fmt.Errorf("ecu did not ack write")
		}
		c++
		if c == 100 {

			c = 0
		}
	}

	var data = make([]byte, 8)

	// Send "Request Download - tool to module" to Trionic
	// (i.e. jump to address 0x7FF00)
	t.c.SendFrame(0x240, jumpMsg2a)
	t.c.SendFrame(0x240, jumpMsg2b)

	// Read response
	f4, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
	if err != nil {

		return fmt.Errorf("failed to read jump response: %v", err)
	}

	// Send acknowledgement
	ack[3] = f4.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)

	if f4.Data[3] != 0x74 {

		return errors.New("flash has failed")
	}

	binCount = 0x7FF01
	// Send 0x7FF00...7FFF0
	data[1] = 0xA1
	for i := 0x28; i >= 0; i-- {
		data[0] = byte(i)
		if i == 0x28 {
			data[0] |= 0x40
			data[2] = 0xF1 // length
			data[3] = 0x36 // Data Transfer
			data[4] = bin[binCount]
			binCount++
			data[5] = bin[binCount]
			binCount++
			data[6] = bin[binCount]
			binCount++
			data[7] = bin[binCount]
		} else if i == 0 {
			data[2] = bin[binCount]
			binCount++
			data[3] = bin[binCount]
			binCount++
			for k := 4; k < 8; k++ {
				data[k] = 0x00
			}
		} else {
			for k := 2; k < 8; k++ {
				data[k] = bin[binCount]
				binCount++
			}
		}
		t.c.SendFrame(0x240, data)

	}
	// Read response
	f5, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
	if err != nil {

		return fmt.Errorf("failed to write to ecu: %v", err)
	}

	ack[3] = f5.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)
	if f5.Data[3] != 0x76 {

		return fmt.Errorf("flash has failed")
	}

	// Send 0x7FFF0...0x7FFFF
	data[1] = 0xA1
	for i := 2; i >= 0; i-- {
		data[0] = byte(i)
		if i == 2 {
			data[0] |= 0x40
			data[2] = 0x11 // length
			data[3] = 0x36 // Data Transfer
			data[4] = bin[binCount]
			binCount++
			data[5] = bin[binCount]
			binCount++
			data[6] = bin[binCount]
			binCount++
			data[7] = bin[binCount]
			binCount++
		} else {
			for k := 2; k < 8; k++ {
				data[k] = bin[binCount]
				binCount++
			}
		}

		time.Sleep(2 * time.Millisecond)
		t.c.SendFrame(0x240, data)
	}
	f6, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
	if err != nil {

		return fmt.Errorf("flash failed :%v", err)
	}

	// Send acknowledgement
	ack[3] = f6.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)
	if f6.Data[3] != 0x76 {

		return fmt.Errorf("flash failed :%v", err)
	}

	t.c.SendFrame(0x240, endDataMsg)
	f7, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
	if err != nil {

		return fmt.Errorf("error waiting for data transfer exit ack: %v", err)
	}

	// Send acknowledgement
	ack[3] = f7.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)

	if f7.Data[3] != 0x77 {

		return errors.New("failed to exit data transfer mode")
	}

	log.Println("flash successfull")

	return nil
}

	blocks := []struct {
		offset byte
		data   []byte
	}{
		{0x90, []byte(bInfo.Vin)},
		{0x91, []byte(bInfo.HwPartNo)},
		{0x92, []byte(bInfo.ImmoCode)},
		{0x94, []byte(bInfo.SoftwarePartNo)},
		{0x95, []byte(bInfo.SoftwareVersion)},
		{0x97, []byte(bInfo.EngineType)},
		{0x98, []byte(bInfo.Tester)},
		{0x99, []byte(bInfo.SoftwareDate)},
	}

	for _, b := range blocks {
		if err := t.WriteDataBlock(ctx, b.offset, b.data); err != nil {
			log.Printf("failed to set data block %X: %v\n", b.offset, err)
		}
	}

	t.c.SendFrame(0x240, exitDiagMsg)
	f8, err := t.c.Poll(ctx, 0x258, 150*time.Millisecond)
	if err != nil {
		return err
	}

	if f8.Data[3] != 0x71 {
		return err
	}
	// Send acknowledgement
	ack[3] = data[0] & 0xBF
	t.c.SendFrame(0x266, ack)

	t.c.SendFrame(0x220, reqdiagResultMsg)
	diag, err := t.c.Poll(ctx, 0x239, 150*time.Millisecond)
	if err != nil {
		log.Println(err)
	} else {
		log.Printf("\nDiagnostic results...\n")

		for k := 0; k < 8; k++ {
			fmt.Printf("0x%02X ", diag.Data[k])
		}

	}
*/
