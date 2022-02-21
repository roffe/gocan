package t8

import (
	"time"

	"github.com/roffe/gocan"
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
