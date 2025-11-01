package pcan

import (
	"log"
	"syscall"
	"unsafe"
)

var (
	funcMap = map[string]**syscall.Proc{
		"CAN_Initialize":     &procCANInitialize,
		"CAN_InitializeFD":   &procCANInitializeFD,
		"CAN_Uninitialize":   &procCANUninitialize,
		"CAN_Reset":          &procCANReset,
		"CAN_GetStatus":      &procCANGetStatus,
		"CAN_Read":           &procCANRead,
		"CAN_ReadFD":         &procCANReadFD,
		"CAN_Write":          &procCANWrite,
		"CAN_WriteFD":        &procCANWriteFD,
		"CAN_FilterMessages": &procCANFilterMessages,
		"CAN_GetValue":       &procGetValue,
		"CAN_SetValue":       &procSetValue,
		"CAN_GetErrorText":   &procCANGetErrorText,
		"CAN_LookUpChannel":  &procCANLookupChannel,
	}

	pcan                  *syscall.DLL
	procCANInitialize     *syscall.Proc
	procCANInitializeFD   *syscall.Proc
	procCANUninitialize   *syscall.Proc
	procCANReset          *syscall.Proc
	procCANGetStatus      *syscall.Proc
	procCANRead           *syscall.Proc
	procCANReadFD         *syscall.Proc
	procCANWrite          *syscall.Proc
	procCANWriteFD        *syscall.Proc
	procCANFilterMessages *syscall.Proc
	procGetValue          *syscall.Proc
	procSetValue          *syscall.Proc
	procCANGetErrorText   *syscall.Proc
	procCANLookupChannel  *syscall.Proc
)

func init() {
	var err error
	pcan, err = syscall.LoadDLL("PCANBasic.dll")
	if err != nil {
		log.Println(err)
		return
	}

	for name, proc := range funcMap {
		*proc, err = pcan.FindProc(name)
		if err != nil {
			panic(err)
		}
	}
}

type PCANError struct {
	Code TPCANStatus
}

func (e PCANError) Error() string {
	return GetErrorText(e.Code)
}

func checkErr(r1, _ uintptr, _ error) error {
	if r1 != uintptr(PCAN_ERROR_OK) {
		return PCANError{Code: TPCANStatus(r1)}
	}
	return nil
}

// CAN_Initialize connects and activates a PCAN channel at a given bit rate.
//
// After a successful call, the channel is considered "initialized":
//   - The CAN controller is put online.
//   - The receive queue for that channel starts filling with any traffic
//     on the bus immediately (unless filtered).
//
// The baudrate argument is the classic CAN bitrate encoded as the
// controller BTR0/BTR1 register pair for SJA1000-style controllers.
// Example values include PCAN_BAUD_500K, PCAN_BAUD_250K, etc.
//
// This call will fail if the channel is already in use by another client,
// unless special modes (like listen-only or bitrate adapting) were
// preconfigured via CAN_SetValue before calling Initialize.
//
// Some parameters (e.g. PCAN_LISTEN_ONLY, PCAN_BITRATE_ADAPTING) are
// "pre-initialization only": they must be set with CAN_SetValue before
// calling CAN_Initialize in order to affect how initialization behaves.
func CAN_Initialize(channel TPCANHandle, baudrate TPCANBaudrate) error {
	return checkErr(procCANInitialize.Call(uintptr(channel), uintptr(baudrate)))
}

// CAN_Uninitialize disconnects a PCAN channel and frees driver resources.
//
// After this call:
//   - The channel is no longer considered initialized.
//   - Any receive event handle you previously assigned with
//     PCAN_RECEIVE_EVENT stops being signaled by the driver, and you
//     should CloseHandle it yourself on Windows because the API will
//     not do that for you. Leaving a stale HANDLE around after
//     uninitializing can lead to weird behavior.
//
// Passing PCAN_NONEBUS uninitializes *all* active channels in one go
func CAN_Uninitialize(channel TPCANHandle) error {
	return checkErr(procCANUninitialize.Call(uintptr(channel)))
}

// CAN_Reset clears the transmit and receive queues of an initialized channel.
//
// In PCAN-Basic terms, "reset" is basically flushing both RX and TX FIFOs.
// It does not forcibly close and reopen the device; it just clears buffers.
//
// On some hardware, if you've enabled features like HARD_RESET_STATUS
// via CAN_SetValue (PCAN_HARD_RESET_STATUS), calling Reset after that
// can additionally request a hardware-level reset.
func CAN_Reset(channel TPCANHandle) error {
	return checkErr(procCANReset.Call(uintptr(channel)))
}

// CAN_GetStatus queries the current status/error flags of a channel.
//
// The returned TPCANStatus bitfield can include:
//   - bus state info (bus light / heavy / passive / bus-off),
//   - queue states (receive empty, transmit full),
//   - and general driver / handle errors (not initialized, invalid handle, etc.).
//
// You can poll this if you want to know if you're bus-off, if you’re
// overflowing RX, etc., without doing a read.
func CAN_GetStatus(channel TPCANHandle) (TPCANStatus, error) {
	var status TPCANStatus
	err := checkErr(procCANGetStatus.Call(uintptr(channel), uintptr(unsafe.Pointer(&status))))
	return status, err
}

// CAN_Read dequeues one classic CAN frame (up to 8 data bytes) from the
// channel's receive queue and optionally returns its timestamp.
//
// Behavior:
//   - On success, *message is filled with ID, DLC, type flags, and DATA[0..7].
//   - timestamp (if non-nil) receives the hardware timestamp of that frame.
//   - If the receive queue is empty, PCAN_ERROR_QRCVEMPTY is returned.
//
// Typical usage is to call this in a loop until CAN_Read returns
// PCAN_ERROR_QRCVEMPTY. This is especially important when you're using
// PCAN_RECEIVE_EVENT: once the event is signaled, you’re expected to
// drain the queue completely so the event can be triggered again next
// time new data arrives.
func CAN_Read(channel TPCANHandle, message *TPCANMsg, timestamp *TPCANTimestamp) error {
	return checkErr(procCANRead.Call(
		uintptr(channel),
		uintptr(unsafe.Pointer(message)),
		uintptr(unsafe.Pointer(timestamp)),
	))
}

// CAN_ReadFD dequeues one CAN FD frame (up to 64 data bytes) plus a timestamp.
//
// This is the FD-capable equivalent of CAN_Read. The returned structure
// uses DLC (0..15) and a 64-byte DATA buffer to cover CAN FD payloads.
//
// Same draining rule applies for event-driven reads: if you use
// PCAN_RECEIVE_EVENT on an FD-capable channel, read until you get
// "queue empty" so the driver can re-signal on the next burst.
func CAN_ReadFD(channel TPCANHandle, message *TPCANMsgFD, timestamp *TPCANTimestamp) error {
	return checkErr(procCANReadFD.Call(
		uintptr(channel),
		uintptr(unsafe.Pointer(message)),
		uintptr(unsafe.Pointer(timestamp)),
	))
}

// CAN_Write enqueues one classic CAN frame for transmission.
//
// You fill in:
//
//	message.ID       = 11-bit or 29-bit CAN ID
//	message.LEN      = number of data bytes (0..8)
//	message.DATA[0:] = payload
//	message.MSGTYPE  = flags (standard/extended/rtr/etc.)
//
// If the driver's transmit queue is full, you'll get PCAN_ERROR_QXMTFULL.
//
// PCAN-Basic examples use this to send "trigger" frames to wake up a
// device and then wait on a read event to know when the device answers.
func CAN_Write(channel TPCANHandle, message *TPCANMsg) error {
	return checkErr(procCANWrite.Call(uintptr(channel), uintptr(unsafe.Pointer(message))))
}

// CAN_WriteFD enqueues one CAN FD frame (up to 64 bytes payload) for TX.
//
// Same idea as CAN_Write, but you provide a TPCANMsgFD with DLC (0..15)
// and DATA[0..63].
func CAN_WriteFD(channel TPCANHandle, message *TPCANMsgFD) error {
	return checkErr(procCANWriteFD.Call(uintptr(channel), uintptr(unsafe.Pointer(message))))
}

// CAN_FilterMessages programs a reception filter range on the CAN controller.
//
// After initialization, a PCAN channel will accept *all* CAN traffic by default
// (the equivalent of PCAN_FILTER_OPEN).
//
// Calling CAN_FilterMessages() tells the driver "only accept CAN IDs in
// [fromID..toID]" (or close the filter completely depending on mode).
// Under the hood this adjusts the controller's acceptance mask/code.
//
// NOTE:
//
//   - Changing the acceptance filter may cause a controller reset on some
//     hardware. If another app shares that same physical adapter, its traffic
//     can momentarily be affected.
//
//   - The driver will automatically close the filter before applying the new
//     custom range, if it was previously "open".
//
// For more advanced, SJA1000-style acceptance code/mask programming
// (like matching multiple disjoint IDs), PCAN also exposes parameters
// PCAN_ACCEPTANCE_FILTER_11BIT / _29BIT via CAN_SetValue instead of
// calling this function multiple times.
func CAN_FilterMessages(channel TPCANHandle, fromID uint32, toID uint32, mode TPCANMode) error {
	return checkErr(procCANFilterMessages.Call(
		uintptr(channel),
		uintptr(fromID),
		uintptr(toID),
		uintptr(mode),
	))
}

// CAN_GetValue reads a PCAN-Basic "parameter" into caller-provided memory.
//
// Parameters are how you query things like:
//   - API / firmware / driver versions (PCAN_API_VERSION, PCAN_CHANNEL_VERSION),
//   - hardware identity (PCAN_DEVICE_ID, PCAN_DEVICE_GUID,
//     PCAN_HARDWARE_NAME, PCAN_CONTROLLER_NUMBER),
//   - bus speed info (PCAN_BITRATE_INFO, PCAN_BUSSPEED_NOMINAL),
//   - driver/channel state (PCAN_CHANNEL_CONDITION),
//   - capabilities / features, etc.
//
// Some parameters can be read even before the channel is initialized
// (for example, things like PCAN_CHANNEL_CONDITION or version info),
// while others require the channel to already be initialized. The PDF
// marks each parameter with "Initialization Status" to tell you which
// case you're in.
//
// buffer must point to storage large enough for the expected value,
// and bufferLength must match that size in bytes.
func CAN_GetValue(channel TPCANHandle, parameter TPCANParameter, buffer uintptr, bufferLength uint32) error {
	return checkErr(procGetValue.Call(
		uintptr(channel),
		uintptr(parameter),
		buffer,
		uintptr(bufferLength),
	))
}

// CAN_SetValue writes/configures a PCAN-Basic "parameter".
//
// You use this to:
//
//   - Enable special init modes *before* Initialize(), e.g.:
//     PCAN_LISTEN_ONLY,
//     PCAN_BITRATE_ADAPTING (auto connect even if bitrate is unknown),
//     etc. Those are marked "pre-initialization only".
//
//   - Control runtime behavior *after* Initialize(), e.g.:
//     PCAN_RECEIVE_EVENT     → tell the driver which Win32 event HANDLE
//     to Set() when new RX data arrives, so your
//     app can WaitForSingleObject instead of polling.
//
//     PCAN_MESSAGE_FILTER    → temporarily open/close reception so you
//     can "pause" processing without tearing
//     down the channel.
//
//     PCAN_ALLOW_*_FRAMES    → choose which frame types (status, RTR,
//     error, echo) you want delivered.
//
// Some parameters reset back to default after you Uninitialize or
// re-Initialize the channel (PCAN_RECEIVE_EVENT is one of those: you
// must set it again every time you bring the channel up).
func CAN_SetValue(channel TPCANHandle, parameter TPCANParameter, buffer uintptr, bufferLength uint32) error {
	return checkErr(procSetValue.Call(
		uintptr(channel),
		uintptr(parameter),
		buffer,
		uintptr(bufferLength),
	))
}

// CAN_GetErrorText turns a TPCANStatus code into a human-readable string.
//
// You pass the status you got (for example from CAN_Read, CAN_Write,
// CAN_Initialize, etc.), plus a language ID (0 = English in the PCAN
// samples), and a caller-owned text buffer.
//
// On success, textBuffer will contain a null-terminated ASCII/ANSI
// description like "Channel not initialized" or "Receive queue empty".
//
// This is handy for surfacing driver errors directly to the user.
func CAN_GetErrorText(errorCode TPCANStatus, language uint16, textBuffer *byte, bufferLength uint32) error {
	return checkErr(procCANGetErrorText.Call(
		uintptr(errorCode),
		uintptr(language),
		uintptr(unsafe.Pointer(textBuffer)),
		uintptr(bufferLength),
	))
}

// CAN_LookupChannel asks the driver to resolve a human/descriptor-style
// name (like "PCAN_USBBUS1" or a parameter string describing bus type,
// device ID, controller index, etc.) into a concrete TPCANHandle.
//
// This is useful when you discover hardware at runtime (for example
// by enumerating available channels via PCAN_AVAILABLE_CHANNELS /
// PCAN_AVAILABLE_CHANNELS_COUNT) and then want to open a specific one
// without hardcoding PCAN_USBBUS1, PCAN_USBBUS2, etc.
//
// The handle it returns is what you then pass to CAN_Initialize,
// CAN_Read, CAN_Write, etc.
func CAN_LookupChannel(channelName string) (TPCANHandle, error) {
	namePtr, err := syscall.BytePtrFromString(channelName)
	if err != nil {
		return 0, err
	}
	var handle TPCANHandle
	err = checkErr(procCANLookupChannel.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&handle)),
	))
	return handle, err
}

// -----------------------------------------------------------------------------
// Extended functionality / helper functions
// -----------------------------------------------------------------------------

type ChannelCondition uint8

const (
	CHANNEL_UNAVAILABLE ChannelCondition = 0x00                                   // Channel handle invalid/not available
	CHANNEL_AVAILABLE   ChannelCondition = 0x01                                   // Channel handle is available
	CHANNEL_OCCUPIED    ChannelCondition = 0x02                                   // Channel already in use
	CHANNEL_PCANVIEW    ChannelCondition = (CHANNEL_AVAILABLE | CHANNEL_OCCUPIED) // In use by PCAN-View but still connectable
)

func (c ChannelCondition) String() string {
	switch c {
	case CHANNEL_UNAVAILABLE:
		return "CHANNEL_UNAVAILABLE"
	case CHANNEL_AVAILABLE:
		return "CHANNEL_AVAILABLE"
	case CHANNEL_OCCUPIED:
		return "CHANNEL_OCCUPIED"
	case CHANNEL_PCANVIEW:
		return "CHANNEL_PCANVIEW"
	default:
		return "UNKNOWN"
	}
}

func cString(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func GetChannelCondition(channel TPCANHandle) ChannelCondition {
	var condition ChannelCondition
	if err := CAN_GetValue(channel, PCAN_CHANNEL_CONDITION, uintptr(unsafe.Pointer(&condition)), 1); err != nil {
		log.Println(err)
		return CHANNEL_UNAVAILABLE
	}
	return ChannelCondition(condition)
}

func SetChannelIdentifying(channel TPCANHandle, enabled bool) error {
	var param uint32
	if enabled {
		param = PCAN_PARAMETER_ON
	}
	return CAN_SetValue(channel, PCAN_CHANNEL_IDENTIFYING, uintptr(unsafe.Pointer(&param)), 4)
}

func GetDeviceID(channel TPCANHandle) (uint32, error) {
	var deviceID uint32
	if err := CAN_GetValue(channel, PCAN_DEVICE_ID, uintptr(unsafe.Pointer(&deviceID)), 4); err != nil {
		return 0, err
	}
	return deviceID, nil
}

func GetHardwareName(channel TPCANHandle) (string, error) {
	var name [MAX_LENGTH_HARDWARE_NAME]byte
	if err := CAN_GetValue(channel, PCAN_HARDWARE_NAME, uintptr(unsafe.Pointer(&name[0])), MAX_LENGTH_HARDWARE_NAME); err != nil {
		return "", err
	}
	return cString(name[:]), nil
}

func GetDevicePartNumber(channel TPCANHandle) (string, error) {
	var partNumber [100]byte
	if err := CAN_GetValue(channel, PCAN_DEVICE_PART_NUMBER, uintptr(unsafe.Pointer(&partNumber[0])), 100); err != nil {
		return "", err
	}
	return cString(partNumber[:]), nil
}

func GetDeviceGUID(channel TPCANHandle) (string, error) {
	var guid [MAX_LENGTH_VERSION_STRING]byte
	if err := CAN_GetValue(channel, PCAN_DEVICE_GUID, uintptr(unsafe.Pointer(&guid[0])), MAX_LENGTH_VERSION_STRING); err != nil {
		return "", err
	}
	return cString(guid[:]), nil
}

func GetAPIVersion() (string, error) {
	var version [MAX_LENGTH_VERSION_STRING]byte
	if err := CAN_GetValue(0, PCAN_API_VERSION, uintptr(unsafe.Pointer(&version[0])), MAX_LENGTH_VERSION_STRING); err != nil {
		return "", err
	}
	return cString(version[:]), nil
}

func GetChannelVersion(channel TPCANHandle) (string, error) {
	var version [MAX_LENGTH_VERSION_STRING]byte
	if err := CAN_GetValue(channel, PCAN_CHANNEL_VERSION, uintptr(unsafe.Pointer(&version[0])), MAX_LENGTH_VERSION_STRING); err != nil {
		return "", err
	}
	return cString(version[:]), nil
}

func GetChannelFeatures(channel TPCANHandle) (uint32, error) {
	var features uint32
	if err := CAN_GetValue(channel, PCAN_CHANNEL_FEATURES, uintptr(unsafe.Pointer(&features)), 4); err != nil {
		return 0, err
	}
	return features, nil
}

func GetFirmwareVersion(channel TPCANHandle) (string, error) {
	var version [MAX_LENGTH_VERSION_STRING]byte
	if err := CAN_GetValue(channel, PCAN_FIRMWARE_VERSION, uintptr(unsafe.Pointer(&version[0])), MAX_LENGTH_VERSION_STRING); err != nil {
		return "", err
	}
	return cString(version[:]), nil
}

func GetAttachedChannelsCount() ([]TPCANChannelInformation, error) {
	var count uint32
	if err := CAN_GetValue(PCAN_NONEBUS, PCAN_ATTACHED_CHANNELS_COUNT, uintptr(unsafe.Pointer(&count)), 4); err != nil {
		return nil, err
	}
	channels := make([]TPCANChannelInformation, count)
	if err := CAN_GetValue(PCAN_NONEBUS, PCAN_ATTACHED_CHANNELS, uintptr(unsafe.Pointer(&channels[0])), uint32(unsafe.Sizeof(TPCANChannelInformation{}))*count); err != nil {
		return nil, err
	}
	return channels, nil
}

func Read(channel TPCANHandle) (*TPCANMsg, *TPCANTimestamp, error) {
	var message TPCANMsg
	var timestamp TPCANTimestamp
	if err := CAN_Read(channel, &message, &timestamp); err != nil {
		return nil, nil, err
	}
	return &message, &timestamp, nil
}

func Write(channel TPCANHandle, message *TPCANMsg) error {
	return CAN_Write(channel, message)
}

func FilterMessages(channel TPCANHandle, fromID uint32, toID uint32, mode TPCANMode) error {
	return CAN_FilterMessages(channel, fromID, toID, mode)
}

func GetErrorText(e TPCANStatus) string {
	textBuffer := make([]byte, MAX_LENGTH_VERSION_STRING)
	if err := CAN_GetErrorText(e, 0, &textBuffer[0], MAX_LENGTH_VERSION_STRING); err != nil {
		return "Unknown error"
	}
	return cString(textBuffer[:])
}
