//go:build ftdi

package gocan

import (
	"context"
	"fmt"
	"time"
)

func (cu *CanusbFTDI) recvManager(ctx context.Context, parseFn func([]byte)) {
	buf := make([]byte, 4*1024) // large enough for worst case
	for ctx.Err() == nil {
		n, err := cu.port.Read(buf)
		if err != nil {
			if !cu.closed {
				cu.Error(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if n == 0 {
			time.Sleep(200 * time.Microsecond)
			continue
		}
		parseFn(buf[:n])

	}
}
