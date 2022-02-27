package t7

import (
	"context"
	"fmt"
	"log"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
)

func (t *Client) StartDiagnosticSession(ctx context.Context) error {
	log.Println("starting diagnostics session")
	testerID := 0x240

	err := retry.Do(
		func() error {
			data := []byte{0x3F, 0x81, 0x00, 0x11, byte((testerID >> 8) & 0xFF), byte(testerID & 0xFF), 0x00, 0x00}
			t.c.SendFrame(0x220, data, gocan.ResponseRequired)
			f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
			if err != nil {
				return err
			}
			d := f.Data()
			if d[0] == 0x40 && d[3] == 0xC1 {
				log.Printf("Tester address: 0x%X\n", d[1])
				log.Printf("ECU address: 0x%X\n", d[2]|0x80)
				log.Printf("ECU ID: 0x%X", uint16(d[6])<<8|uint16(d[7]))
				return nil
			}

			return fmt.Errorf("invalid response to enter diagnostics session: %s", f.String())
		},
		retry.Context(ctx),
		retry.Attempts(5),
		retry.OnRetry(func(n uint, err error) {
			log.Println(err)
		}),
	)
	if err != nil {
		return err
	}

	return nil
}
