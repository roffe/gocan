package canusb

import (
	"fmt"
	"sync/atomic"
)

type rawCommand struct {
	data      string
	processed chan struct{}
}

func (r *rawCommand) Send(c *Canusb) error {
	defer close(r.processed)

	n, err := c.port.Write(B(r.data + "\r"))
	if err != nil {
		return fmt.Errorf("failed to write %q to COM: %v", r.data, err)
	}

	atomic.AddUint64(&c.sentBytes, uint64(n))

	return nil
}
