package t7

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) EraseECU(ctx context.Context, callback model.ProgressCallback) error {
	data := make([]byte, 8)
	eraseMsg := []byte{0x40, 0xA1, 0x02, 0x31, 0x52, 0x00, 0x00, 0x00}
	confirmMsg := []byte{0x40, 0xA1, 0x01, 0x3E, 0x00, 0x00, 0x00, 0x00}

	if callback != nil {
		callback(-float64(17))
		callback("Erasing FLASH")
	}

	progress := 0

	// Send "Erase message 1" to Trionic
	data[3] = 0
	i := 0
	for data[3] != 0x71 && i < 30 {
		f, err := t.c.SendAndPoll(ctx, gocan.NewFrame(0x240, eraseMsg, gocan.ResponseRequired), t.defaultTimeout, 0x258)
		if err != nil {
			return err
		}
		data = f.Data()
		t.Ack(data[0], gocan.Outgoing)
		i++
		progress++
		if callback != nil {
			callback(float64(progress))
		}
		time.Sleep(250 * time.Millisecond)
	}
	if i > 10 {
		return errors.New("to many tries to erase 1")
	}

	// Send "Erase message 2" to Trionic
	data[3] = 0
	i = 0
	eraseMsg[4] = 0x53
	for data[3] != 0x71 && i < 200 {
		f, err := t.c.SendAndPoll(ctx, gocan.NewFrame(0x240, eraseMsg, gocan.ResponseRequired), t.defaultTimeout, 0x258)
		if err != nil {
			return err
		}
		data = f.Data()
		t.Ack(data[0], gocan.Outgoing)
		i++
		progress++
		if callback != nil {
			callback(float64(progress))
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Check to see if erase operation lasted longer than 20 sec...
	if i > 200 {
		return errors.New("to many tries to erase 2")
	}

	data[3] = 0
	i = 0
	for data[3] != 0x7E && i < 10 {
		time.Sleep(250 * time.Millisecond)
		f, err := t.c.SendAndPoll(ctx, gocan.NewFrame(0x240, confirmMsg, gocan.ResponseRequired), t.defaultTimeout, 0x258)
		if err != nil {
			return err
		}
		data = f.Data()
		i++
		progress++
		if callback != nil {
			callback(float64(progress))
		}
	}
	if i < 10 {
		callback("Erase done")
		return nil
	}

	return fmt.Errorf("unknown erase error")
}
