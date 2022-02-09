package gmlan

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"time"

	"github.com/roffe/gocan"
)

type Client struct {
	c *gocan.Client
}

func New(c *gocan.Client) *Client {
	return &Client{
		c: c,
	}
}

func (cl *Client) DisableNormalCommunication(ctx context.Context) error {
	if err := cl.c.SendFrame(0x101, []byte{0xFE, 0x01, 0x28}); err != nil { // DisableNormalCommunication Request Message
		return err
	}
	return nil
}

func (cl *Client) WriteDataByIdentifier(ctx context.Context, canID uint32, identifier byte, data []byte) error {
	r := bytes.NewReader(data)

	firstPart := make([]byte, 4)
	_, err := r.Read(firstPart)
	if err != nil {
		if err == io.EOF {
			// do nothing
		} else {
			return err
		}
	}
	payload := []byte{0x10, byte(len(data) + 2), 0x3B, identifier}
	payload = append(payload, firstPart...)
	cl.c.SendFrame(canID, payload)
	log.Printf("%X\n", payload)
	resp, err := cl.c.Poll(ctx, 100*time.Millisecond, canID+0x400)
	if err != nil {
		return err
	}

	if resp.Data[0] != 0x30 || resp.Data[1] != 0x00 {
		log.Println(resp.String())
		return errors.New("invalid response to initial writeDataByIdentifier")
	}

	delay := resp.Data[2]

	var seq byte = 0x21

	for r.Len() > 0 {
		pkg := []byte{seq}
	inner:
		for i := 1; i < 8; i++ {
			b, err := r.ReadByte()
			if err != nil {
				if err == io.EOF {
					log.Println("eof")
					break inner
				}
				return err
			}
			pkg = append(pkg, b)
		}
		cl.c.SendFrame(canID, pkg)
		log.Printf("%X\n", pkg)
		time.Sleep(time.Duration(delay) * time.Millisecond)
		seq++
		if seq == 0x30 {
			seq = 0x20
		}
	}

	return nil
}

func (cl *Client) ReadDataByIdentifier(ctx context.Context, canID uint32, identifier byte) ([]byte, error) {
	out := bytes.NewBuffer([]byte{})
	cl.c.SendFrame(canID, []byte{0x02, 0x1A, identifier})
	resp, err := cl.c.Poll(ctx, 100*time.Millisecond, canID+0x400)
	if err != nil {
		return nil, err
	}
	if resp.Data[3] == 0x78 {
		resp, err = cl.c.Poll(ctx, 150*time.Millisecond, canID+0x400)
		if err != nil {
			return nil, err
		}
		out.Write(resp.Data[4:])
	} else {
		out.Write(resp.Data[4:])
	}

	left := int(resp.Data[1])
	left -= 6
	cl.c.SendFrame(canID, []byte{0x30, 0x00, 0x00})

outer:
	for left > 0 {
		read, err := cl.c.Poll(ctx, 100*time.Millisecond, canID+0x400)
		if err != nil {
			return nil, err
		}
		for _, b := range read.Data[1:] {
			out.WriteByte(b)
			left--
			if left == 0 {
				break outer
			}
		}

	}

	return out.Bytes(), nil
}
