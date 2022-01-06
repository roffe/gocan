package t7

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

func (t *Trionic) ReadBin(ctx context.Context, filename string) error {
	ok, err := t.KnockKnock(ctx)
	if err != nil || !ok {
		return fmt.Errorf("failed to authenticate: %v", err)
	}
	b, err := t.readTrionic(ctx, 0x00, 0x80000)
	if err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return err
	}
	return nil
}

func (t *Trionic) readTrionic(ctx context.Context, addr, leng int) ([]byte, error) {
	bin := make([]byte, leng)
	var binPos int

	jumpMsg1a := []byte{0x41, 0xA1, 0x08, 0x2C, 0xF0, 0x03, 0x00, 0xEF} // 0x000000 length=0xEF
	jumpMsg1b := []byte{0x00, 0xA1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	dataMsg := []byte{0x40, 0xA1, 0x02, 0x21, 0xF0, 0x00, 0x00, 0x00}
	ack := []byte{0x40, 0xA1, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00}

	rcvlen := 0
	address := addr
	start := time.Now()
	log.Println("starting bin download")
	bar := progressbar.DefaultBytes(int64(leng), "progress:")
	retries := 0
outer:
	for rcvlen < leng {
		if retries > 15 {
			return nil, fmt.Errorf("to many retries downloading bin")
		}
		bytesThisRound := 0
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if (leng - rcvlen) < 0xEF {
			jumpMsg1a[7] = byte(leng - rcvlen)
		} else {
			jumpMsg1a[7] = 0xEF
		}
		t.c.SendFrame(0x240, jumpMsg1a)

		jumpMsg1b[2] = byte((address >> 16) & 0xFF)
		jumpMsg1b[3] = byte((address >> 8) & 0xFF)
		jumpMsg1b[4] = byte(address & 0xFF)
		t.c.SendFrame(0x240, jumpMsg1b)

		f, err := t.c.Poll(ctx, 0x258, t.defaultTimeout)
		if err != nil {
			log.Println(err)
			retries++
			continue outer
		}
		ack[3] = f.Data[0] & 0xBF
		t.c.SendFrame(0x266, ack)

		t.c.SendFrame(0x240, dataMsg)
		var length int
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			f2, err := t.c.Poll(ctx, 0x258, t.defaultTimeout)
			if err != nil {
				if retries > 10 {
					return nil, fmt.Errorf("to many retries downloading bin")
				}
				log.Println(err)
				rcvlen -= bytesThisRound
				binPos -= bytesThisRound
				length += bytesThisRound
				retries++
				continue outer
			}
			if f2.Data[0]&0x40 == 0x40 {
				length = int(f2.Data[2]) - 2 // subtract two non-payload bytes
				if length > 0 && rcvlen < leng {
					bin[binPos] = f2.Data[5]
					binPos++
					rcvlen++
					bytesThisRound++
					length--
				}
				if length > 0 && rcvlen < leng {
					bin[binPos] = f2.Data[6]
					binPos++
					rcvlen++
					bytesThisRound++
					length--
				}
				if length > 0 && rcvlen < leng {
					bin[binPos] = f2.Data[7]
					binPos++
					rcvlen++
					bytesThisRound++
					length--
				}
			} else {
				for i := 0; i < 6; i++ {
					if rcvlen < leng {
						bin[binPos] = f2.Data[2+i]
						binPos++
						rcvlen++
						bytesThisRound++
						length--
						if length == 0 {
							break
						}
					}
				}
			}
			ack[3] = f2.Data[0] & 0xBF
			t.c.SendFrame(0x266, ack)
			if f2.Data[0] == 0x80 || f2.Data[0] == 0xC0 {
				break
			}
		}
		bar.Add(bytesThisRound)
		address = addr + rcvlen
	}
	bar.Close()
	t.c.SendFrame(0x240, []byte{0x40, 0xA1, 0x01, 0x82, 0x00, 0x00, 0x00, 0x00})
	f3, err := t.c.Poll(ctx, 0x258, t.defaultTimeout)
	if err != nil {
		return nil, err
	}
	ack[3] = f3.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)
	log.Println("download done, took:", time.Since(start).String())
	return bin, nil
}
