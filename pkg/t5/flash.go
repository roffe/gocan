package t5

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/roffe/gocan/pkg/bar"
)

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	var bytesRead uint32

	ecutype, err := t.DetermineECU(ctx)
	if err != nil {
		return err
	}

	start := getstartAddress(ecutype)

	bar := bar.New(len(bin), "flashing ECU")
	defer func() {
		if !bar.IsFinished() {
			bar.Finish()
			fmt.Println()
		}
	}()

	r := bytes.NewReader(bin)
	startTime := time.Now()
	for (start + bytesRead) < 0x80000 {
		buff := make([]byte, 0x80)
		n, err := r.Read(buff)
		if err != nil {
			return err
		}
		if n != 0x80 {
			return fmt.Errorf("reading the BIN failed after: 0x%X bytes", bytesRead)
		}

		var FFBlock byte = 0x80
		for i := 0; i < 0x80; i++ {
			if buff[i] == 0xFF {
				FFBlock--
			}
		}

		if FFBlock > 0 {
			if err := t.sendBootloaderAddressCommand(ctx, start+bytesRead, 0x80); err != nil {
				return err
			}
			data := make([]byte, 8)
			for i := 0; i < 0x80; i++ {
				// set the index number
				if i%7 == 0 {
					data[0] = byte(i)
				}
				// put bytes them in the dataframe!
				data[(i%7)+1] = buff[i]

				// send a bootloader frame whenever 7 bytes or a block of 0x80 bytes have been read from the BIN file
				if i%7 == 6 || i == 0x80-1 {
					if err := t.sendBootloaderDataCommand(ctx, data, 8); err != nil {
						return fmt.Errorf("!!! FLASHing Failed !!! after: 0x%X bytes: %v", bytesRead, err)

					}
				}
			}
		}
		bytesRead += 0x80
		bar.Set(int(bytesRead))
	}
	bar.Finish()
	fmt.Printf(" took: %s\n", time.Since(startTime).Round(time.Millisecond).String())
	return nil
}
