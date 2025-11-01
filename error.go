package gocan

import (
	"errors"
	"fmt"
)

var (
	ErrErrorChannelFull       = errors.New("error channel full")
	ErrNillAdapter            = errors.New("adapter is nil")
	ErrDroppedFrame           = errors.New("adapter incoming channel full")
	ErrFramhandlerRegisterSub = errors.New("failed to register subscription, framehandler is full")
	ErrSendTimeout            = errors.New("timeout sending frame")
	ErrResponsechannelClosed  = errors.New("response channel closed")
)

type TimeoutError struct {
	Timeout int64
	Frames  []uint32
	Type    string
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s timeout (%dms) for frame 0x%03X", e.Type, e.Timeout, e.Frames)
}
