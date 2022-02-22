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

func (t *Client) Info(ctx context.Context) ([]model.HeaderResult, error) {
	return nil, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	return nil
}

func (t *Client) ResetECU(ctx context.Context) error {
	return nil
}

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	return nil, nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) EraseECU(ctx context.Context) error {
	return nil
}
