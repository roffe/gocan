package t8mcp

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/legion"
	"github.com/roffe/gocan/pkg/model"
)

const (
	IBusRate = 47.619
	PBusRate = 500
)

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
	legion         *legion.Client
}

func New(c *gocan.Client) *Client {
	t := &Client{
		c:              c,
		defaultTimeout: 150 * time.Millisecond,
		legion:         legion.New(c, 0x7e0, 0x7e8),
	}
	return t
}

func (t *Client) ReadDTC(ctx context.Context) ([]model.DTC, error) {
	return nil, errors.New("MCP cannot do this")
}

func (t *Client) Info(ctx context.Context, callback model.ProgressCallback) ([]model.HeaderResult, error) {
	if err := t.legion.Bootstrap(ctx, callback); err != nil {
		return nil, err
	}

	_, err := t.legion.IDemand(ctx, legion.StartSecondaryBootloader, 0)
	if err != nil {
		return nil, errors.New("failed to start secondary bootloader")
	}

	ver, err := t.legion.GetMCPVersion(ctx)
	if err != nil {
		return nil, err
	}

	if callback != nil {
		callback("MCP Firmware information: " + ver)
	}

	return nil, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	return nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) DumpECU(ctx context.Context, callback model.ProgressCallback) ([]byte, error) {
	if err := t.legion.Bootstrap(ctx, callback); err != nil {
		return nil, err
	}

	_, err := t.legion.IDemand(ctx, legion.StartSecondaryBootloader, 0)
	if err != nil {
		return nil, errors.New("failed to start secondary bootloader")
	}

	if callback != nil {
		callback("Dumping MCP")
	}
	start := time.Now()

	bin, err := t.legion.ReadFlash(ctx, legion.EcuByte_MCP, 0x40100, false, callback)
	if err != nil {
		return nil, err
	}

	if callback != nil {
		callback("Verifying md5..")
	}

	ecumd5bytes, err := t.legion.IDemand(ctx, legion.GetTrionic8MCPMD5, 0x00)
	if err != nil {
		return nil, err
	}
	calculatedMD5 := md5.Sum(bin)

	if callback != nil {
		callback(fmt.Sprintf("Legion md5 : %X", ecumd5bytes))
		callback(fmt.Sprintf("Local md5  : %X", calculatedMD5))
	}

	if !bytes.Equal(ecumd5bytes, calculatedMD5[:]) {
		return nil, errors.New("md5 Verification failed")
	}

	if callback != nil {
		callback("Done, took: " + time.Since(start).String())
	}

	return bin, nil
}

func (t *Client) EraseECU(ctx context.Context, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) ResetECU(ctx context.Context, callback model.ProgressCallback) error {
	if t.legion.IsRunning() {
		if err := t.legion.Exit(ctx); err != nil {
			return err
		}
	}
	return nil
}
