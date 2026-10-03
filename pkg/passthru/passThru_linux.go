//go:build linux

package passthru

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// PassThru drives a J2534 shared library through purego, so no cgo is
// needed. Every "unsigned long" in the J2534 API is 32 bits here, exactly
// as on Windows: that is the convention Linux J2534 drivers follow (see
// rnd-ash/ecu_diagnostics, which also defined the ~/.passthru/*.json layout
// FindDLLs reads), and PassThruMsg's layout depends on it. A driver built
// with a 64-bit unsigned long would not be usable through this binding.
//
// ReadMsgs and WriteMsgs run per frame (and per idle poll), so they are raw
// symbols called through purego.SyscallN: a RegisterFunc'd func goes through
// reflect, five allocations a call. SyscallN still costs one, its variadic
// argument slice, and moves what the pointer arguments point at to the heap
// (//go:uintptrescapes), so pass buffers that already live there.
type PassThru struct {
	lib uintptr

	passThruReadMsgs, passThruWriteMsgs uintptr

	passThruReadVersionProc func(deviceID uint32, firmware, dll, api *byte) uint32
	passThruOpen            func(name *byte, deviceID *uint32) uint32
	passThruClose           func(deviceID uint32) uint32
	passThruConnect         func(deviceID, protocolID, flags, baudRate uint32, channelID *uint32) uint32
	passThruDisconnect      func(channelID uint32) uint32
	passThruStartMsgFilter  func(channelID, filterType uint32, mask, pattern, flowControl *PassThruMsg, msgID *uint32) uint32
	passThruIoctl           func(handleID, ioctlID uint32, input, output unsafe.Pointer) uint32
	passThruGetLastError    func(description *byte) uint32
}

func New(libName string) (*PassThru, error) {
	lib, err := purego.Dlopen(libName, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	pt := &PassThru{lib: lib}
	for _, s := range []struct {
		name string
		fptr any
	}{
		{"PassThruReadVersion", &pt.passThruReadVersionProc},
		{"PassThruOpen", &pt.passThruOpen},
		{"PassThruClose", &pt.passThruClose},
		{"PassThruConnect", &pt.passThruConnect},
		{"PassThruDisconnect", &pt.passThruDisconnect},
		{"PassThruReadMsgs", &pt.passThruReadMsgs},
		{"PassThruWriteMsgs", &pt.passThruWriteMsgs},
		{"PassThruStartMsgFilter", &pt.passThruStartMsgFilter},
		{"PassThruIoctl", &pt.passThruIoctl},
		{"PassThruGetLastError", &pt.passThruGetLastError},
	} {
		sym, err := purego.Dlsym(lib, s.name)
		if err != nil {
			// Probing a library that turns out not to be J2534 would
			// otherwise leave it mapped for the life of the process.
			purego.Dlclose(lib)
			return nil, fmt.Errorf("%s: %w", libName, err)
		}
		if p, ok := s.fptr.(*uintptr); ok {
			*p = sym
			continue
		}
		purego.RegisterFunc(s.fptr, sym)
	}
	return pt, nil
}

func (j *PassThru) Close() error {
	return purego.Dlclose(j.lib)
}

// long PassThruOpen(void *pName, unsigned long *pDeviceID);
func (j *PassThru) PassThruOpen(deviceName string, pDeviceID *uint32) error {
	var pName *byte
	if deviceName != "" {
		b := []byte(deviceName + "\x00")
		pName = &b[0]
	}
	return j.checkErr(j.passThruOpen(pName, pDeviceID))
}

// long PassThruClose(unsigned long DeviceID);
func (j *PassThru) PassThruClose(deviceID uint32) error {
	return j.checkErr(j.passThruClose(deviceID))
}

// long PassThruConnect(unsigned long DeviceID, unsigned long ProtocolID, unsigned long Flags, unsigned long BaudRate, unsigned long *pChannelID);
func (j *PassThru) PassThruConnect(deviceID, protocolID, flags, baudRate uint32, pChannelID *uint32) error {
	return j.checkErr(j.passThruConnect(deviceID, protocolID, flags, baudRate, pChannelID))
}

// long PassThruDisconnect(unsigned long ChannelID);
func (j *PassThru) PassThruDisconnect(channelID uint32) error {
	return j.checkErr(j.passThruDisconnect(channelID))
}

// PassThruReadMsg reads a single message into pMsg and returns how many
// messages (0 or 1) the library actually delivered.
func (j *PassThru) PassThruReadMsg(channelID uint32, pMsg *PassThruMsg, timeout uint32) (uint32, error) {
	pNumMsgs := uint32(1)
	if err := j.PassThruReadMsgs(channelID, pMsg, &pNumMsgs, timeout); err != nil {
		return 0, err
	}
	return pNumMsgs, nil
}

// PassThruReadMsgs reads up to *pNumMsgs messages into the contiguous array
// starting at pMsg. The caller must provide at least *pNumMsgs messages of
// backing storage, e.g. &msgs[0] on a []PassThruMsg.
//
// long PassThruReadMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
func (j *PassThru) PassThruReadMsgs(channelID uint32, pMsg *PassThruMsg, pNumMsgs *uint32, timeout uint32) error {
	ret, _, _ := purego.SyscallN(j.passThruReadMsgs, uintptr(channelID), uintptr(unsafe.Pointer(pMsg)), uintptr(unsafe.Pointer(pNumMsgs)), uintptr(timeout))
	return j.checkErr(uint32(ret))
}

// long PassThruWriteMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
func (j *PassThru) PassThruWriteMsgs(channelID uint32, pMsg *PassThruMsg, pNumMsgs *uint32, timeout uint32) error {
	ret, _, _ := purego.SyscallN(j.passThruWriteMsgs, uintptr(channelID), uintptr(unsafe.Pointer(pMsg)), uintptr(unsafe.Pointer(pNumMsgs)), uintptr(timeout))
	return j.checkErr(uint32(ret))
}

// long PassThruStartMsgFilter(unsigned long ChannelID, unsigned long FilterType, PassThruMsg *pMaskMsg, PassThruMsg *pPatternMsg, PassThruMsg *pFlowControlMsg, unsigned long *pMsgID);
func (j *PassThru) PassThruStartMsgFilter(channelID, filterType uint32, pMaskMsg, pPatternMsg, pFlowControlMsg *PassThruMsg, pMsgID *uint32) error {
	return j.checkErr(j.passThruStartMsgFilter(channelID, filterType, pMaskMsg, pPatternMsg, pFlowControlMsg, pMsgID))
}

// long PassThruReadVersion(unsigned long DeviceID, char *pFirmwareVersion, char *pDllVersion, char *pApiVersion);
func (j *PassThru) PassThruReadVersion(deviceID uint32) (string, string, string, error) {
	var firmware, dll, api [80]byte
	if err := j.checkErr(j.passThruReadVersionProc(deviceID, &firmware[0], &dll[0], &api[0])); err != nil {
		return "", "", "", err
	}
	return cstr(firmware[:]), cstr(dll[:]), cstr(api[:]), nil
}

// long PassThruIoctl(unsigned long HandleID, unsigned long IoctlID, void *pInput, void *pOutput);
func (j *PassThru) ioctl(handleID, ioctlID uint32, input, output unsafe.Pointer) uint32 {
	return j.passThruIoctl(handleID, ioctlID, input, output)
}

// long PassThruGetLastError(char *pErrorDescription);
func (j *PassThru) PassThruGetLastError() (string, error) {
	var description [80]byte
	ret := j.passThruGetLastError(&description[0])
	return cstr(description[:]), CheckError(ret)
}
