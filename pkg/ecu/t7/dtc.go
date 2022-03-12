package t7

import (
	"context"
	"log"

	"github.com/roffe/gocan/pkg/kwp2000"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) ReadDTC(ctx context.Context) ([]model.DTC, error) {
	kw := kwp2000.New(t.c, 0x220, 0x238)
	if err := kw.StartSession(ctx); err != nil {
		return nil, err
	}

	ok, err := kw.RequestSecurityAccess(ctx, false)
	if err != nil {
		return nil, err
	}
	log.Printf("%t", ok)

	return nil, nil
}
