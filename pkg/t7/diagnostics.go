package t7

import (
	"context"
	"fmt"
	"log"

	"github.com/avast/retry-go"
)

func (t *Trionic) StartDiagnosticSession(ctx context.Context) error {
	log.Println("starting diagnostics session")
	testerID := 0x240

	err := retry.Do(
		func() error {
			data := []byte{0x3F, 0x81, 0x00, 0x11, byte((testerID >> 8) & 0xFF), byte(testerID & 0xFF), 0x00, 0x00}
			t.c.SendFrame(0x220, data)

			f, err := t.c.Poll(ctx, t.defaultTimeout, 0x258)
			if err != nil {
				return err
			}
			if f.Data[0] == 0x40 && f.Data[3] == 0xC1 {
				log.Printf("Tester address: 0x%X\n", f.Data[1])
				log.Printf("ECU address: 0x%X\n", f.Data[2]|0x80)
				log.Printf("ECU ID: 0x%X", uint16(f.Data[6])<<8|uint16(f.Data[7]))
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
