package passthru

import (
	"bytes"
	"fmt"
	"syscall"
	"unsafe"
)

type PassThru struct {
	dll                     *syscall.DLL
	passThruReadVersionProc *syscall.Proc
	passThruOpen            *syscall.Proc
	passThruClose           *syscall.Proc
	passThruConnect         *syscall.Proc
	passThruDisconnect      *syscall.Proc
	passThruReadMsgs        *syscall.Proc
	passThruWriteMsgs       *syscall.Proc
	passThruStartMsgFilter  *syscall.Proc
	passThruIoctl           *syscall.Proc
	passThruGetLastError    *syscall.Proc
}

func New(dllName string) (*PassThru, error) {
	dll, err := syscall.LoadDLL(dllName)
	if err != nil {
		return nil, err
	}
	pt := &PassThru{dll: dll}
	for _, p := range []struct {
		name string
		dst  **syscall.Proc
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
		proc, err := dll.FindProc(p.name)
		if err != nil {
			// Probing a DLL that turns out not to be J2534 would otherwise
			// leave it mapped for the life of the process.
			dll.Release()
			return nil, err
		}
		*p.dst = proc
	}
	return pt, nil
}

func (j *PassThru) Close() error {
	return j.dll.Release()
}

// checkErr maps a J2534 return code to an error, appending the DLL's own text
// description only for ERR_FAILED. Per J2534-1 v04.04 the description is valid
// only immediately after that code; for anything else the buffer holds
// undefined content and some DLLs do device I/O to produce it, which is not
// something to spend on every empty read.
func (j *PassThru) checkErr(ret uint32) error {
	err := CheckError(ret)
	if err == nil || ret != ERR_FAILED {
		return err
	}
	if str, err2 := j.PassThruGetLastError(); err2 == nil && str != "" {
		return fmt.Errorf("%s: %w", str, err)
	}
	return err
}

// # PASSTHRUCONNECT
//
// This function is used to establish a logical connection with a protocol channel on the specified SAE J2534
// device.  After this function is called, the value pointed to by pChannelID is used as the logical identifier for
// the combination of Device ID and Protocol ID. If the function is successful, a value of
// STATUS_NOERROR is returned and a valid channel ID will be placed in <pChannelID>. All future
// interactions with the protocol channel will be done using the pChannelID.  Note that the interface will
// block all received messages on this channel until a filter is set.
//
//	 extern "C" long WINAPI PassThruConnect
//
//		(
//			unsigned long DeviceID,
//			unsigned long ProtocolID,
//			unsigned long Flags,
//			unsigned long BaudRate,
//			unsigned long *pChannelID
//		)
//
// Parameters
//
//   - Device ID returned from PassThruOpen
//   - Protocol ID,
//   - Connection flags,
//   - Initial baud rate
//   - Pointer to location for the channel ID that is assigned by the DLL.
func (j *PassThru) PassThruConnect(deviceID, protocolID, flags, baudRate uint32, pChannelID *uint32) error {
	// long PassThruConnect(unsigned long DeviceID, unsigned long ProtocolID, unsigned long Flags, unsigned long BaudRate, unsigned long *pChannelID);
	ret, _, _ := j.passThruConnect.Call(
		uintptr(deviceID),
		uintptr(protocolID),
		uintptr(flags),
		uintptr(baudRate),
		uintptr(unsafe.Pointer(pChannelID)),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruDisconnect(channelID uint32) error {
	// long PassThruDisconnect(unsigned long ChannelID);
	ret, _, _ := j.passThruDisconnect.Call(
		uintptr(channelID),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruClose(deviceID uint32) error {
	// long PassThruClose(unsigned long DeviceID);
	ret, _, _ := j.passThruClose.Call(
		uintptr(deviceID),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruOpen(deviceName string, pDeviceID *uint32) error {
	var pName *byte
	if deviceName != "" {
		var err error
		if pName, err = syscall.BytePtrFromString(deviceName); err != nil {
			return err
		}
	}
	// long PassThruOpen(void* pName, unsigned long *pDeviceID);
	ret, _, _ := j.passThruOpen.Call(
		uintptr(unsafe.Pointer(pName)),
		uintptr(unsafe.Pointer(pDeviceID)),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruReadMsg(channelID uint32, pMsg *PassThruMsg, timeout uint32) (uint32, error) {
	pNumMsgs := uint32(1)
	// long PassThruReadMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
	ret, _, _ := j.passThruReadMsgs.Call(
		uintptr(channelID),
		uintptr(unsafe.Pointer(pMsg)),
		uintptr(unsafe.Pointer(&pNumMsgs)),
		uintptr(timeout),
	)
	if err := j.checkErr(uint32(ret)); err != nil {
		return 0, err
	}
	return pNumMsgs, nil
}

// PassThruReadMsgs reads up to *pNumMsgs messages into the contiguous array
// starting at pMsg. The caller must provide at least *pNumMsgs messages of
// backing storage, e.g. &msgs[0] on a []PassThruMsg.
func (j *PassThru) PassThruReadMsgs(channelID uint32, pMsg *PassThruMsg, pNumMsgs *uint32, timeout uint32) error {
	// long PassThruReadMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
	ret, _, _ := j.passThruReadMsgs.Call(
		uintptr(channelID),
		uintptr(unsafe.Pointer(pMsg)),
		uintptr(unsafe.Pointer(pNumMsgs)),
		uintptr(timeout),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruWriteMsgs(channelID uint32, pMsg *PassThruMsg, pNumMsgs *uint32, timeout uint32) error {
	// long PassThruWriteMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
	ret, _, _ := j.passThruWriteMsgs.Call(
		uintptr(channelID),
		uintptr(unsafe.Pointer(pMsg)),
		uintptr(unsafe.Pointer(pNumMsgs)),
		uintptr(timeout),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruStartMsgFilter(channelID, filterType uint32, pMaskMsg, pPatternMsg, pFlowControlMsg *PassThruMsg, pMsgID *uint32) error {
	// long PassThruStartMsgFilter(unsigned long ChannelID, unsigned long FilterType, PassThruMsg *pMaskMsg, PassThruMsg *pPatternMsg, PassThruMsg *pFlowControlMsg, unsigned long *pMsgID);
	ret, _, _ := j.passThruStartMsgFilter.Call(
		uintptr(channelID),
		uintptr(filterType),
		uintptr(unsafe.Pointer(pMaskMsg)),
		uintptr(unsafe.Pointer(pPatternMsg)),
		uintptr(unsafe.Pointer(pFlowControlMsg)),
		uintptr(unsafe.Pointer(pMsgID)),
	)
	return j.checkErr(uint32(ret))
}

func (j *PassThru) PassThruReadVersion(deviceID uint32) (string, string, string, error) {
	var pFirmwareVersion [80]byte
	var pDllVersion [80]byte
	var pApiVersion [80]byte

	// long PassThruReadVersion(unsigned long DeviceID, char *pFirmwareVersion, char *pDllVersion, char *pApiVersion);
	ret, _, _ := j.passThruReadVersionProc.Call(
		uintptr(deviceID),
		uintptr(unsafe.Pointer(&pFirmwareVersion)),
		uintptr(unsafe.Pointer(&pDllVersion)),
		uintptr(unsafe.Pointer(&pApiVersion)),
	)

	if err := j.checkErr(uint32(ret)); err != nil {
		return "", "", "", err
	}

	return cstr(pFirmwareVersion[:]), cstr(pDllVersion[:]), cstr(pApiVersion[:]), nil
}

// cstr returns the string up to the first NUL; the DLL need not zero the
// rest of the buffer.
func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// long PassThruIoctl(unsigned long HandleID, unsigned long IoctlID, void *pInput, void *pOutput);
func (j *PassThru) PassThruIoctl(handleID, ioctlID uint32, opts ...interface{}) error {
	switch ioctlID {
	case SET_CONFIG, GET_CONFIG:
		if len(opts) == 0 {
			return ErrInvalidParameter
		}
		list, ok := opts[0].(*SCONFIG_LIST)
		if !ok || list == nil {
			return ErrInvalidParameter
		}
		// Marshal to the C SCONFIG_LIST layout: { unsigned long NumOfParams;
		// SCONFIG *ConfigPtr; } — a Go slice header is not that. The count
		// comes from the slice rather than list.NumOfParams: the DLL walks
		// exactly that many entries and GET_CONFIG writes to them, so an
		// overlarge value would read and write past the Go slice.
		cList := struct {
			NumOfParams uint32
			ConfigPtr   *SCONFIG
		}{NumOfParams: uint32(len(list.Params))}
		if len(list.Params) > 0 {
			cList.ConfigPtr = &list.Params[0]
		}
		ret, _, _ := j.passThruIoctl.Call(
			uintptr(handleID),
			uintptr(ioctlID),
			uintptr(unsafe.Pointer(&cList)),
			uintptr(0),
		)
		return j.checkErr(uint32(ret))
	case CLEAR_MSG_FILTERS, CLEAR_RX_BUFFER, CLEAR_TX_BUFFER:
		ret, _, _ := j.passThruIoctl.Call(
			uintptr(handleID),
			uintptr(ioctlID),
			uintptr(0),
			uintptr(0),
		)
		return j.checkErr(uint32(ret))
	case FAST_INIT:
		if len(opts) != 2 {
			return ErrInvalidParameter
		}
		in, ok := opts[0].(*PassThruMsg)
		if !ok {
			return ErrInvalidParameter
		}
		out, ok := opts[1].(*PassThruMsg)
		if !ok {
			return ErrInvalidParameter
		}
		ret, _, _ := j.passThruIoctl.Call(
			uintptr(handleID),
			uintptr(ioctlID),
			uintptr(unsafe.Pointer(in)),
			uintptr(unsafe.Pointer(out)),
		)
		return j.checkErr(uint32(ret))
	}
	return ErrNotSupported
}

// long PassThruGetLastError(char *pErrorDescription);
func (j *PassThru) PassThruGetLastError() (string, error) {
	var pErrorDescription [80]byte
	ret, _, _ := j.passThruGetLastError.Call(
		uintptr(unsafe.Pointer(&pErrorDescription)),
	)
	return cstr(pErrorDescription[:]), CheckError(uint32(ret))
}
