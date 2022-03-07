package t8

import (
	"context"
	"time"

	"github.com/roffe/gocan"
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
		defaultTimeout: 150 * time.Millisecond,
	}
	return t
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	return nil
}

func (t *Client) ResetECU(ctx context.Context, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) DumpECU(ctx context.Context, callback model.ProgressCallback) ([]byte, error) {
	if err := t.Bootstrap(ctx, callback); err != nil {
		return nil, err
	}

	time.Sleep(2 * time.Second)

	if callback != nil {
		callback("Exiting bootloader")
	}
	if err := t.LegionExit(ctx); err != nil {
		return nil, err
	}

	return nil, nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) EraseECU(ctx context.Context, callback model.ProgressCallback) error {
	return nil
}
