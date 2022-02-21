package t7

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/avast/retry-go"
	gocan "github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

const (
	IBusRate = 47.619
	PBusRate = 500
)

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
}

func New(c *gocan.Client) *Client {
	t := &Client{
		c:              c,
		defaultTimeout: 250 * time.Millisecond,
	}
	return t
}

// 266h Send acknowledgement, has 0x3F on 3rd!
func (t *Client) Ack(val byte, opts ...model.FrameOpt) {
	ack := []byte{0x40, 0xA1, 0x3F, val & 0xBF, 0x00, 0x00, 0x00, 0x00}
	t.c.SendFrame(0x266, ack, opts...)
}

var T7Headers = []model.Header{
	{Desc: "VIN:", ID: 0x90},
	{Desc: "Box HW part number:", ID: 0x91},
	{Desc: "Immo Code:", ID: 0x92},
	{Desc: "Software Saab part number:", ID: 0x94},
	{Desc: "ECU Software version:", ID: 0x95},
	{Desc: "Engine type:", ID: 0x97},
	{Desc: "Tester info:", ID: 0x98},
	{Desc: "Software date:", ID: 0x99},
}

// Print out some Trionic7 info
func (t *Client) Info(ctx context.Context) ([]model.HeaderResult, error) {
	if err := t.DataInitialization(ctx); err != nil {
		return nil, err
	}
	var out []model.HeaderResult
	for _, d := range T7Headers {
		h, err := t.GetHeader(ctx, byte(d.ID))
		if err != nil {
			return nil, fmt.Errorf("info failed: %v", err)
		}
		a := model.HeaderResult{Value: h}
		a.Desc = d.Desc
		a.ID = d.ID
		out = append(out, a)
	}
	time.Sleep(30 * time.Millisecond)
	return out, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	res, err := t.Info(ctx)
	if err != nil {
		return err
	}
	log.Println("----- ECU info ---------------")
	for _, r := range res {
		log.Println(r.Desc, r.Value)
	}
	log.Println("------------------------------")
	return nil
}

var lastDataInitialization time.Time

func (t *Client) DataInitialization(ctx context.Context) error {
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
		retry.Attempts(3),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("#%d: %s\n", n, err.Error())
		}),
		retry.Delay(100*time.Millisecond),
	)
	if err != nil {
		return errors.New("trionic data initialization failed")
	}
	return nil
}

func (t *Client) GetHeader(ctx context.Context, id byte) (string, error) {
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
		d := f.Data()
		if d[0]&0x40 == 0x40 {
			if int(d[2]) > 2 {
				length = int(d[2]) - 2
			}
			for b := 5; b < 8; b++ {
				if length > 0 {
					answer = append(answer, d[b])
				}
				length--
			}
		} else {
			for c := 0; c < 6; c++ {
				if length == 0 {
					break
				}
				answer = append(answer, d[2+c])
				length--
			}
		}

		if d[0] == 0x80 || d[0] == 0xC0 {
			t.Ack(d[0], model.OptFrameType(model.Outgoing))
			break
		} else {
			t.Ack(d[0])
		}
	}

	return string(answer), nil
}

func (t *Client) KnockKnock(ctx context.Context) (bool, error) {
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
		time.Sleep(200 * time.Millisecond)
	}
	log.Println("/!\\ authentication failure 😞👎🏻")
	return false, nil
}

func (t *Client) letMeIn(ctx context.Context, method int) (bool, error) {
	msg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	msgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}

	t.c.SendFrame(0x240, msg)

	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		return false, err

	}
	d := f.Data()
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
	d2 := f2.Data()
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

func (t *Client) LetMeTry(ctx context.Context, key1, key2 int) bool {
	msg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	msgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}

	t.c.SendFrame(0x240, msg)

	f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
	if err != nil {
		log.Println(err)
		return false

	}
	d := f.Data()
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
	d2 := f2.Data()
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
