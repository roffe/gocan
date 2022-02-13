package t7

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/avast/retry-go"
	gocan "github.com/roffe/gocan"
)

const (
	IBusRate = 47.619
	PBusRate = 500
)

type Trionic struct {
	c              *gocan.Client
	defaultTimeout time.Duration
}

func New(c *gocan.Client) *Trionic {
	t := &Trionic{
		c:              c,
		defaultTimeout: 250 * time.Millisecond,
	}
	return t
}

// 266h Send acknowledgement, has 0x3F on 3rd!
func (t *Trionic) Ack(val byte) {
	ack := []byte{0x40, 0xA1, 0x3F, val & 0xBF, 0x00, 0x00, 0x00, 0x00}
	t.c.SendFrame(0x266, ack)
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

var lastDataInitialization time.Time

func (t *Trionic) DataInitialization(ctx context.Context) error {
	if !lastDataInitialization.IsZero() {
		if time.Since(lastDataInitialization) < 10*time.Second {
			return nil
		}
	}
	lastDataInitialization = time.Now()

	err := retry.Do(
		func() error {
			t.c.SendFrame(0x220, []byte{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00}) //init:msg
			_, err := t.c.Poll(ctx, t.defaultTimeout, 0x238)
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
		retry.Delay(200*time.Millisecond),
	)
	if err != nil {
		return errors.New("Trionic data initialization failed")
	}
	return nil
}

func (t *Trionic) GetHeader(ctx context.Context, id byte) (string, error) {
	err := retry.Do(
		func() error {
			return t.c.SendFrame(0x240, []byte{0x40, 0xA1, 0x02, 0x1A, id, 0x00, 0x00, 0x00})
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
		f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
		if err != nil {
			log.Println(err)
			continue
		}
		d := f.GetData()
		if d[0]&0x40 == 0x40 {
			if int(d[2]) > 2 {
				length = int(d[2]) - 2
			}
			for i := 5; i < 8; i++ {
				if length > 0 {
					answer = append(answer, d[i])
				}
				length--
			}
		} else {
			for i := 0; i < 6; i++ {
				if length == 0 {
					break
				}
				answer = append(answer, d[2+i])
				length--
			}
		}
		t.c.SendFrame(0x266, []byte{0x40, 0xA1, 0x3F, d[0] & 0xBF, 0x00, 0x00, 0x00, 0x00})
		if bytes.Equal(d[:1], []byte{0x80}) || bytes.Equal(d[:1], []byte{0xC0}) {
			break
		}
	}

	return string(answer), nil
}

func (t *Trionic) KnockKnock(ctx context.Context) (bool, error) {
	if err := t.DataInitialization(ctx); err != nil {
		return false, err
	}
	for i := 0; i < 3; i++ {
		ok, err := t.letMeIn(ctx, i)
		if err != nil {
			fmt.Printf("failed to auth %d: %v\n", i, err)
			continue
		}
		if ok {
			log.Printf("authentication successfull with method %d 🥳🎉", i)
			return true, nil
		}
	}
	log.Println("/!\\ authentication failure 😞👎🏻")
	return false, nil
}

func (t *Trionic) letMeIn(ctx context.Context, method int) (bool, error) {
	msg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	msgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}

	t.c.SendFrame(0x240, msg)

	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return false, err

	}
	d := f.GetData()
	t.Ack(d[0])

	s := int(d[5])<<8 | int(d[6])
	k := calcen(s, method)

	msgReply[5] = byte(int(k) >> 8 & int(0xFF))
	msgReply[6] = byte(k) & 0xFF

	t.c.SendFrame(0x240, msgReply)
	f2, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return false, err

	}
	d2 := f2.GetData()
	t.Ack(d2[0])
	if d2[3] == 0x67 && d2[5] == 0x34 {
		return true, nil
	} else {
		return false, errors.New("invalid response")
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
	case 2:
		key ^= 0x3DC
		key -= 0x2356
	}
	key &= 0xFFFF
	return key
}

func (t *Trionic) LetMeTry(ctx context.Context, key1, key2 int) bool {
	msg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	msgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}

	t.c.SendFrame(0x240, msg)

	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		log.Println(err)
		return false

	}
	d := f.GetData()
	t.Ack(d[0])

	s := int(d[5])<<8 | int(d[6])
	k := calcenCustom(s, key1, key2)

	msgReply[5] = byte(int(k) >> 8 & int(0xFF))
	msgReply[6] = byte(k) & 0xFF

	t.c.SendFrame(0x240, msgReply)
	f2, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		log.Println(err)
		return false

	}
	d2 := f2.GetData()
	t.Ack(d2[0])
	if d2[3] == 0x67 && d2[5] == 0x34 {
		return true
	} else {
		return false
	}
}

func calcenCustom(seed int, key1, key2 int) int {
	key := seed << 2
	key &= 0xFFFF
	key ^= key1
	key -= key2
	key &= 0xFFFF
	return key
}
