package t5

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) ResetECU(ctx context.Context, callback model.ProgressCallback) error {
	//if !t.bootloaded {
	//	if err := t.UploadBootLoader(ctx); err != nil {
	//		return err
	//	}
	//}
	//log.Println("Resetting ECU")
	frame := gocan.NewFrame(0x5, []byte{0xC2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0xC)
	if err != nil {
		return fmt.Errorf("failed to reset ECU: %v", err)
	}
	data := resp.Data()
	if data[0] != 0xC2 || data[1] != 0x00 || data[2] != 0x08 {
		return errors.New("invalid response to reset ECU")
	}
	if callback != nil {
		callback("ECU has been reset")
	}
	return nil
}
