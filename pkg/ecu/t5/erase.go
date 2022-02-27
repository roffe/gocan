package t5

import (
	"context"
	"fmt"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) EraseECU(ctx context.Context, callback model.ProgressCallback) error {
	startTime := time.Now()
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx, callback); err != nil {
			return err
		}
	}
	if callback != nil {
		callback(-float64(100))
		callback("Erasing FLASH...")
	}
	cmd := []byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	frame := gocan.NewFrame(0x005, cmd, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 20*time.Second, 0xC)
	if err != nil {
		return err
	}
	data := resp.Data()
	if data[0] == 0xC0 && data[1] == 0x00 {
		if callback != nil {
			callback(fmt.Sprintf("FLASH erased, took: %s\n", time.Since(startTime).Round(time.Millisecond).String()))
			callback(float64(100))
		}
		return nil
	}

	return fmt.Errorf("erase FAILED: %X", data)
}
