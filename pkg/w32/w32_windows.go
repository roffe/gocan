package w32

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	Modkernel32 *syscall.DLL
	procMap     = map[string]**syscall.Proc{
		"CreateEventW":        &procCreateEventW,
		"WaitForSingleObject": &procWaitForSingleObject,
		"CloseHandle":         &procCloseHandle,
	}
	procCreateEventW        *syscall.Proc
	procWaitForSingleObject *syscall.Proc
	procCloseHandle         *syscall.Proc
)

const (
	WAIT_OBJECT_0    = 0x00000000
	WAIT_TIMEOUT     = 0x00000102
	WAIT_FAILED      = 0xFFFFFFFF
	INFINITE_TIMEOUT = 0xFFFFFFFF
)

func init() {
	var err error
	Modkernel32, err = syscall.LoadDLL("kernel32.dll")
	if err != nil {
		panic(err)
	}
	for name, procPtr := range procMap {
		*procPtr, err = Modkernel32.FindProc(name)
		if err != nil {
			panic(err)
		}
	}
}

// LoadDLL loads a driver DLL by name from the system directory or the
// executable's, never from the working directory or PATH, where a planted
// copy would be found first. An absolute path loads that file, and the DLLs it
// imports are looked for in its own folder first.
func LoadDLL(name string) (*syscall.DLL, error) {
	flags := uintptr(windows.LOAD_LIBRARY_SEARCH_SYSTEM32 | windows.LOAD_LIBRARY_SEARCH_APPLICATION_DIR)
	if filepath.IsAbs(name) {
		flags = windows.LOAD_WITH_ALTERED_SEARCH_PATH
	}
	h, err := windows.LoadLibraryEx(name, 0, flags)
	if err != nil {
		return nil, &syscall.DLLError{Err: err, ObjName: name, Msg: "Failed to load " + name + ": " + err.Error()}
	}
	return &syscall.DLL{Name: name, Handle: syscall.Handle(h)}, nil
}

// CreateEvent creates or opens a named or unnamed event object.
// manualReset indicates whether the event must be manually reset.
// initialState indicates whether the event is initially signaled.
// name is the name of the event object. If empty, an unnamed event is created.
func CreateEvent(manualReset, initialState bool, name string) (syscall.Handle, error) {
	namePtr, _ := syscall.UTF16PtrFromString(name)
	mReset := uintptr(0)
	if manualReset {
		mReset = 1
	}
	iState := uintptr(0)
	if initialState {
		iState = 1
	}

	r1, _, err := procCreateEventW.Call(0, mReset, iState, uintptr(unsafe.Pointer(namePtr)))
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return 0, err
		}
		return 0, syscall.EINVAL
	}
	return syscall.Handle(r1), nil
}

// WaitForSingleObject(waitHandle, timeoutMs)
// timeoutMs can be INFINITE_TIMEOUT
//
// Receive loops call this per frame burst, so it uses syscall.SyscallN
// rather than Proc.Call, which heap allocates its argument slice.
func WaitForSingleObject(h syscall.Handle, timeoutMs uint32) (uint32, error) {
	r1, _, err := syscall.SyscallN(procWaitForSingleObject.Addr(), uintptr(h), uintptr(timeoutMs))
	ret := uint32(r1)
	switch ret {
	case WAIT_OBJECT_0:
		return ret, nil
	case WAIT_TIMEOUT:
		return ret, syscall.ETIMEDOUT
	case WAIT_FAILED:
		if err != 0 {
			return ret, err
		}
		return ret, syscall.EINVAL
	default:
		// shouldn't normally happen
		return ret, fmt.Errorf("WaitForSingleObject unexpected result 0x%08X", ret)
	}
}

// CloseHandle closes an open object handle.
func CloseHandle(h syscall.Handle) error {
	r1, _, err := procCloseHandle.Call(uintptr(h))
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
