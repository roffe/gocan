package gocan

import (
	"errors"
	"fmt"
)

type unrecoverableError struct {
	error
}

func (e unrecoverableError) Error() string {
	if e.error == nil {
		return "unrecoverable error"
	}
	return e.error.Error()
}

func (e unrecoverableError) Unwrap() error {
	return e.error
}

// Unrecoverable wraps an error in `unrecoverableError` struct
func Unrecoverable(err error) error {
	return unrecoverableError{err}
}

// IsRecoverable checks if error is an instance of `unrecoverableError`
func IsRecoverable(err error) bool {
	if _, ok := err.(unrecoverableError); ok {
		return false
	}
	return true
}

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
