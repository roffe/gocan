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
)

func (t *Client) UploadBootloader(ctx context.Context) error {
	gm := gmlan.New(t.c)

	t.SendKeepAlive(ctx)

	if err := t.ShutUp(ctx); err != nil {
		return err
	}

	if err := gm.InitiateDiagnosticOperation(ctx, 0x02, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.ReportProgrammedState(ctx, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.ProgrammingMode(ctx, 0x01, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.ProgrammingMode(ctx, 0x03, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := t.RequestSecurityAccess(ctx, AccessLevel01); err != nil {
		return err
	}

	if err := gm.RequestDownload(ctx, 0x7E0, 0x7E8, false); err != nil {
		return err
	}

	log.Println("uploading bootloader")
	startAddress := 0x102400
	Len := 9996 / 238
	seq := byte(0x21)

	r := bytes.NewReader(LegionBytes)
	for i := 0; i < Len; i++ {
		if err := gm.TransferData(ctx, 0xF0, startAddress, 0x7E0, 0x7E8); err != nil {
			return err
		}

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

			tt := gocan.Outgoing
			if j == 0x21 {
				tt = gocan.ResponseRequired // we want a response on the last one
			}

			f := gocan.NewFrame(0x7E0, payload, tt)
			if err := t.c.Send(f); err != nil {
				return err
			}
			log.Println(f.String())

			seq++
			if seq > 0x2F {
				seq = 0x20
			}
		}
		resp, err := t.c.Poll(ctx, 150*time.Millisecond, 0x7E8)
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
		t.SendKeepAlive(ctx)
		startAddress += 0xEA
	}

	seq = 0x21

	if err := gm.TransferData(ctx, 0x0A, startAddress, 0x7E0, 0x7E8); err != nil {
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
	resp, err := t.c.SendAndPoll(ctx, f2, 150*time.Millisecond, 0x7E8)
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
	t.SendKeepAlive(ctx)

	startAddress += 0x06

	return nil
}

func (t *Client) ShutUp(ctx context.Context) error {
	frame := gocan.NewFrame(0x7E0, []byte{0x01, 0x28}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0x7E8)
	if err != nil {
		return errors.New("Shutup: " + err.Error())
	}
	if err := gmlan.CheckErr(resp); err != nil {
		return errors.New("Shutup: " + err.Error())
	}
	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x68 {
		return errors.New("/!\\ Invalid response to ShutUp")
	}

	return nil
}
