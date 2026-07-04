package gocan

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrDroppedFrame is raised as an error event by adapters when an incoming
	// frame had to be dropped because the receive buffer was full.
	ErrDroppedFrame = errors.New("adapter incoming channel full")

	// ErrResponseChannelClosed is returned when a wait ended because the
	// subscription's delivery channel was closed.
	ErrResponseChannelClosed = errors.New("response channel closed")

	// Deprecated: use ErrResponseChannelClosed.
	ErrResponsechannelClosed = ErrResponseChannelClosed
)

// TimeoutError is returned when a send or receive operation times out.
type TimeoutError struct {
	Timeout time.Duration
	Frames  []uint32
	Type    string // operation that timed out, e.g. "send"
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s timeout (%s) for frame 0x%03X", e.Type, e.Timeout, e.Frames)
}
