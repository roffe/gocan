package t8

import (
	"context"
	"encoding/binary"
	"log"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/ecu/t8sec"
	"github.com/roffe/gocan/pkg/gmlan"
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
	gm             *gmlan.Client
}

func New(c *gocan.Client) *Client {
	t := &Client{
		c:              c,
		defaultTimeout: 150 * time.Millisecond,
		legion:         legion.New(c),
		gm:             gmlan.New(c),
	}
	return t
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
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

func (t *Client) FlashECU(ctx context.Context, bin []byte, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) EraseECU(ctx context.Context, callback model.ProgressCallback) error {
	return nil
}

func (t *Client) RequestSecurityAccess(ctx context.Context) error {
	log.Println("Requesting security access")
	return t.gm.RequestSecurityAccess(ctx, 0xFD, 0, 0x7E0, 0x7E8, t8sec.CalculateAccessKey)
}

func (t *Client) GetOilQuality(ctx context.Context) (float64, error) {
	resp, err := t.RequestECUInfoAsUint64(ctx, pidOilQuality)
	if err != nil {
		return 0, err
	}
	quality := float64(resp) / 256
	return quality, nil
}

func (t *Client) SetOilQuality(ctx context.Context, quality float64) error {
	quality *= 256
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(quality))

	return t.gm.WriteDataByIdentifier(ctx, pidOilQuality, b, 0x7E0, 0x7E8)
}

func (t *Client) GetTopSpeed(ctx context.Context) (uint16, error) {
	resp, err := t.gm.ReadDataByIdentifier(ctx, pidTopSpeed, 0x7E0, 0x7E8)
	if err != nil {
		return 0, err
	}
	retval := uint16(resp[0]) * 256
	retval += uint16(resp[1])
	speed := retval / 10
	return speed, nil
}

func (t *Client) SetTopSpeed(ctx context.Context, speed uint16) error {
	speed *= 10
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(speed))
	return t.gm.WriteDataByIdentifier(ctx, pidTopSpeed, b, 0x7E0, 0x7E8)
}

func (t *Client) GetRPMLimiter(ctx context.Context) (uint32, error) {
	resp, err := t.gm.ReadDataByIdentifier(ctx, pidRPMLimiter, 0x7E0, 0x7E8)
	if err != nil {
		return 0, err
	}
	retval := uint32(resp[0]) * 256
	retval += uint32(resp[1])
	return retval, nil
}

func (t *Client) SetRPMLimit(ctx context.Context, limit uint16) error {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, limit)
	return t.gm.WriteDataByIdentifier(ctx, pidRPMLimiter, b, 0x7E0, 0x7E8)
}

func (t *Client) GetVehicleVIN(ctx context.Context) (string, error) {
	return t.RequestECUInfoAsString(ctx, pidVIN)
}

const (
	pidRPMLimiter = 0x29
	pidOilQuality = 0x25
	pidTopSpeed   = 0x02
	pidVIN        = 0x90
)
