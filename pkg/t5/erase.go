package t5

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) EraseECU(ctx context.Context) error {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return err
		}
	}
	log.Println("Erasing FLASH...")
	cmd := []byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	frame := model.NewFrame(0x005, cmd, model.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 20*time.Second, 0xC)
	if err != nil {
		return err
	}
	data := resp.Data()
	if data[0] == 0xC0 && data[1] == 0x00 {
		log.Println("FLASH erased...")
		return nil
	}

	return fmt.Errorf("erase FAILED: %X", data)
}
