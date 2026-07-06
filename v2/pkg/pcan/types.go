package pcan

//  PCAN-Basic API
//
//  ~~~~~~~~~~~~
//
//  Copyright (C) 1999-2024  PEAK-System Technik GmbH, Darmstadt
//  more Info at http://www.peak-system.com

// -----------------------------------------------------------------------------
// Base type mappings (C → Go)
// -----------------------------------------------------------------------------

// C: BYTE   -> uint8
// C: WORD   -> uint16
// C: DWORD  -> uint32
// C: UINT64 -> uint64
// C: LPSTR  -> char* (C string)
// We'll model LPSTR as uintptr for now (pointer-sized). You can change to *C.char in cgo builds.

type BYTE = uint8
type WORD = uint16
type DWORD = uint32
type UINT64 = uint64
type LPSTR = uintptr // or *C.char in a cgo context

// -----------------------------------------------------------------------------
// Type definitions from header typedefs
// -----------------------------------------------------------------------------

type TPCANHandle uint16      // Represents a PCAN hardware channel handle (WORD in C)
type TPCANStatus uint32      // Represents a PCAN status/error code (DWORD in C)
type TPCANParameter uint8    // Represents a PCAN parameter to be read or set (BYTE in C)
type TPCANDevice uint8       // Represents a PCAN device (BYTE in C)
type TPCANMessageType uint8  // Represents the type of a PCAN message (BYTE in C)
type TPCANType uint8         // Represents the type of PCAN hardware to be initialized (BYTE in C)
type TPCANMode uint8         // Represents a PCAN filter mode (BYTE in C)
type TPCANBaudrate uint16    // Represents a PCAN Baud rate register value (WORD in C)
type TPCANBitrateFD uintptr  // Represents a PCAN-FD bit rate string (LPSTR in C)
type TPCANTimestampFD uint64 // Represents a timestamp of a received PCAN FD message (UINT64 in C)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

// Currently defined and supported PCAN channels
const (
	PCAN_NONEBUS TPCANHandle = 0x00 // Undefined/default value for a PCAN bus

	PCAN_ISABUS1 TPCANHandle = 0x21 // PCAN-ISA interface, channel 1
	PCAN_ISABUS2 TPCANHandle = 0x22 // PCAN-ISA interface, channel 2
	PCAN_ISABUS3 TPCANHandle = 0x23 // PCAN-ISA interface, channel 3
	PCAN_ISABUS4 TPCANHandle = 0x24 // PCAN-ISA interface, channel 4
	PCAN_ISABUS5 TPCANHandle = 0x25 // PCAN-ISA interface, channel 5
	PCAN_ISABUS6 TPCANHandle = 0x26 // PCAN-ISA interface, channel 6
	PCAN_ISABUS7 TPCANHandle = 0x27 // PCAN-ISA interface, channel 7
	PCAN_ISABUS8 TPCANHandle = 0x28 // PCAN-ISA interface, channel 8

	PCAN_DNGBUS1 TPCANHandle = 0x31 // PCAN-Dongle/LPT interface, channel 1

	PCAN_PCIBUS1  TPCANHandle = 0x41  // PCAN-PCI interface, channel 1
	PCAN_PCIBUS2  TPCANHandle = 0x42  // PCAN-PCI interface, channel 2
	PCAN_PCIBUS3  TPCANHandle = 0x43  // PCAN-PCI interface, channel 3
	PCAN_PCIBUS4  TPCANHandle = 0x44  // PCAN-PCI interface, channel 4
	PCAN_PCIBUS5  TPCANHandle = 0x45  // PCAN-PCI interface, channel 5
	PCAN_PCIBUS6  TPCANHandle = 0x46  // PCAN-PCI interface, channel 6
	PCAN_PCIBUS7  TPCANHandle = 0x47  // PCAN-PCI interface, channel 7
	PCAN_PCIBUS8  TPCANHandle = 0x48  // PCAN-PCI interface, channel 8
	PCAN_PCIBUS9  TPCANHandle = 0x409 // PCAN-PCI interface, channel 9
	PCAN_PCIBUS10 TPCANHandle = 0x40A // PCAN-PCI interface, channel 10
	PCAN_PCIBUS11 TPCANHandle = 0x40B // PCAN-PCI interface, channel 11
	PCAN_PCIBUS12 TPCANHandle = 0x40C // PCAN-PCI interface, channel 12
	PCAN_PCIBUS13 TPCANHandle = 0x40D // PCAN-PCI interface, channel 13
	PCAN_PCIBUS14 TPCANHandle = 0x40E // PCAN-PCI interface, channel 14
	PCAN_PCIBUS15 TPCANHandle = 0x40F // PCAN-PCI interface, channel 15
	PCAN_PCIBUS16 TPCANHandle = 0x410 // PCAN-PCI interface, channel 16

	PCAN_USBBUS1  TPCANHandle = 0x51  // PCAN-USB interface, channel 1
	PCAN_USBBUS2  TPCANHandle = 0x52  // PCAN-USB interface, channel 2
	PCAN_USBBUS3  TPCANHandle = 0x53  // PCAN-USB interface, channel 3
	PCAN_USBBUS4  TPCANHandle = 0x54  // PCAN-USB interface, channel 4
	PCAN_USBBUS5  TPCANHandle = 0x55  // PCAN-USB interface, channel 5
	PCAN_USBBUS6  TPCANHandle = 0x56  // PCAN-USB interface, channel 6
	PCAN_USBBUS7  TPCANHandle = 0x57  // PCAN-USB interface, channel 7
	PCAN_USBBUS8  TPCANHandle = 0x58  // PCAN-USB interface, channel 8
	PCAN_USBBUS9  TPCANHandle = 0x509 // PCAN-USB interface, channel 9
	PCAN_USBBUS10 TPCANHandle = 0x50A // PCAN-USB interface, channel 10
	PCAN_USBBUS11 TPCANHandle = 0x50B // PCAN-USB interface, channel 11
	PCAN_USBBUS12 TPCANHandle = 0x50C // PCAN-USB interface, channel 12
	PCAN_USBBUS13 TPCANHandle = 0x50D // PCAN-USB interface, channel 13
	PCAN_USBBUS14 TPCANHandle = 0x50E // PCAN-USB interface, channel 14
	PCAN_USBBUS15 TPCANHandle = 0x50F // PCAN-USB interface, channel 15
	PCAN_USBBUS16 TPCANHandle = 0x510 // PCAN-USB interface, channel 16

	PCAN_PCCBUS1 TPCANHandle = 0x61 // PCAN-PC Card interface, channel 1
	PCAN_PCCBUS2 TPCANHandle = 0x62 // PCAN-PC Card interface, channel 2

	PCAN_LANBUS1  TPCANHandle = 0x801 // PCAN-LAN interface, channel 1
	PCAN_LANBUS2  TPCANHandle = 0x802 // PCAN-LAN interface, channel 2
	PCAN_LANBUS3  TPCANHandle = 0x803 // PCAN-LAN interface, channel 3
	PCAN_LANBUS4  TPCANHandle = 0x804 // PCAN-LAN interface, channel 4
	PCAN_LANBUS5  TPCANHandle = 0x805 // PCAN-LAN interface, channel 5
	PCAN_LANBUS6  TPCANHandle = 0x806 // PCAN-LAN interface, channel 6
	PCAN_LANBUS7  TPCANHandle = 0x807 // PCAN-LAN interface, channel 7
	PCAN_LANBUS8  TPCANHandle = 0x808 // PCAN-LAN interface, channel 8
	PCAN_LANBUS9  TPCANHandle = 0x809 // PCAN-LAN interface, channel 9
	PCAN_LANBUS10 TPCANHandle = 0x80A // PCAN-LAN interface, channel 10
	PCAN_LANBUS11 TPCANHandle = 0x80B // PCAN-LAN interface, channel 11
	PCAN_LANBUS12 TPCANHandle = 0x80C // PCAN-LAN interface, channel 12
	PCAN_LANBUS13 TPCANHandle = 0x80D // PCAN-LAN interface, channel 13
	PCAN_LANBUS14 TPCANHandle = 0x80E // PCAN-LAN interface, channel 14
	PCAN_LANBUS15 TPCANHandle = 0x80F // PCAN-LAN interface, channel 15
	PCAN_LANBUS16 TPCANHandle = 0x810 // PCAN-LAN interface, channel 16
)

// PCAN error and status codes
const (
	PCAN_ERROR_OK           TPCANStatus = 0x00000             // No error
	PCAN_ERROR_XMTFULL      TPCANStatus = 0x00001             // Transmit buffer in CAN controller is full
	PCAN_ERROR_OVERRUN      TPCANStatus = 0x00002             // CAN controller was read too late
	PCAN_ERROR_BUSLIGHT     TPCANStatus = 0x00004             // Bus error: an error counter reached the 'light' limit
	PCAN_ERROR_BUSHEAVY     TPCANStatus = 0x00008             // Bus error: an error counter reached the 'heavy' limit
	PCAN_ERROR_BUSWARNING   TPCANStatus = PCAN_ERROR_BUSHEAVY // 'warning' limit alias
	PCAN_ERROR_BUSPASSIVE   TPCANStatus = 0x40000             // Bus error: controller is error passive
	PCAN_ERROR_BUSOFF       TPCANStatus = 0x00010             // Bus error: controller is bus-off
	PCAN_ERROR_ANYBUSERR    TPCANStatus = (PCAN_ERROR_BUSWARNING | PCAN_ERROR_BUSLIGHT | PCAN_ERROR_BUSHEAVY | PCAN_ERROR_BUSOFF | PCAN_ERROR_BUSPASSIVE)
	PCAN_ERROR_QRCVEMPTY    TPCANStatus = 0x00020 // Receive queue empty
	PCAN_ERROR_QOVERRUN     TPCANStatus = 0x00040 // Receive queue read too late
	PCAN_ERROR_QXMTFULL     TPCANStatus = 0x00080 // Transmit queue full
	PCAN_ERROR_REGTEST      TPCANStatus = 0x00100 // Controller register test failed
	PCAN_ERROR_NODRIVER     TPCANStatus = 0x00200 // Driver not loaded
	PCAN_ERROR_HWINUSE      TPCANStatus = 0x00400 // Hardware already in use by a Net
	PCAN_ERROR_NETINUSE     TPCANStatus = 0x00800 // A Client is already connected to the Net
	PCAN_ERROR_ILLHW        TPCANStatus = 0x01400 // Invalid hardware handle
	PCAN_ERROR_ILLNET       TPCANStatus = 0x01800 // Invalid Net handle
	PCAN_ERROR_ILLCLIENT    TPCANStatus = 0x01C00 // Invalid Client handle
	PCAN_ERROR_ILLHANDLE    TPCANStatus = (PCAN_ERROR_ILLHW | PCAN_ERROR_ILLNET | PCAN_ERROR_ILLCLIENT)
	PCAN_ERROR_RESOURCE     TPCANStatus = 0x02000   // Cannot create resource (FIFO, timeout, etc.)
	PCAN_ERROR_ILLPARAMTYPE TPCANStatus = 0x04000   // Invalid parameter
	PCAN_ERROR_ILLPARAMVAL  TPCANStatus = 0x08000   // Invalid parameter value
	PCAN_ERROR_UNKNOWN      TPCANStatus = 0x10000   // Unknown error
	PCAN_ERROR_ILLDATA      TPCANStatus = 0x20000   // Invalid data / function / action
	PCAN_ERROR_ILLMODE      TPCANStatus = 0x80000   // Driver object wrong state for operation
	PCAN_ERROR_CAUTION      TPCANStatus = 0x2000000 // Operation ok but irregularities logged
	PCAN_ERROR_INITIALIZE   TPCANStatus = 0x4000000 // Channel not initialized
	PCAN_ERROR_ILLOPERATION TPCANStatus = 0x8000000 // Invalid operation
)

// PCAN devices
const (
	PCAN_NONE    TPCANDevice = 0x00 // Undefined / unknown / not selected
	PCAN_PEAKCAN TPCANDevice = 0x01 // Non-PnP (not used in PCAN-Basic)
	PCAN_ISA     TPCANDevice = 0x02 // PCAN-ISA, PC/104, PC/104-Plus
	PCAN_DNG     TPCANDevice = 0x03 // PCAN-Dongle
	PCAN_PCI     TPCANDevice = 0x04 // PCI, cPCI, miniPCI, PCIe
	PCAN_USB     TPCANDevice = 0x05 // PCAN-USB, PCAN-USB Pro
	PCAN_PCC     TPCANDevice = 0x06 // PCAN-PC Card
	PCAN_VIRTUAL TPCANDevice = 0x07 // Virtual hardware (not used in Basic)
	PCAN_LAN     TPCANDevice = 0x08 // PCAN Gateway devices
)

// PCAN parameters (for CAN_GetValue / CAN_SetValue)
const (
	PCAN_DEVICE_ID                TPCANParameter = 0x01 // Device identifier parameter
	PCAN_5VOLTS_POWER             TPCANParameter = 0x02 // 5-Volt power parameter
	PCAN_RECEIVE_EVENT            TPCANParameter = 0x03 // Receive event handler parameter
	PCAN_MESSAGE_FILTER           TPCANParameter = 0x04 // Message filter parameter
	PCAN_API_VERSION              TPCANParameter = 0x05 // PCAN-Basic API version
	PCAN_CHANNEL_VERSION          TPCANParameter = 0x06 // PCAN device channel version
	PCAN_BUSOFF_AUTORESET         TPCANParameter = 0x07 // Reset-On-Busoff parameter
	PCAN_LISTEN_ONLY              TPCANParameter = 0x08 // Listen-Only parameter
	PCAN_LOG_LOCATION             TPCANParameter = 0x09 // Directory path for log files
	PCAN_LOG_STATUS               TPCANParameter = 0x0A // Debug-Log activation status
	PCAN_LOG_CONFIGURE            TPCANParameter = 0x0B // Logged info mask (LOG_FUNCTION_*)
	PCAN_LOG_TEXT                 TPCANParameter = 0x0C // Custom text into log
	PCAN_CHANNEL_CONDITION        TPCANParameter = 0x0D // Availability status of a channel
	PCAN_HARDWARE_NAME            TPCANParameter = 0x0E // Hardware name
	PCAN_RECEIVE_STATUS           TPCANParameter = 0x0F // Message reception status
	PCAN_CONTROLLER_NUMBER        TPCANParameter = 0x10 // CAN-Controller index
	PCAN_TRACE_LOCATION           TPCANParameter = 0x11 // Directory path for trace files
	PCAN_TRACE_STATUS             TPCANParameter = 0x12 // Tracing activation status
	PCAN_TRACE_SIZE               TPCANParameter = 0x13 // Max trace file size
	PCAN_TRACE_CONFIGURE          TPCANParameter = 0x14 // Trace storing mode (TRACE_FILE_*)
	PCAN_CHANNEL_IDENTIFYING      TPCANParameter = 0x15 // Blink LED to identify channel
	PCAN_CHANNEL_FEATURES         TPCANParameter = 0x16 // Device capability flags (FEATURE_*)
	PCAN_BITRATE_ADAPTING         TPCANParameter = 0x17 // Using existing bitrate (PCAN-View attached)
	PCAN_BITRATE_INFO             TPCANParameter = 0x18 // Nominal bitrate as Btr0Btr1
	PCAN_BITRATE_INFO_FD          TPCANParameter = 0x19 // BitrateFD string
	PCAN_BUSSPEED_NOMINAL         TPCANParameter = 0x1A // Nominal bus speed in bit/s
	PCAN_BUSSPEED_DATA            TPCANParameter = 0x1B // Data phase bus speed in bit/s
	PCAN_IP_ADDRESS               TPCANParameter = 0x1C // Remote IPv4 of LAN channel
	PCAN_LAN_SERVICE_STATUS       TPCANParameter = 0x1D // Status of Virtual PCAN-Gateway Service
	PCAN_ALLOW_STATUS_FRAMES      TPCANParameter = 0x1E // Receive status frames?
	PCAN_ALLOW_RTR_FRAMES         TPCANParameter = 0x1F // Receive RTR frames?
	PCAN_ALLOW_ERROR_FRAMES       TPCANParameter = 0x20 // Receive error frames?
	PCAN_INTERFRAME_DELAY         TPCANParameter = 0x21 // Delay (µs) between sent frames
	PCAN_ACCEPTANCE_FILTER_11BIT  TPCANParameter = 0x22 // Acceptance filter for 11-bit IDs
	PCAN_ACCEPTANCE_FILTER_29BIT  TPCANParameter = 0x23 // Acceptance filter for 29-bit IDs
	PCAN_IO_DIGITAL_CONFIGURATION TPCANParameter = 0x24 // Digital I/O mode bitmap
	PCAN_IO_DIGITAL_VALUE         TPCANParameter = 0x25 // Digital I/O value bitmap
	PCAN_IO_DIGITAL_SET           TPCANParameter = 0x26 // Set multiple digital I/O pins =1
	PCAN_IO_DIGITAL_CLEAR         TPCANParameter = 0x27 // Clear multiple digital I/O pins =0
	PCAN_IO_ANALOG_VALUE          TPCANParameter = 0x28 // Read analog input pin
	PCAN_FIRMWARE_VERSION         TPCANParameter = 0x29 // Firmware version
	PCAN_ATTACHED_CHANNELS_COUNT  TPCANParameter = 0x2A // Count of attached PCAN channels
	PCAN_ATTACHED_CHANNELS        TPCANParameter = 0x2B // Info about attached channels
	PCAN_ALLOW_ECHO_FRAMES        TPCANParameter = 0x2C // Receive echo frames?
	PCAN_DEVICE_PART_NUMBER       TPCANParameter = 0x2D // Device part number
	PCAN_HARD_RESET_STATUS        TPCANParameter = 0x2E // Hard reset status via CAN_Reset
	PCAN_LAN_CHANNEL_DIRECTION    TPCANParameter = 0x2F // Direction for LAN channels
	PCAN_DEVICE_GUID              TPCANParameter = 0x30 // Global unique device GUID
)

// Deprecated alias
const PCAN_DEVICE_NUMBER = PCAN_DEVICE_ID // Deprecated. Use PCAN_DEVICE_ID.

// PCAN parameter values / misc value masks
const (
	PCAN_PARAMETER_OFF       = 0x00                                             // Parameter inactive
	PCAN_PARAMETER_ON        = 0x01                                             // Parameter active
	PCAN_FILTER_CLOSE        = 0x00                                             // Filter closed: receive nothing
	PCAN_FILTER_OPEN         = 0x01                                             // Filter open: receive all
	PCAN_FILTER_CUSTOM       = 0x02                                             // Filter custom: only registered IDs
	PCAN_CHANNEL_UNAVAILABLE = 0x00                                             // Channel handle invalid/not available
	PCAN_CHANNEL_AVAILABLE   = 0x01                                             // Channel handle is available
	PCAN_CHANNEL_OCCUPIED    = 0x02                                             // Channel already in use
	PCAN_CHANNEL_PCANVIEW    = (PCAN_CHANNEL_AVAILABLE | PCAN_CHANNEL_OCCUPIED) // In use by PCAN-View but still connectable

	LOG_FUNCTION_DEFAULT    = 0x00   // Log system exceptions / errors
	LOG_FUNCTION_ENTRY      = 0x01   // Log entry to API functions
	LOG_FUNCTION_PARAMETERS = 0x02   // Log function parameters
	LOG_FUNCTION_LEAVE      = 0x04   // Log exit from API functions
	LOG_FUNCTION_WRITE      = 0x08   // Log CAN messages in CAN_Write
	LOG_FUNCTION_READ       = 0x10   // Log CAN messages in CAN_Read
	LOG_FUNCTION_ALL        = 0xFFFF // Log everything

	TRACE_FILE_SINGLE      = 0x00  // Single file until size limit
	TRACE_FILE_SEGMENTED   = 0x01  // Rotate through multiple files of max size
	TRACE_FILE_DATE        = 0x02  // Include date in filename
	TRACE_FILE_TIME        = 0x04  // Include start time in filename
	TRACE_FILE_OVERWRITE   = 0x80  // Overwrite existing trace files
	TRACE_FILE_DATA_LENGTH = 0x100 // Use data length column ('l') instead of DLC ('L')

	FEATURE_FD_CAPABLE    = 0x01 // Device supports CAN FD
	FEATURE_DELAY_CAPABLE = 0x02 // Device supports TX delay between frames
	FEATURE_IO_CAPABLE    = 0x04 // Device has I/O pins (USB-Chip devices)

	SERVICE_STATUS_STOPPED = 0x01 // Service not running
	SERVICE_STATUS_RUNNING = 0x04 // Service running

	LAN_DIRECTION_READ       = 0x01          // Incoming only
	LAN_DIRECTION_WRITE      = 0x02          // Outgoing only
	LAN_DIRECTION_READ_WRITE = (0x01 | 0x02) // Bidirectional

	MAX_LENGTH_HARDWARE_NAME  = 33  // 32 chars + terminator
	MAX_LENGTH_VERSION_STRING = 256 // 255 chars + terminator
)

// PCAN message types
const (
	PCAN_MESSAGE_STANDARD TPCANMessageType = 0x00 // 11-bit CAN frame
	PCAN_MESSAGE_RTR      TPCANMessageType = 0x01 // Remote-Transfer-Request
	PCAN_MESSAGE_EXTENDED TPCANMessageType = 0x02 // 29-bit CAN frame
	PCAN_MESSAGE_FD       TPCANMessageType = 0x04 // CAN FD frame
	PCAN_MESSAGE_BRS      TPCANMessageType = 0x08 // Bit rate switch (FD data at higher bitrate)
	PCAN_MESSAGE_ESI      TPCANMessageType = 0x10 // Error state indicator
	PCAN_MESSAGE_ECHO     TPCANMessageType = 0x20 // Echo frame
	PCAN_MESSAGE_ERRFRAME TPCANMessageType = 0x40 // Error frame
	PCAN_MESSAGE_STATUS   TPCANMessageType = 0x80 // PCAN status message
)

// Lookup parameter keys (originally TCHAR strings)
const (
	LOOKUP_DEVICE_TYPE       = "devicetype"       // by device type (e.g. PCAN_USB)
	LOOKUP_DEVICE_ID         = "deviceid"         // by device id
	LOOKUP_CONTROLLER_NUMBER = "controllernumber" // by 0-based CAN controller index
	LOOKUP_IP_ADDRESS        = "ipaddress"        // by IP address (LAN only)
	LOOKUP_DEVICE_GUID       = "deviceguid"       // by device GUID (USB only)
)

// Frame Type / Initialization Mode (aliases)
const (
	PCAN_MODE_STANDARD TPCANMode = TPCANMode(PCAN_MESSAGE_STANDARD)
	PCAN_MODE_EXTENDED TPCANMode = TPCANMode(PCAN_MESSAGE_EXTENDED)
)

// Classical CAN bitrates (BTR0/BTR1)
const (
	PCAN_BAUD_1M   TPCANBaudrate = 0x0014 //   1 MBit/s
	PCAN_BAUD_800K TPCANBaudrate = 0x0016 // 800 kBit/s
	PCAN_BAUD_500K TPCANBaudrate = 0x001C // 500 kBit/s
	PCAN_BAUD_250K TPCANBaudrate = 0x011C // 250 kBit/s
	PCAN_BAUD_125K TPCANBaudrate = 0x031C // 125 kBit/s
	PCAN_BAUD_100K TPCANBaudrate = 0x432F // 100 kBit/s
	PCAN_BAUD_95K  TPCANBaudrate = 0xC34E //  95.238 kBit/s
	PCAN_BAUD_83K  TPCANBaudrate = 0x852B //  83.333 kBit/s
	PCAN_BAUD_50K  TPCANBaudrate = 0x472F //  50 kBit/s
	PCAN_BAUD_47K  TPCANBaudrate = 0x1414 //  47.619 kBit/s
	PCAN_BAUD_33K  TPCANBaudrate = 0x8B2F //  33.333 kBit/s
	PCAN_BAUD_20K  TPCANBaudrate = 0x532F //  20 kBit/s
	PCAN_BAUD_10K  TPCANBaudrate = 0x672F //  10 kBit/s
	PCAN_BAUD_5K   TPCANBaudrate = 0x7F7F //   5 kBit/s
)

// FD bitrate string parameter names
const (
	PCAN_BR_CLOCK       = "f_clock"
	PCAN_BR_CLOCK_MHZ   = "f_clock_mhz"
	PCAN_BR_NOM_BRP     = "nom_brp"
	PCAN_BR_NOM_TSEG1   = "nom_tseg1"
	PCAN_BR_NOM_TSEG2   = "nom_tseg2"
	PCAN_BR_NOM_SJW     = "nom_sjw"
	PCAN_BR_NOM_SAMPLE  = "nom_sam"
	PCAN_BR_DATA_BRP    = "data_brp"
	PCAN_BR_DATA_TSEG1  = "data_tseg1"
	PCAN_BR_DATA_TSEG2  = "data_tseg2"
	PCAN_BR_DATA_SJW    = "data_sjw"
	PCAN_BR_DATA_SAMPLE = "data_ssp_offset"
)

// Type of PCAN (Non-PnP) hardware
const (
	PCAN_TYPE_ISA         TPCANType = 0x01 // PCAN-ISA 82C200
	PCAN_TYPE_ISA_SJA     TPCANType = 0x09 // PCAN-ISA SJA1000
	PCAN_TYPE_ISA_PHYTEC  TPCANType = 0x04 // PHYTEC ISA
	PCAN_TYPE_DNG         TPCANType = 0x02 // PCAN-Dongle 82C200
	PCAN_TYPE_DNG_EPP     TPCANType = 0x03 // PCAN-Dongle EPP 82C200
	PCAN_TYPE_DNG_SJA     TPCANType = 0x05 // PCAN-Dongle SJA1000
	PCAN_TYPE_DNG_SJA_EPP TPCANType = 0x06 // PCAN-Dongle EPP SJA1000
)

// -----------------------------------------------------------------------------
// Structs
// -----------------------------------------------------------------------------

// TPCANMsg represents a classical CAN frame.
type TPCANMsg struct {
	ID      DWORD            // 11/29-bit message identifier
	MSGTYPE TPCANMessageType // Message type flags
	LEN     BYTE             // DLC (0..8)
	DATA    [8]BYTE          // Data bytes
}

// TPCANTimestamp represents the timestamp of a received classical CAN frame.
// TotalMicroseconds = micros + (1000 * millis) + (0x100000000 * 1000 * millis_overflow)
type TPCANTimestamp struct {
	Millis         DWORD // ms: 0 .. 2^32-1
	MillisOverflow WORD  // rollovers of Millis
	Micros         WORD  // µs: 0..999
}

// TPCANMsgFD represents a CAN FD frame.
type TPCANMsgFD struct {
	ID      DWORD            // 11/29-bit message identifier
	MSGTYPE TPCANMessageType // Message type flags
	DLC     BYTE             // Data Length Code (0..15)
	DATA    [64]BYTE         // Up to 64 data bytes
}

// TPCANChannelInformation describes an available PCAN channel.
type TPCANChannelInformation struct {
	ChannelHandle    TPCANHandle                    // PCAN channel handle
	DeviceType       TPCANDevice                    // Kind of PCAN device
	ControllerNumber BYTE                           // CAN controller number (index)
	DeviceFeatures   DWORD                          // Device capability flags (FEATURE_*)
	DeviceName       [MAX_LENGTH_HARDWARE_NAME]byte // Null-terminated string
	DeviceID         DWORD                          // Device number
	ChannelCondition DWORD                          // Channel availability status
}
