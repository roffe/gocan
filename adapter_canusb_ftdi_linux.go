//go:build ftdi

package gocan

import (
	"context"
	"fmt"
	"time"
)

func (cu *CanusbFTDI) recvManager(ctx context.Context, parseFn func([]byte)) {
	var rx_cnt int32
	var err error
	buf := make([]byte, 4*1024) // large enough for worst case
	for ctx.Err() == nil {
		rx_cnt, err = cu.port.GetQueueStatus()
		if err != nil {
			if !cu.closed {
				cu.setError(fmt.Errorf("failed to get queue status: %w", err))
			}
			return
		}
		if rx_cnt == 0 {
			time.Sleep(300 * time.Microsecond)
			continue
		}
		// Adjust slice length without reallocation
		readBuffer := buf[:rx_cnt]
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			if !cu.closed {
				cu.setError(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if n == 0 {
			continue
		}
		parseFn(readBuffer[:n])

	}
}
