package kwp2000

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/roffe/gocan"
)

type Client struct {
	c                 *gocan.Client
	canID             uint32
	recvID            []uint32
	defaultTimeout    time.Duration
	gotSequrityAccess bool
}

type KWPRequest struct {
}

type KWPReply struct {
}

func New(c *gocan.Client, canID uint32, recvID ...uint32) *Client {
	return &Client{
		c:              c,
		canID:          canID,
		recvID:         recvID,
		defaultTimeout: 150 * time.Millisecond,
	}
}

func (t *Client) StartSession(ctx context.Context) error {
	payload := []byte{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00}
	frame := gocan.NewFrame(t.canID, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, t.recvID...)
	if err != nil {
		return err
	}
	log.Println(resp.String())
	return nil
}

func (t *Client) RequestSecurityAccess(ctx context.Context, force bool) (bool, error) {
	if t.gotSequrityAccess && !force {
		return true, nil
	}
	for i := 1; i <= 2; i++ {
		ok, err := t.requestSecurityAccessLevel(ctx, i)
		if err != nil {
			return false, err
		}
		if ok {
			t.gotSequrityAccess = true

			break
		}
	}

	return false, errors.New("security access was not granted")
}

func (t *Client) requestSecurityAccessLevel(ctx context.Context, method int) (bool, error) {
	log.Println("requestSecurityAccessLevel", method)

	return false, nil
}

func (t *Client) SendRequest(req *KWPRequest) (*KWPReply, error) {
	return nil, nil
}
