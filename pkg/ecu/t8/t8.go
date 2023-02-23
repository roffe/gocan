package t8

import (
	"context"
	"errors"
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
		legion:         legion.New(c, 0x7e0, 0x7e8),
		gm:             gmlan.New(c, 0x7e0, 0x5e8, 0x7e8),
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
	return t.gm.RequestSecurityAccess(ctx, 0x01, 0, t8sec.CalculateAccessKey)
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
	return t.gm.WriteDataByIdentifierUint32(ctx, pidOilQuality, uint32(quality*256))
}

func (t *Client) GetTopSpeed(ctx context.Context) (uint16, error) {
	resp, err := t.gm.ReadDataByIdentifierUint16(ctx, pidTopSpeed)
	if err != nil {
		return 0, err
	}
	speed := resp / 10
	return speed, nil
}

func (t *Client) SetTopSpeed(ctx context.Context, speed uint16) error {
	speed *= 10
	return t.gm.WriteDataByIdentifierUint16(ctx, pidTopSpeed, speed)
}

func (t *Client) GetRPMLimiter(ctx context.Context) (uint16, error) {
	return t.gm.ReadDataByIdentifierUint16(ctx, pidRPMLimiter)

}

func (t *Client) SetRPMLimit(ctx context.Context, limit uint16) error {
	return t.gm.WriteDataByIdentifierUint16(ctx, pidRPMLimiter, limit)
}

func (t *Client) GetVehicleVIN(ctx context.Context) (string, error) {
	return t.gm.ReadDataByIdentifierString(ctx, pidVIN)
}

func (t *Client) SetVehicleVIN(ctx context.Context, vin string) error {
	if len(vin) != 17 {
		return errors.New("invalid vin length")
	}
	return t.gm.WriteDataByIdentifier(ctx, pidVIN, []byte(vin))
}

const (
	pidRPMLimiter = 0x29
	pidOilQuality = 0x25
	pidTopSpeed   = 0x02
	pidVIN        = 0x90
)
