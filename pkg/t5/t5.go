package t5

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

const (
	PBusRate = 615.384
)

type ECUType int

const (
	T52ECU ECUType = iota
	T55ECU16MHZAMDIntel
	T55ECU16MHZCatalyst
	T55ECU20MHZ
	Autodetect
	UnknownECU
	T55ECU
	T55AST52
)

const (
	Partnumber byte = 0x01
	SoftwareID byte = 0x02
	Dataname   byte = 0x03 // SW Version
	EngineType byte = 0x04
	ImmoCode   byte = 0x05
	Unknown    byte = 0x06
	ROMend     byte = 0xFC // Always 07FFFF
	ROMoffset  byte = 0xFD // T5.5 = 040000, T5.2 = 020000
	CodeEnd    byte = 0xFE
)

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
	bootloaded     bool
}

func New(c *gocan.Client) *Client {
	t := &Client{
		c:              c,
		defaultTimeout: 250 * time.Millisecond,
	}
	return t
}

var chipTypes []byte

func (t *Client) GetChipTypes(ctx context.Context) ([]byte, error) {
	if len(chipTypes) > 0 {
		return chipTypes, nil
	}
	frame := model.NewFrame(0x5, []byte{0xC9, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, model.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0xC)
	if err != nil {
		return nil, err
	}
	d := resp.Data()
	if d[0] != 0xC9 || d[1] != 0x00 {
		return nil, errors.New("invalid GetChipTypes response")
	}
	chipTypes = d[2:]
	return chipTypes, nil
}

func (t *Client) ReadMemoryByAddress(ctx context.Context, address uint32) ([]byte, error) {
	p := []byte{0xC7, byte(address >> 24), byte(address >> 16), byte(address >> 8), byte(address), 0x00, 0x00, 0x00}
	frame := model.NewFrame(0x5, p, model.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0xC)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory by address: %v", err)
	}
	data := resp.Data()[2:]
	reverse(data)
	return data, nil
}

func (t *Client) ResetECU(ctx context.Context) error {
	if !t.bootloaded {
		t.UploadBootLoader(ctx)
	}
	log.Println("Resetting ECU")
	frame := model.NewFrame(0x5, []byte{0xC2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, model.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0xC)
	if err != nil {
		return fmt.Errorf("failed to reset ECU: %v", err)
	}
	data := resp.Data()
	if data[0] != 0xC2 || data[1] != 0x00 || data[2] != 0x08 {
		return errors.New("invalid response to reset ECU")
	}
	return nil
}

func reverse(s []byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
