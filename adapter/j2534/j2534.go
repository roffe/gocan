package j2534

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	//
	// J2534-1 v04.04 ProtocolID Values
	//
	J1850VPW     = 0x01
	J1850PWM     = 0x02
	ISO9141      = 0x03
	ISO14230     = 0x04
	CAN          = 0x05
	ISO15765     = 0x06
	SCI_A_ENGINE = 0x07
	SCI_A_TRANS  = 0x08
	SCI_B_ENGINE = 0x09
	SCI_B_TRANS  = 0x0A

	//
	// J2534-2 ProtocolID Values
	//
	J1850VPW_PS           = 0x00008000
	J1850PWM_PS           = 0x00008001
	ISO9141_PS            = 0x00008002
	ISO14230_PS           = 0x00008003
	CAN_PS                = 0x00008004
	ISO15765_PS           = 0x00008005
	J2610_PS              = 0x00008006
	SW_ISO15765_PS        = 0x00008007
	SW_CAN_PS             = 0x00008008
	GM_UART_PS            = 0x00008009
	CAN_CH1               = 0x00009000
	CAN_CH2               = (CAN_CH1 + 1)
	CAN_CH128             = (CAN_CH1 + 127)
	J1850VPW_CH1          = 0x00009080
	J1850VPW_CH2          = (J1850VPW_CH1 + 1)
	J1850VPW_CH128        = (J1850VPW_CH1 + 127)
	J1850PWM_CH1          = 0x00009160
	J1850PWM_CH2          = (J1850PWM_CH1 + 1)
	J1850PWM_CH128        = (J1850PWM_CH1 + 127)
	ISO9141_CH1           = 0x00009240
	ISO9141_CH2           = (ISO9141_CH1 + 1)
	ISO9141_CH128         = (ISO9141_CH1 + 127)
	ISO14230_CH1          = 0x00009320
	ISO14230_CH2          = (ISO14230_CH1 + 1)
	ISO14230_CH128        = (ISO14230_CH1 + 127)
	ISO15765_CH1          = 0x00009400
	ISO15765_CH2          = (ISO15765_CH1 + 1)
	ISO15765_CH128        = (ISO15765_CH1 + 127)
	SW_CAN_CAN_CH1        = 0x00009480
	SW_CAN_CAN_CH2        = (SW_CAN_CAN_CH1 + 1)
	SW_CAN_CAN_CH128      = (SW_CAN_CAN_CH1 + 127)
	SW_CAN_ISO15765_CH1   = 0x00009560
	SW_CAN_ISO15765_CH2   = (SW_CAN_ISO15765_CH1 + 1)
	SW_CAN_ISO15765_CH128 = (SW_CAN_ISO15765_CH1 + 127)
	J2610_CH1             = 0x00009640
	J2610_CH2             = (J2610_CH1 + 1)
	J2610_CH128           = (J2610_CH1 + 127)
	ANALOG_IN_CH1         = 0x0000C000
	ANALOG_IN_CH2         = 0x0000C001
	ANALOG_IN_CH32        = 0x0000C01F

	//
	// J2534-1 v04.04 Error Values
	//
	STATUS_NOERROR           = 0x00 // Function call successful.
	ERR_NOT_SUPPORTED        = 0x01 // Device cannot support requested functionality mandated in J2534. Device is not fully SAE J2534 compliant.
	ERR_INVALID_CHANNEL_ID   = 0x02 // Invalid ChannelID value.
	ERR_INVALID_PROTOCOL_ID  = 0x03 // Invalid or unsupported ProtocolID, or there is a resource conflict (i.e. trying to connect to multiple mutually exclusive protocols such as J1850PWM and J1850VPW, or CAN and SCI, etc.).
	ERR_NULL_PARAMETER       = 0x04 // NULL pointer supplied where a valid pointer is required.
	ERR_INVALID_IOCTL_VALUE  = 0x05 // Invalid value for Ioctl parameter.
	ERR_INVALID_FLAGS        = 0x06 // Invalid flag values.
	ERR_FAILED               = 0x07 // Undefined error, use PassThruGetLastError() for text description.
	ERR_DEVICE_NOT_CONNECTED = 0x08 // Unable to communicate with device.
	ERR_TIMEOUT              = 0x09 // Read or write timeout:
	// PassThruReadMsgs() - No message available to read or could not read the specified number of messages. The actual number of messages read is placed in <NumMsgs>.
	// PassThruWriteMsgs() - Device could not write the specified number of messages. The actual number of messages sent on the vehicle network is placed in <NumMsgs>.
	ERR_INVALID_MSG           = 0x0A // Invalid message structure pointed to by pMsg.
	ERR_INVALID_TIME_INTERVAL = 0x0B // Invalid TimeInterval value.
	ERR_EXCEEDED_LIMIT        = 0x0C // Exceeded maximum number of message IDs or allocated space.
	ERR_INVALID_MSG_ID        = 0x0D // Invalid MsgID value.
	ERR_DEVICE_IN_USE         = 0x0E // Device is currently open.
	ERR_INVALID_IOCTL_ID      = 0x0F // Invalid IoctlID value.
	ERR_BUFFER_EMPTY          = 0x10 // Protocol message buffer empty, no messages available to read.
	ERR_BUFFER_FULL           = 0x11 // Protocol message buffer full. All the messages specified may not have been transmitted.
	ERR_BUFFER_OVERFLOW       = 0x12 // Indicates a buffer overflow occurred and messages were lost.
	ERR_PIN_INVALID           = 0x13 // Invalid pin number, pin number already in use, or voltage already applied to a different pin.
	ERR_CHANNEL_IN_USE        = 0x14 // Channel number is currently connected.
	ERR_MSG_PROTOCOL_ID       = 0x15 // Protocol type in the message does not match the protocol associated with the Channel ID
	ERR_INVALID_FILTER_ID     = 0x16 // Invalid Filter ID value.
	ERR_NO_FLOW_CONTROL       = 0x17 // No flow control filter set or matched (for ProtocolID ISO15765 only).
	ERR_NOT_UNIQUE            = 0x18 // A CAN ID in pPatternMsg or pFlowControlMsg matches either ID in an existing FLOW_CONTROL_FILTER
	ERR_INVALID_BAUDRATE      = 0x19 // The desired baud rate cannot be achieved within the tolerance specified in SAE J2534-1 Section 6.5
	ERR_INVALID_DEVICE_ID     = 0x1A // Device ID invalid.

	//
	// J2534-1 v04.04 Connect Flags
	//
	CAN_29BIT_ID        = 0x0100
	ISO9141_NO_CHECKSUM = 0x0200
	CAN_ID_BOTH         = 0x0800
	ISO9141_K_LINE_ONLY = 0x1000

	//
	// J2534-1 v04.04 Filter Type Values
	//
	PASS_FILTER         = 0x00000001
	BLOCK_FILTER        = 0x00000002
	FLOW_CONTROL_FILTER = 0x00000003

	//
	// J2534-1 v04.04 Programming Voltage Pin Numbers
	//
	AUXILIARY_OUTPUT_PIN       = 0
	SAE_J1962_CONNECTOR_PIN_6  = 6
	SAE_J1962_CONNECTOR_PIN_9  = 9
	SAE_J1962_CONNECTOR_PIN_11 = 11
	SAE_J1962_CONNECTOR_PIN_12 = 12
	SAE_J1962_CONNECTOR_PIN_13 = 13
	SAE_J1962_CONNECTOR_PIN_14 = 14
	SAE_J1962_CONNECTOR_PIN_15 = 15 // Short to ground only

	//
	// J2534-1 v04.04 Programming Voltage Values
	//
	SHORT_TO_GROUND = 0xFFFFFFFE
	VOLTAGE_OFF     = 0xFFFFFFFF

	//
	// J2534-1 v04.04 API Version Values
	//
	J2534_APIVER_FEBRUARY_2002 = "02.02"
	J2534_APIVER_NOVEMBER_2004 = "04.04"

	//
	// J2534-1 v04.04 IOCTL ID Values
	//
	GET_CONFIG                         = 0x01 // pInput = SCONFIG_LIST, pOutput = NULL
	SET_CONFIG                         = 0x02 // pInput = SCONFIG_LIST, pOutput = NULL
	READ_VBATT                         = 0x03 // pInput = NULL, pOutput = unsigned long
	FIVE_BAUD_INIT                     = 0x04 // pInput = SBYTE_ARRAY, pOutput = SBYTE_ARRAY
	FAST_INIT                          = 0x05 // pInput = PASSTHRU_MSG, pOutput = PASSTHRU_MSG
	CLEAR_TX_BUFFER                    = 0x07 // pInput = NULL, pOutput = NULL
	CLEAR_RX_BUFFER                    = 0x08 // pInput = NULL, pOutput = NULL
	CLEAR_PERIODIC_MSGS                = 0x09 // pInput = NULL, pOutput = NULL
	CLEAR_MSG_FILTERS                  = 0x0A // pInput = NULL, pOutput = NULL
	CLEAR_FUNCT_MSG_LOOKUP_TABLE       = 0x0B // pInput = NULL, pOutput = NULL
	ADD_TO_FUNCT_MSG_LOOKUP_TABLE      = 0x0C // pInput = SBYTE_ARRAY, pOutput = NULL
	DELETE_FROM_FUNCT_MSG_LOOKUP_TABLE = 0x0D // pInput = SBYTE_ARRAY, pOutput = NULL
	READ_PROG_VOLTAGE                  = 0x0E // pInput = NULL, pOutput = unsigned long

	//
	// J2534-2 IOCTL ID Values
	//
	SW_CAN_HS         = 0x00008000 // pInput = NULL, pOutput = NULL
	SW_CAN_NS         = 0x00008001 // pInput = NULL, pOutput = NULL
	SET_POLL_RESPONSE = 0x00008002 // pInput = SBYTE_ARRAY, pOutput = NULL
	BECOME_MASTER     = 0x00008003 // pInput = unsigned char, pOutput = NULL

	//
	// J2534-1 v04.04 Configuration Parameter Values
	// Default value is enclosed in square brackets "[" and "]"
	//
	DATA_RATE         = 0x01 // 5-500000
	LOOPBACK          = 0x03 // 0 (OFF), 1 (ON) [0]
	NODE_ADDRESS      = 0x04 // J1850PWM: 0x00-0xFF
	NETWORK_LINE      = 0x05 // J1850PWM: 0 (BUS_NORMAL), 1 (BUS_PLUS), 2 (BUS_MINUS) [0]
	P1_MIN            = 0x06 // ISO9141 or ISO14230: Not used by interface
	P1_MAX            = 0x07 // ISO9141 or ISO14230: 0x1-0xFFFF (.5 ms per bit) [40 (20ms)]
	P2_MIN            = 0x08 // ISO9141 or ISO14230: Not used by interface
	P2_MAX            = 0x09 // ISO9141 or ISO14230: Not used by interface
	P3_MIN            = 0x0A // ISO9141 or ISO14230: 0x0-0xFFFF (.5 ms per bit) [110 (55ms)]
	P3_MAX            = 0x0B // ISO9141 or ISO14230: Not used by interface
	P4_MIN            = 0x0C // ISO9141 or ISO14230: 0x0-0xFFFF (.5 ms per bit) [10 (5ms)]
	P4_MAX            = 0x0D // ISO9141 or ISO14230: Not used by interface
	W0                = 0x19 // ISO9141: 0x0-0xFFFF (1 ms per bit) [300]
	W1                = 0x0E // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [300]
	W2                = 0x0F // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [20]
	W3                = 0x10 // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [20]
	W4                = 0x11 // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [50]
	W5                = 0x12 // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [300]
	TIDLE             = 0x13 // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [300]
	TINIL             = 0x14 // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [25]
	TWUP              = 0x15 // ISO9141 or ISO14230: 0x0-0xFFFF (1 ms per bit) [50]
	PARITY            = 0x16 // ISO9141 or ISO14230: 0 (NO_PARITY), 1 (ODD_PARITY), 2 (EVEN_PARITY) [0]
	BIT_SAMPLE_POINT  = 0x17 // CAN: 0-100 (1% per bit) [80]
	SYNC_JUMP_WIDTH   = 0x18 // CAN: 0-100 (1% per bit) [15]
	T1_MAX            = 0x1A // SCI: 0x0-0xFFFF (1 ms per bit) [20]
	T2_MAX            = 0x1B // SCI: 0x0-0xFFFF (1 ms per bit) [100]
	T3_MAX            = 0x24 // SCI: 0x0-0xFFFF (1 ms per bit) [50]
	T4_MAX            = 0x1C // SCI: 0x0-0xFFFF (1 ms per bit) [20]
	T5_MAX            = 0x1D // SCI: 0x0-0xFFFF (1 ms per bit) [100]
	ISO15765_BS       = 0x1E // ISO15765: 0x0-0xFF [0]
	ISO15765_STMIN    = 0x1F // ISO15765: 0x0-0xFF [0]
	ISO15765_BS_TX    = 0x22 // ISO15765: 0x0-0xFF,0xFFFF [0xFFFF]
	ISO15765_STMIN_TX = 0x23 // ISO15765: 0x0-0xFF,0xFFFF [0xFFFF]
	DATA_BITS         = 0x20 // ISO9141 or ISO14230: 0 (8 data bits), 1 (7 data bits) [0]
	FIVE_BAUD_MOD     = 0x21 // ISO9141 or ISO14230: 0 (ISO 9141-2/14230-4), 1 (Inv KB2), 2 (Inv Addr), 3 (ISO 9141) [0]
	ISO15765_WFT_MAX  = 0x25 // ISO15765: 0x0-0xFF [0]

	//
	// J2534-2 Configuration Parameter Values
	// Default value is enclosed in square brackets "[" and "]"
	//
	CAN_MIXED_FORMAT          = 0x00008000 // See #defines below. [0]
	J1962_PINS                = 0x00008001 // 0xPPSS PP: 0x00-0x10 SS: 0x00-0x10 PP!=SS, except 0x0000. Exclude pins 4, 5, and 16. [0]
	SW_CAN_HS_DATA_RATE       = 0x00008010 // SWCAN: 5-500000 [83333]
	SW_CAN_SPEEDCHANGE_ENABLE = 0x00008011 // SWCAN: 0 (DISABLE_SPDCHANGE), 1 (ENABLE_SPDCHANGE) [0]
	SW_CAN_RES_SWITCH         = 0x00008012 // SWCAN: 0 (DISCONNECT_RESISTOR), 1 (CONNECT_RESISTOR), 2 (AUTO_ RESISTOR) [0]
	ACTIVE_CHANNELS           = 0x00008020 // ANALOG: 0-0xFFFFFFFF
	SAMPLE_RATE               = 0x00008021 // ANALOG: 0-0xFFFFFFFF [0] (high bit changes meaning from samples/sec to seconds/sample)
	SAMPLES_PER_READING       = 0x00008022 // ANALOG: 1-0xFFFFFFFF [1]
	READINGS_PER_MSG          = 0x00008023 // ANALOG: 1-0x00000408 (1 - 1032) [1]
	AVERAGING_METHOD          = 0x00008024 // ANALOG: 0-0xFFFFFFFF [0]
	SAMPLE_RESOLUTION         = 0x00008025 // ANALOG READ-ONLY: 0x1-0x20 (1 - 32)
	INPUT_RANGE_LOW           = 0x00008026 // ANALOG READ-ONLY: 0x80000000-0x7FFFFFFF (-2147483648-2147483647)
	INPUT_RANGE_HIGH          = 0x00008027 // ANALOG READ-ONLY: 0x80000000-0x7FFFFFFF (-2147483648-2147483647)

	//
	// J2534-2 Mixed-Mode/Format CAN Definitions
	//
	CAN_MIXED_FORMAT_OFF        = 0 // Messages will be treated as ISO 15765 ONLY.
	CAN_MIXED_FORMAT_ON         = 1 // Messages will be treated as either ISO 15765 or an unformatted CAN frame.
	CAN_MIXED_FORMAT_ALL_FRAMES = 2 // Messages will be treated as ISO 15765, an unformatted CAN frame, or both.

	//
	// J2534-2 Analog Channel Averaging Method Definitions
	//
	SIMPLE_AVERAGE    = 0x00000000 // Simple arithmetic mean
	MAX_LIMIT_AVERAGE = 0x00000001 // Choose the biggest value
	MIN_LIMIT_AVERAGE = 0x00000002 // Choose the lowest value
	MEDIAN_AVERAGE    = 0x00000003 // Choose arithmetic median

	//
	// J2534-1 v04.04 RxStatus Definitions
	//
	TX_MSG_TYPE            = 0x0001
	START_OF_MESSAGE       = 0x0002
	RX_BREAK               = 0x0004
	TX_INDICATION          = 0x0008
	ISO15765_PADDING_ERROR = 0x0010
	ISO15765_ADDR_TYPE     = 0x0080
	// CAN_29BIT_ID				0x0100		// Defined above

	//
	// J2534-2 RxStatus Definitions
	//
	SW_CAN_HV_RX = 0x00010000 // SWCAN Channels Only
	SW_CAN_HS_RX = 0x00020000 // SWCAN Channels Only
	SW_CAN_NS_RX = 0x00040000 // SWCAN Channels Only
	OVERFLOW_    = 0x00010000 // Analog Input Channels Only

	//
	// J2534-1 v04.04 TxFlags Definitions
	//
	ISO15765_FRAME_PAD = 0x0040
	// ISO15765_ADDR_TYPE		0x0080		// Defined above
	// CAN_29BIT_ID				0x0100		// Defined above
	WAIT_P3_MIN_ONLY = 0x0200
	SCI_MODE         = 0x400000
	SCI_TX_VOLTAGE   = 0x800000

	//
	// J2534-2 TxFlags Definitions
	//
	SW_CAN_HV_TX = 0x00000400
)

type SCONFIG struct {
	Parameter uint32 // Name of parameter
	Value     uint32 // Value of the parameter
}

type SCONFIG_LIST struct {
	NumOfParams uint32   // Number of SCONFIG elements
	ConfigPtr   *SCONFIG // Array of SCONFIG
}

type SBYTE_ARRAY struct {
	NumOfBytes uint32 // Number of bytes in the array
	BytePtr    *byte  // Array of bytes
}

type PASSTHRU_MSG struct {
	// Protocol type
	ProtocolID uint32 // Protocol ID

	// Receive message status. See RxStatus in "Message Flags and Status Definition" section
	RxStatus uint32 // RxStatus

	// Transmit message flags
	TxFlags uint32 // TxFlags

	// Received message timestamp (microseconds): For the START_OF_FRAME
	// indication, the timestamp is for the start of the first bit of the message. For all other
	// indications and transmit and receive messages, the timestamp is the end of the last
	// bit of the message. For all other error indications, the timestamp is the time the error
	// is detected.
	Timestamp uint32 // Timestamp

	// Data size in bytes, including header bytes, ID bytes, message data bytes, and extra
	// data, if any.
	DataSize uint32 // Number of bytes in the data array

	// Start position of extra data in received message (for example, IFR). The extra data
	// bytes follow the body bytes in the Data array. The index is zero-based. When no
	// extra data bytes are present in the message, ExtraDataIndex shall be set equal to
	// DataSize. Therefore, if DataSize equals ExtraDataIndex, there are no extra data
	// bytes. If ExtraDataIndex=0, then all bytes in the data array are extra bytes.
	ExtraDataIndex uint32 // Index to start of extra data

	// Start position of extra data in received message (for example, IFR). The extra data
	// bytes follow the body bytes in the Data array. The index is zero-based. When no
	// extra data bytes are present in the message, ExtraDataIndex shall be set equal to
	// DataSize. Therefore, if DataSize equals ExtraDataIndex, there are no extra data
	// bytes. If ExtraDataIndex=0, then all bytes in the data array are extra bytes.
	Data [4128]byte
}

func (m *PASSTHRU_MSG) DataBytes() []byte {
	return m.Data[:m.DataSize]
}

func (m *PASSTHRU_MSG) String() string {
	return fmt.Sprintf("ProtocolID: %d RxStatus: %02X TxFlags: %02X Timestamp: %d DataSize: %d ExtraDataIndex: %d Data: %X", m.ProtocolID, m.RxStatus, m.TxFlags, m.Timestamp, m.DataSize, m.ExtraDataIndex, m.DataBytes())
}

type J2534PassThru struct {
	dll                     *syscall.LazyDLL
	passThruReadVersionProc *syscall.LazyProc
	passThruOpen            *syscall.LazyProc
	passThruClose           *syscall.LazyProc
	passThruConnect         *syscall.LazyProc
	passThruDisconnect      *syscall.LazyProc
	passThruReadMsgs        *syscall.LazyProc
	passThruWriteMsgs       *syscall.LazyProc
	passThruStartMsgFilter  *syscall.LazyProc
	passThruIoctl           *syscall.LazyProc
	passThruGetLastError    *syscall.LazyProc
}

func NewJ2534(dllName string) *J2534PassThru {
	dll := syscall.NewLazyDLL(dllName)

	return &J2534PassThru{
		dll:                     dll,
		passThruReadVersionProc: dll.NewProc("PassThruReadVersion"),
		passThruOpen:            dll.NewProc("PassThruOpen"),
		passThruClose:           dll.NewProc("PassThruClose"),
		passThruConnect:         dll.NewProc("PassThruConnect"),
		passThruDisconnect:      dll.NewProc("PassThruDisconnect"),
		passThruReadMsgs:        dll.NewProc("PassThruReadMsgs"),
		passThruWriteMsgs:       dll.NewProc("PassThruWriteMsgs"),
		passThruStartMsgFilter:  dll.NewProc("PassThruStartMsgFilter"),
		passThruIoctl:           dll.NewProc("PassThruIoctl"),
		passThruGetLastError:    dll.NewProc("PassThruGetLastError"),
	}
}

func (j *J2534PassThru) PassThruConnect(deviceID uint32, protocolID uint32, flags uint32, baudRate uint32, pChannelID *uint32) error {
	// long PassThruConnect(unsigned long DeviceID, unsigned long ProtocolID, unsigned long Flags, unsigned long BaudRate, unsigned long *pChannelID);
	ret, _, _ := j.passThruConnect.Call(
		uintptr(deviceID),
		uintptr(protocolID),
		uintptr(flags),
		uintptr(baudRate),
		uintptr(unsafe.Pointer(pChannelID)),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruDisconnect(channelID uint32) error {
	// long PassThruDisconnect(unsigned long ChannelID);
	ret, _, _ := j.passThruDisconnect.Call(
		uintptr(channelID),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruClose(deviceID uint32) error {
	// long PassThruClose(unsigned long DeviceID);
	ret, _, _ := j.passThruClose.Call(
		uintptr(deviceID),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruOpen(deviceName string, pDeviceID *uint32) error {
	var pName *string
	if deviceName != "" {
		pName = &deviceName
	}
	// long PassThruOpen(void* pName, unsigned long *pDeviceID);
	ret, _, _ := j.passThruOpen.Call(
		uintptr(unsafe.Pointer(pName)),
		uintptr(unsafe.Pointer(pDeviceID)),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruReadMsgs(channelID uint32, pMsg uintptr, pNumMsgs uint32, timeout uint32) error {
	// long PassThruReadMsgs(unsigned long ChannelID, PASSTHRU_MSG *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
	ret, _, _ := j.passThruReadMsgs.Call(
		uintptr(channelID),
		pMsg,
		uintptr(unsafe.Pointer(&pNumMsgs)),
		uintptr(timeout),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruWriteMsgs(channelID uint32, pMsg uintptr, pNumMsgs uint32, timeout uint32) error {
	// long PassThruWriteMsgs(unsigned long ChannelID, PASSTHRU_MSG *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
	ret, _, _ := j.passThruWriteMsgs.Call(
		uintptr(channelID),
		pMsg,
		uintptr(unsafe.Pointer(&pNumMsgs)),
		uintptr(timeout),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruStartMsgFilter(channelID uint32, filterType uint32, pMaskMsg, pPatternMsg, pFlowControlMsg *PASSTHRU_MSG, pMsgID *uint32) error {
	// long PassThruStartMsgFilter(unsigned long ChannelID, unsigned long FilterType, PASSTHRU_MSG *pMaskMsg, PASSTHRU_MSG *pPatternMsg, PASSTHRU_MSG *pFlowControlMsg, unsigned long *pMsgID);
	ret, _, _ := j.passThruStartMsgFilter.Call(
		uintptr(channelID),
		uintptr(filterType),
		uintptr(unsafe.Pointer(pMaskMsg)),
		uintptr(unsafe.Pointer(pPatternMsg)),
		uintptr(unsafe.Pointer(pFlowControlMsg)),
		uintptr(unsafe.Pointer(pMsgID)),
	)
	return CheckError(ret)
}

func (j *J2534PassThru) PassThruReadVersion(deviceID uint32) (string, string, string, error) {
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

	return string(pFirmwareVersion[:]),
		string(pDllVersion[:]),
		string(pApiVersion[:]),
		CheckError(ret)
}

// long PassThruIoctl(unsigned long HandleID, unsigned long IoctlID, void *pInput, void *pOutput);
func (j *J2534PassThru) PassThruIoctl(handleID uint32, ioctlID uint32, pInput uintptr, pOutput uintptr) error {
	ret, _, _ := j.passThruIoctl.Call(
		uintptr(handleID),
		uintptr(ioctlID),
		pInput,
		pOutput,
	)
	return CheckError(ret)
}

// long PassThruIoctl(unsigned long HandleID, unsigned long IoctlID, void *pInput, void *pOutput);
func (j *J2534PassThru) PassThruIoctlS(handleID uint32, ioctlID uint32, pInput uintptr) error {
	ret, _, _ := j.passThruIoctl.Call(
		uintptr(handleID),
		uintptr(ioctlID),
		pInput,
		uintptr(0),
	)
	return CheckError(ret)
}

// long PassThruGetLastError(char *pErrorDescription);
func (j *J2534PassThru) PassThruGetLastError() (string, error) {
	var pErrorDescription [80]byte

	ret, _, _ := j.passThruGetLastError.Call(
		uintptr(unsafe.Pointer(&pErrorDescription)),
	)

	return string(pErrorDescription[:]), CheckError(ret)
}
