package t7

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/canusb"
	"github.com/schollz/progressbar/v3"
)

const (
	IBusRate = 47.619
	PBusRate = 500
)

type Trionic struct {
	c *canusb.Canusb
}

func New(c *canusb.Canusb) *Trionic {
	t := &Trionic{
		c: c,
	}
	return t
}

func DecodeSaabFrame(f *canusb.Frame) {
	//https://pikkupossu.1g.fi/tomi/projects/p-bus/p-bus.html
	var prefix string
	var signfBit bool
	switch f.Identifier {
	case 0x238: // Trionic data initialization reply
		prefix = "TDI"
	case 0x240: //  Trionic data query
		prefix = "TDIR"
	case 0x258: // Trionic data query reply
		prefix = "TDQR"
	case 0x266: // Trionic reply acknowledgement
		prefix = "TRA"
	case 0x370: // Mileage
		prefix = "MLG"
	case 0x3A0: // Vehicle speed (MIU?)
		prefix = "MIU"
	case 0x1A0: // Engine information
		signfBit = true
		prefix = "ENG"
	default:
		prefix = "UNK"
	}
	if signfBit {
		log.Printf("%s> 0x%x  %d  %08b  %X\n", prefix, f.Identifier, f.Len, f.Data[:1], f.Data[1:])
		return
	}
	log.Printf("in> %s> 0x%x  %d %X\n", prefix, f.Identifier, f.Len, f.Data)
}

// Print out some Trionic7 info
func (t *Trionic) Info(ctx context.Context) error {
	if err := t.DataInitialization(ctx); err != nil {
		return err
	}
	data := []struct {
		name string
		id   uint16
	}{
		{"VIN:", 0x90},
		{"Box HW part number:", 0x91},
		{"Immo Code:", 0x92},
		{"Software Saab part number:", 0x94},
		{"ECU Software version:", 0x95},
		{"Engine type:", 0x97},
		{"Tester info:", 0x98},
		{"Software date:", 0x99},
	}

	for _, d := range data {
		h, err := t.GetHeader(ctx, byte(d.id))
		if err != nil {
			return fmt.Errorf("info failed: %v", err)
		}
		log.Println(d.name, h)
	}
	return nil
}

func (t *Trionic) DataInitialization(ctx context.Context) error {
	err := retry.Do(
		func() error {
			t.c.SendFrame(0x220, canusb.B{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00}) //init:msg
			_, err := t.c.Poll(ctx, 0x238, 100*time.Millisecond)
			if err != nil {
				return fmt.Errorf("%v", err)
			}
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(5),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("#%d: %s\n", n, err.Error())
		}),
	)
	if err != nil {
		return fmt.Errorf("trionic data initialization failed: %v", err)
	}
	return nil
}

func (t *Trionic) GetHeader(ctx context.Context, id byte) (string, error) {
	err := retry.Do(
		func() error {
			return t.c.SendFrame(0x240, canusb.B{0x40, 0xA1, 0x02, 0x1A, id, 0x00, 0x00, 0x00})
		},
		retry.Context(ctx),
		retry.Attempts(3),
	)
	if err != nil {
		return "", fmt.Errorf("failed getting header: %v", err)
	}

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("failed getting header: %v", err)
	default:
	}

	var answer []byte
	var length int
	for i := 0; i < 10; i++ {
		f, err := t.c.Poll(ctx, 0x258, 100*time.Millisecond)
		if err != nil {
			log.Println(err)
			continue
		}
		if f.Data[0]&0x40 == 0x40 {
			if int(f.Data[2]) > 2 {
				length = int(f.Data[2]) - 2
			}
			for i := 5; i < 8; i++ {
				if length > 0 {
					answer = append(answer, f.Data[i])
				}
				length--
			}
		} else {
			for i := 0; i < 6; i++ {
				if length == 0 {
					break
				}
				answer = append(answer, f.Data[2+i])
				length--
			}
		}
		t.c.SendFrame(0x266, canusb.B{0x40, 0xA1, 0x3F, f.Data[0] & 0xBF, 0x00, 0x00, 0x00, 0x00})
		if bytes.Equal(f.Data[:1], canusb.B{0x80}) || bytes.Equal(f.Data[:1], canusb.B{0xC0}) {
			break
		}
	}

	return string(answer), nil
}

func (t *Trionic) KnockKnock(ctx context.Context) bool {
	for i := 0; i < 2; i++ {
		if letMeIn(ctx, t.c, i) {
			log.Println("trusted 🥳🎉")
			return true
		}
	}
	log.Println("/!\\ untrusted 😞👎🏻")
	return false
}

func letMeIn(ctx context.Context, c *canusb.Canusb, method int) bool {
	msg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	msgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}
	ack := []byte{0x40, 0xA1, 0x3F, 0x00, 0x00, 0x00, 0x00, 0x00}

	c.SendFrame(0x240, msg)

	f, err := c.Poll(ctx, 0x258, 100*time.Millisecond)
	if err != nil {
		log.Println(err)
		return false

	}
	ack[3] = f.Data[0] & 0xBF
	c.SendFrame(0x266, ack)

	seed := int(f.Data[5])<<8 | int(f.Data[6])
	key := calcen(seed, method)

	msgReply[5] = byte(int(key) >> 8 & int(0xFF))
	msgReply[6] = byte(key) & 0xFF

	c.SendFrame(0x240, msgReply)
	f2, err := c.Poll(ctx, 0x258, 100*time.Millisecond)
	if err != nil {
		log.Println(err)
		return false

	}
	ack[3] = f2.Data[0] & 0xBF
	c.SendFrame(0x266, ack)
	if f2.Data[3] == 0x67 && f2.Data[5] == 0x34 {
		return true
	} else {
		return false
	}
}

func calcen(seed int, method int) int {
	key := seed << 2
	key &= 0xFFFF
	switch method {
	case 0:
		key ^= 0x8142
		key -= 0x2356
	case 1:
		key ^= 0x4081
		key -= 0x1F6F
	}
	key &= 0xFFFF
	return key
}

func (t *Trionic) Erase(ctx context.Context) error {
	if !t.KnockKnock(ctx) {
		log.Fatal("failed to autenticate")
	}
	return nil
}

func (t *Trionic) ReadBin(ctx context.Context, filename string) error {
	//if err := t.DataInitialization(ctx); err != nil {
	//	return err
	//}
	if !t.KnockKnock(ctx) {
		log.Fatal("failed to autenticate")
	}
	//bin := make([]byte, 512*1024)
	b, err := t.readTrionic(ctx, 0x00, 0x80000)
	if err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
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
	//int address, length, rcv_len, dot, bytes_this_round, retries, ret;
	//initMsg := []byte{0x20, 0x81, 0x00, 0x11, 0x02, 0x42, 0x00, 0x00}
	endDataMsg := []byte{0x40, 0xA1, 0x01, 0x82, 0x00, 0x00, 0x00, 0x00}
	jumpMsg1a := []byte{0x41, 0xA1, 0x08, 0x2C, 0xF0, 0x03, 0x00, 0xEF} // 0x000000 length=0xEF
	jumpMsg1b := []byte{0x00, 0xA1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	//postJumpMsg := []byte{0x40, 0xA1, 0x01, 0x3E, 0x00, 0x00, 0x00, 0x00}
	dataMsg := []byte{0x40, 0xA1, 0x02, 0x21, 0xF0, 0x00, 0x00, 0x00}
	ack := []byte{0x40, 0xA1, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00}

	rcvlen := 0
	log.Println("start download")
	address := addr
	bar := progressbar.DefaultBytes(int64(leng), "downloading flash")
	retries := 0
outer:
	for rcvlen < leng {
		bytesThisRound := 0
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		//fmt.Printf("%d / %d [%d]\r", rcvlen, leng, binPos)

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

		f, err := t.c.Poll(ctx, 0x258, 100*time.Millisecond)
		if err != nil {
			return nil, err
		}
		ack[3] = f.Data[0] & 0xBF
		t.c.SendFrame(0x266, ack)
		t.c.SendFrame(0x240, dataMsg)
		var length int
		for {
			//log.Println("more", rcvlen, leng)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			f2, err := t.c.Poll(ctx, 0x258, 500*time.Millisecond)
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
				//log.Println("else")
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
	t.c.SendFrame(0x240, endDataMsg)
	f3, err := t.c.Poll(ctx, 0x258, 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	ack[3] = f3.Data[0] & 0xBF
	t.c.SendFrame(0x266, ack)
	log.Println("download done")
	return bin, nil
}
