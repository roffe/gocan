//go:build ftdi

package gocan

import (
	"context"
	"fmt"
	"syscall"

	ftdi "github.com/roffe/gocan/pkg/ftdi"
	"github.com/roffe/gocan/pkg/w32"
)

func (cu *CanusbFTDI) recvManager(ctx context.Context, parseFn func([]byte)) {
	var rx_cnt int32
	var err error
	buf := make([]byte, 4*1024) // large enough for worst case

	// Create a Windows event
	hEvent, err := w32.CreateEvent(false, false, "canusbFTDIRecvEvent")
	if err != nil {
		cu.setError(fmt.Errorf("CreateEvent failed: %w", err))
	}

	if err := cu.port.SetEventNotification(ftdi.FT_EVENT_RXCHAR, hEvent); err != nil {
		cu.setError(fmt.Errorf("SetEventNotification failed: %w", err))
	}
	defer w32.CloseHandle(hEvent)

	for ctx.Err() == nil {
		_, err := w32.WaitForSingleObject(hEvent, 10)
		if err != nil {
			if err == syscall.ETIMEDOUT {
				continue
			}
			cu.setError(fmt.Errorf("failed to wait for event: %w", err))
			return
		}

		rx_cnt, err = cu.port.GetQueueStatus()
		if err != nil {
			if !cu.closed {
				cu.setError(fmt.Errorf("failed to get queue status: %w", err))
			}
			return
		}
		if rx_cnt == 0 {
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
