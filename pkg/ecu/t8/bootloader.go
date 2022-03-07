package t8

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) StartBootloader(ctx context.Context, startAddress uint32) error {
	// 06368000102400
	payload := []byte{
		0x06, 0x36, 0x80,
		byte(startAddress >> 24),
		byte(startAddress >> 16),
		byte(startAddress >> 8),
		byte(startAddress),
	}

	resp, err := t.c.SendAndPoll(ctx, gocan.NewFrame(0x7E0, payload, gocan.ResponseRequired), t.defaultTimeout, 0x7E8)
	if err != nil {
		return err
	}
	if err := gmlan.CheckErr(resp); err != nil {
		return err
	}
	return nil
}

func (t *Client) UploadBootloader(ctx context.Context, callback model.ProgressCallback) error {
	gm := gmlan.New(t.c)

	time.Sleep(50 * time.Millisecond)

	if err := gm.RequestDownload(ctx, 0x7E0, 0x7E8, false); err != nil {
		return err
	}
	startAddress := 0x102400
	Len := 9996 / 238
	seq := byte(0x21)

	start := time.Now()

	if callback != nil {
		callback(-float64(Len))
		callback(float64(0))
		callback("Uploading bootloader")
	}

	r := bytes.NewReader(LegionBytes)
	for i := 0; i < Len; i++ {
		if callback != nil {
			callback(float64(i + 1))
		}
		if err := gm.DataTransfer(ctx, 0xF0, startAddress, 0x7E0, 0x7E8); err != nil {
			return err
		}
		seq = 0x21
		for j := 0; j < 0x22; j++ {
			payload := make([]byte, 8)
			payload[0] = seq
			for x := 1; x < 8; x++ {
				b, err := r.ReadByte()
				if err != nil && err != io.EOF {
					return err
				}
				payload[x] = b
			}

			tt := gocan.CANFrameType{Type: 1, Responses: 0}
			if j == 0x21 {
				tt.Responses = 1
			}

			f := gocan.NewFrame(0x7E0, payload, tt)
			if err := t.c.Send(f); err != nil {
				return err
			}

			seq++
			if seq > 0x2F {
				seq = 0x20
			}
		}
		resp, err := t.c.Poll(ctx, t.defaultTimeout, 0x7E8)
		if err != nil {
			return err
		}
		if err := gmlan.CheckErr(resp); err != nil {
			log.Println(resp.String())
			return err
		}
		d := resp.Data()
		if d[0] != 0x01 || d[1] != 0x76 {
			return errors.New("invalid transfer data response")
		}
		gm.TesterPresentNoResponseAllowed()
		startAddress += 0xEA
	}

	seq = 0x21

	if err := gm.DataTransfer(ctx, 0x0A, startAddress, 0x7E0, 0x7E8); err != nil {
		return err
	}

	payload := make([]byte, 8)
	payload[0] = seq
	for x := 1; x < 8; x++ {
		b, err := r.ReadByte()
		if err != nil && err != io.EOF {
			return err
		}
		payload[x] = b
	}
	f2 := gocan.NewFrame(0x7E0, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, f2, t.defaultTimeout, 0x7E8)
	if err != nil {
		return err
	}

	if err := gmlan.CheckErr(resp); err != nil {
		return err
	}

	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x76 {
		return errors.New("invalid transfer data response")
	}
	gm.TesterPresentNoResponseAllowed()

	startAddress += 0x06

	log.Println("Done, took: " + time.Since(start).String())
	return nil
}

func (t *Client) LegionPing(ctx context.Context) error {
	frame := gocan.NewFrame(0x7E0, []byte{0xEF, 0xBE, 0x00, 0x00, 0x00, 0x00, 0x33, 0x66}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, 0x7E8)
	if err != nil {
		return errors.New("LegionPing: " + err.Error())
	}
	if err := gmlan.CheckErr(resp); err != nil {
		return errors.New("LegionPing: " + err.Error())
	}
	d := resp.Data()
	if d[0] == 0xDE && d[1] == 0xAD && d[2] == 0xF0 && d[3] == 0x0F {
		return nil
	}
	return errors.New("LegionPing: no response")
}

func (t *Client) LegionExit(ctx context.Context) error {
	payload := []byte{0x01, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	frame := gocan.NewFrame(0x7E0, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, 0x7E8)
	if err != nil {
		return errors.New("LegionExit: " + err.Error())
	}
	if err := gmlan.CheckErr(resp); err != nil {
		return errors.New("LegionExit: " + err.Error())
	}
	d := resp.Data()
	if d[0] != 0x01 || (d[1] != 0x50 && d[1] != 0x60) {
		return errors.New("LegionExit: invalid response")
	}
	return nil
}
