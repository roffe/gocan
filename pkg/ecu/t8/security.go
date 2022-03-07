package t8

import (
	"context"
	"log"
	"time"

	"github.com/roffe/gocan/pkg/gmlan"
)

// Valid levels are 0x01, 0xFB, 0xFD
func (t *Client) RequestSecurityAccess(ctx context.Context, accesslevel byte, delay time.Duration) error {
	gm := gmlan.New(t.c)

	seed, err := gm.SecurityAccessRequestSeed(ctx, 0x7E0, 0x7E8, accesslevel)
	if err != nil {
		return err
	}

	if seed[0] == 0x00 && seed[1] == 0x00 {
		log.Println("Security access already granted")
		return nil
	}

	secondsToWait := delay.Milliseconds() / 1000
	for secondsToWait > 0 {
		time.Sleep(1 * time.Second)
		gmlan.New(t.c).TesterPresentNoResponseAllowed()
		secondsToWait--
	}

	high, low := calculateAccessKey(seed, accesslevel)

	if err := gm.SecurityAccessSendKey(ctx, 0x7E0, 0x7E8, accesslevel, high, low); err != nil {
		return err
	}

	return nil
}

func calculateAccessKey(a_seed []byte, level byte) (byte, byte) {
	seed := int(a_seed[0])<<8 | int(a_seed[1])

	key := convertSeed(seed)

	switch level {
	case 0xFB:
		key ^= 0x8749
		key += 0x06D3
		key ^= 0xCFDF
	case 0xFD:
		key /= 3
		key ^= 0x8749
		key += 0x0ACF
		key ^= 0x81BF
	}

	return (byte)((key >> 8) & 0xFF), (byte)(key & 0xFF)
}

func convertSeed(seed int) int {
	key := seed>>5 | seed<<11
	return (key + 0xB988) & 0xFFFF
}
