package t7

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

func (t *Client) Erase(ctx context.Context) error {
	ok, err := t.KnockKnock(ctx)
	if err != nil || !ok {
		return fmt.Errorf("failed to autenticate: %v", err)
	}

	data := make([]byte, 8)
	eraseMsg := []byte{0x40, 0xA1, 0x02, 0x31, 0x52, 0x00, 0x00, 0x00}
	eraseMsg2 := []byte{0x40, 0xA1, 0x02, 0x31, 0x53, 0x00, 0x00, 0x00}
	confirmMsg := []byte{0x40, 0xA1, 0x01, 0x3E, 0x00, 0x00, 0x00, 0x00}

	bar := progressbar.NewOptions(25,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(false),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[cyan][1/2][reset] erasing ECU "),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	// Send "Erase message 1" to Trionic
	data[3] = 0
	i := 0
	for data[3] != 0x71 && i < 10 {
		t.c.SendFrame(0x240, eraseMsg)
		f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
		if err != nil {
			log.Println(err)
		} else {
			data = f.Data()
			t.Ack(data[0])
		}
		time.Sleep(100 * time.Millisecond)
		i++
		bar.Add(1)
	}
	if i > 10 {
		return errors.New("to many tries to erase 1")
	}

	// Send "Erase message 2" to Trionic
	data[3] = 0
	i = 0
	for data[3] != 0x71 && i < 200 {
		t.c.SendFrame(0x240, eraseMsg2)
		f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
		if err != nil {
			log.Println(err)
		} else {
			data = f.Data()
			t.Ack(data[0])
		}
		time.Sleep(100 * time.Millisecond)
		i++
		bar.Add(1)
	}
	// Check to see if erase operation lasted longer than 20 sec...
	if i > 200 {
		return errors.New("to many tries to erase 2")
	}

	t.c.SendFrame(0x240, confirmMsg)
	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		bar.Finish()
		log.Println()
		return err
	}
	d := f.Data()
	if d[3] == 0x7E {
		time.Sleep(100 * time.Millisecond)
		t.c.SendFrame(0x240, confirmMsg)
		f2, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
		if err != nil {
			log.Println(err)
		}
		d2 := f2.Data()
		if d2[3] == 0x7E {
			bar.Finish()
			fmt.Println()
			return nil
		}
	} else {
		bar.Finish()
		fmt.Println()
		return errors.New("erase failed")
	}
	return fmt.Errorf("unknown erase error %X", d)
}
