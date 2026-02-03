package gocan

import (
	"fmt"
	"os"
)

func setLatencyTimer(device string, latency int) error {
	// Construct the path to the latency_timer sysfs file
	// The path format is: /sys/bus/usb-serial/devices/ttyUSBX/latency_timer
	latencyPath := fmt.Sprintf("/sys/bus/usb-serial/devices/%s/latency_timer", device)

	// Write the latency value to the file
	err := os.WriteFile(latencyPath, []byte(fmt.Sprintf("%d", latency)), 0644)
	if err != nil {
		return fmt.Errorf("failed to set latency timer: %w", err)
	}
	return nil
}
