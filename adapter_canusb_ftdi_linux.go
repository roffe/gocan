//go:build ftdi

package gocan

import (
	"context"
	"fmt"
	"time"
)

func (cu *CanusbFTDI) recvManager(ctx context.Context, parseFn func([]byte)) {
	port := cu.port             // capture once; Close() may set cu.port = nil concurrently
	buf := make([]byte, 4*1024) // large enough for worst case
	for ctx.Err() == nil {
		n, err := port.Read(buf)
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
