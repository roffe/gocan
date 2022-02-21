package t7

import "context"

// Noop command to satisfy interface
func (t *Client) ResetECU(ctx context.Context) error {
	return nil
}
