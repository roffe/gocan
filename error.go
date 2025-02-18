package gocan

import "errors"

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
	return !errors.Is(err, unrecoverableError{})
}
