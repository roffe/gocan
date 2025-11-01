package pcan

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modkernel32 *syscall.DLL
	procMap     = map[string]**syscall.Proc{
		"CreateEventW":        &procCreateEventW,
		"WaitForSingleObject": &procWaitForSingleObject,
		"CloseHandle":         &procCloseHandle,
	}
	procCreateEventW        *syscall.Proc
	procWaitForSingleObject *syscall.Proc
	procCloseHandle         *syscall.Proc
)

func init() {
	var err error
	modkernel32, err = syscall.LoadDLL("kernel32.dll")
	if err != nil {
		panic(err)
	}
	for name, procPtr := range procMap {
		*procPtr, err = modkernel32.FindProc(name)
		if err != nil {
			panic(err)
		}
	}
}

const (
	WAIT_OBJECT_0    = 0x00000000
	WAIT_TIMEOUT     = 0x00000102
	WAIT_FAILED      = 0xFFFFFFFF
	INFINITE_TIMEOUT = 0xFFFFFFFF
)

// winCreateEvent wraps CreateEventW(NULL, FALSE, FALSE, NULL)
//
// bManualReset   = FALSE  -> auto-reset event
// bInitialState  = FALSE  -> start non-signaled
// lpName         = NULL   -> unnamed event
//
// Returns a Windows HANDLE as syscall.Handle.
func winCreateEvent() (syscall.Handle, error) {
	r1, _, e := procCreateEventW.Call(
		0, // lpEventAttributes (LPSECURITY_ATTRIBUTES) = NULL
		0, // bManualReset (BOOL) = FALSE  (auto-reset)
		0, // bInitialState (BOOL) = FALSE (nonsignaled initially)
		0, // lpName (LPCWSTR) = NULL
	)
	if r1 == 0 {
		if e != syscall.Errno(0) {
			return 0, e
		}
		return 0, syscall.EINVAL
	}
	return syscall.Handle(r1), nil
}

// winWaitForSingleObject(waitHandle, timeoutMs)
// timeoutMs can be INFINITE_TIMEOUT
func winWaitForSingleObject(h syscall.Handle, timeoutMs uint32) (uint32, error) {
	r1, _, e := procWaitForSingleObject.Call(
		uintptr(h),
		uintptr(timeoutMs),
	)
	ret := uint32(r1)

	switch ret {
	case WAIT_OBJECT_0, WAIT_TIMEOUT:
		return ret, nil
	case WAIT_FAILED:
		if e != syscall.Errno(0) {
			return ret, e
		}
		return ret, syscall.EINVAL
	default:
		// shouldn't normally happen
		return ret, fmt.Errorf("WaitForSingleObject unexpected result 0x%08X", ret)
	}
}

// winCloseHandle is just CloseHandle on the HANDLE we created.
func winCloseHandle(h syscall.Handle) error {
	r1, _, e := procCloseHandle.Call(uintptr(h))
	if r1 == 0 {
		if e != syscall.Errno(0) {
			return e
		}
		return syscall.EINVAL
	}
	return nil
}

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
	h, err := winCreateEvent()
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
		_ = winCloseHandle(h)
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
	if err := winCloseHandle(re.Handle); err != nil {
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

	res, err := winWaitForSingleObject(re.Handle, timeoutMs)
	if err != nil {
		return err
	}
	switch res {
	case WAIT_OBJECT_0:
		return nil
	case WAIT_TIMEOUT:
		return syscall.ETIMEDOUT
	default:
		return fmt.Errorf("unexpected wait result 0x%X", res)
	}
}
