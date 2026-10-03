//go:build ftdi

package canusb

// FT_Read blocks until len(p) bytes arrive or the timeout expires — ask only
// for what's queued, with a 1-byte read to wait for the first byte. (libftdi
// on Linux already returns what one latency-timer poll delivered.)
func (d d2xxPort) Read(p []byte) (int, error) {
	n, err := d.GetQueueStatus()
	if err != nil {
		return 0, err
	}
	if n < 1 {
		n = 1
	}
	if int(n) > len(p) {
		n = int32(len(p))
	}
	return d.Device.Read(p[:n])
}
