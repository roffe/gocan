package t8

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
)

const (
	AccessLevel01 = byte(0x01)
	AccessLevelFB = byte(0xFB)
	AccessLevelFD = byte(0xFD)
)

func (t *Client) RequestSecurityAccess(ctx context.Context, accesslevel byte) error {
	payload := []byte{0x02, 0x27, byte(accesslevel)}
	f := gocan.NewFrame(0x7E0, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, f, t.defaultTimeout, 0x7E8)
	if err != nil {
		log.Println(err)
		return err
	}
	d := resp.Data()

	if err := gmlan.CheckErr(resp); err != nil {
		return err
	}

	if d[1] != 0x67 || (d[2] != 0xFB && d[2] != 0xFD && d[2] != 0x01) {
		log.Println(resp.String())
		return fmt.Errorf("/!\\ Invalid response to security access request")
	}

	if d[3] == 0x00 && d[4] == 0x00 {
		log.Println("Security access already granted")
		return nil
	}

	secondsToWait := 1
	for secondsToWait > 0 {
		time.Sleep(1 * time.Second)
		t.SendKeepAlive(ctx)
		secondsToWait--
	}

	seed := []byte{d[3], d[4]}
	calc := calculateKey(seed, accesslevel)

	respPayload := []byte{0x04, 0x27, accesslevel + 0x01, calc[0], calc[1]}
	f2 := gocan.NewFrame(0x7E0, respPayload, gocan.ResponseRequired)
	resp2, err := t.c.SendAndPoll(ctx, f2, t.defaultTimeout, 0x7E8)
	if err != nil {
		log.Println(err)
		return err
	}

	if err := gmlan.CheckErr(resp2); err != nil {
		log.Println(err)
		return err
	}

	d2 := resp2.Data()
	if d2[1] == 0x67 && (d2[2] == AccessLevel01+0x01 || d2[2] == AccessLevelFB+0x01 || d2[2] == AccessLevelFD+0x01) {
		log.Println("Security access granted")
		return nil
	}

	return errors.New("/!\\ Failed to obtain security access")
}

func (t *Client) SendKeepAlive(ctx context.Context) {
	payload := []byte{0x01, 0x3E}
	f := gocan.NewFrame(0x7E0, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, f, t.defaultTimeout, 0x7E8)
	if err != nil {
		log.Printf("failed to send keep-alive: %v", err)
		return
	}
	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x7E {
		log.Println("Keep-alive invalid response", resp.String())
	}
}

func calculateKey(a_seed []byte, level byte) []byte {
	seed := int(a_seed[0])<<8 | int(a_seed[1])
	returnKey := make([]byte, 2)
	key := convertSeed(seed)

	switch level {
	case AccessLevelFB:
		key ^= 0x8749
		key += 0x06D3
		key ^= 0xCFDF
	case AccessLevelFD:
		key /= 3
		key ^= 0x8749
		key += 0x0ACF
		key ^= 0x81BF
	}

	returnKey[0] = (byte)((key >> 8) & 0xFF)
	returnKey[1] = (byte)(key & 0xFF)
	return returnKey

}

func convertSeed(seed int) int {
	key := seed>>5 | seed<<11
	return (key + 0xB988) & 0xFFFF
}
