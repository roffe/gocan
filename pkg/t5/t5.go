package t5

import (
	"time"

	"github.com/roffe/gocan"
)

const (
	PBusRate = 615.384
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
