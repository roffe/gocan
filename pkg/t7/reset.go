package t7

import (
	"context"

	"github.com/roffe/gocan/pkg/model"
)

// Noop command to satisfy interface
func (t *Client) ResetECU(ctx context.Context, callback model.ProgressCallback) error {
	return nil
}
