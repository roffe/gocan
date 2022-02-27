package t5

import (
	"context"
	"fmt"
	"time"

	"github.com/roffe/gocan/pkg/bar"
)

func (t *Client) GetSRAMSnapshot(ctx context.Context) ([]byte, error) {
	sram, err := t.readRAM(ctx, 0, 0x8000)
	if err != nil {
		return nil, err
	}
	return sram, nil
}

func (t *Client) readRAM(ctx context.Context, address, length uint32) ([]byte, error) {
	bar := bar.New(int(length+0x9), "downloading sram")
	out := make([]byte, length)
	address += 5
	num := length / 6
	if (length % 6) > 0 {
		num++
	}
	startTime := time.Now()
	for i := uint32(0); i < num; i++ {
		buff, err := t.ReadMemoryByAddress(ctx, address)
		if err != nil {
			return nil, err
		}
		for j := uint32(0); j < 6; j++ {
			if (i*6)+j < length {
				out[(i*6)+j] = buff[j]
			}
		}
		address += 6
		bar.Set(int(address))
	}

	bar.Finish()
	fmt.Printf(" took: %s\n", time.Since(startTime).Round(time.Millisecond).String())
	return out, nil
}
