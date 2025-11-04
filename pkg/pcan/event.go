package pcan

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/roffe/gocan/pkg/w32"
)

// ----- PCAN receive event glue -----
//
// The PCAN docs say:
//
//   HANDLE readEvent = CreateEvent(NULL, FALSE, FALSE, NULL);
//   DWORD eventValue = (DWORD)readEvent;
//   CAN_SetValue(channelUsed, PCAN_RECEIVE_EVENT, &eventValue, sizeof(eventValue));
//
// Meaning: you give the driver an event handle (as a DWORD) so it can
// SetEvent() whenever a CAN frame is queued in its RX FIFO.
// After that you can WaitForSingleObject(readEvent, ...) instead of polling.
//
// Notes for Go / 64-bit:
// - HANDLE is pointer-sized (64-bit on 64-bit Windows).
// - PCAN expects a DWORD (32-bit) even on 64-bit, according to their samples.
//   So they explicitly cast (DWORD)readEvent.
// - We'll mirror that: take the low 32 bits of the HANDLE value.
//   This matches their sample code and usually works because Windows
//   kernel handle values are 32-bit numbers zero-extended to 64-bit.
//
// If Peak ever publishes a 64-bit safe variant that wants uint64, you'd
// change this, but this matches the reference code you pasted.

type ReceiveEvent struct {
	Handle syscall.Handle
}

// SetReceiveEvent creates an auto-reset event, gives it to the PCAN driver
// via PCAN_RECEIVE_EVENT, and returns a small helper struct so you can wait
// on it and later clean it up.
func SetReceiveEvent(channel TPCANHandle) (*ReceiveEvent, error) {
	// 1. Create the Windows event
	h, err := w32.CreateEvent(false, false, "")
	if err != nil {
		return nil, fmt.Errorf("CreateEvent failed: %w", err)
	}

	// 2. Convert HANDLE -> DWORD like the C sample (truncate to 32 bits)
	eventVal32 := uint32(uintptr(h) & 0xFFFFFFFF)

	// 3. Call CAN_SetValue(channel, PCAN_RECEIVE_EVENT, &eventVal32, sizeof(DWORD))
	if err := CAN_SetValue(
		channel,
		PCAN_RECEIVE_EVENT,
		uintptr(unsafe.Pointer(&eventVal32)),
		4, // sizeof(DWORD)
	); err != nil {
		// important: close handle if we fail to register it
		_ = w32.CloseHandle(h)
		return nil, fmt.Errorf("CAN_SetValue(PCAN_RECEIVE_EVENT) failed: %w", err)
	}

	return &ReceiveEvent{Handle: h}, nil
}

// ClearReceiveEvent tells the PCAN driver "no event" and closes the handle.
// This mirrors passing 0 back to PCAN_RECEIVE_EVENT when you're done.
func (re *ReceiveEvent) ClearReceiveEvent(channel TPCANHandle) error {
	if re == nil || re.Handle == 0 {
		return nil
	}

	// Tell driver to stop signaling any event:
	var zero uint32 = 0
	if err := CAN_SetValue(
		channel,
		PCAN_RECEIVE_EVENT,
		uintptr(unsafe.Pointer(&zero)),
		4,
	); err != nil {
		return fmt.Errorf("CAN_SetValue(PCAN_RECEIVE_EVENT=0) failed: %w", err)
	}

	// Close our side of the handle
	if err := w32.CloseHandle(re.Handle); err != nil {
		return fmt.Errorf("CloseHandle failed: %w", err)
	}

	re.Handle = 0
	return nil
}

// Wait blocks until the PCAN driver signals that there is at least one CAN
// frame queued in RX, or until timeoutMs elapses.
// timeoutMs can be INFINITE_TIMEOUT if you want to block forever.
// Returns:
//   - nil if signaled (you should now call CAN_Read in a loop until queue empty)
//   - syscall.ETIMEDOUT if it timed out
//   - other error on failure
func (re *ReceiveEvent) Wait(timeoutMs uint32) error {
	if re == nil || re.Handle == 0 {
		return fmt.Errorf("ReceiveEvent not initialized")
	}

	res, err := w32.WaitForSingleObject(re.Handle, timeoutMs)
	if err != nil {
		return err
	}
	switch res {
	case w32.WAIT_OBJECT_0:
		return nil
	case w32.WAIT_TIMEOUT:
		return syscall.ETIMEDOUT
	default:
		return fmt.Errorf("unexpected wait result 0x%X", res)
	}
}
