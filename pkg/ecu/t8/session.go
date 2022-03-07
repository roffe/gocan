package t8

import (
	"context"

	"github.com/roffe/gocan"
)

func (t *Client) InitializeSession(ctx context.Context) error {
	f := gocan.NewFrame(0x11, []byte{0xFE, 0x01, 0x3E, 0x00, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing)
	return t.c.Send(f)
}
