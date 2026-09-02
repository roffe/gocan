package t7kwp

const (
	READ_ECU_IDENTIFICATION = 0x1A // present in Trionic 7

	/* DATA TRANSMISSION FUNCTIONAL UNIT */
	READ_DATA_BY_COMMON_IDENTIFIER      = 0x22
	DYNAMICALLY_DEFINE_LOCAL_IDENTIFIER = 0x2C // present in Trionic 7
	WRITE_DATA_BY_LOCAL_IDENTIFIER      = 0x3B // present in Trionic 7
	WRITE_DATA_BY_COMMON_IDENTIFIER     = 0x2E
	SET_DATA_RATES                      = 0x26

	/* STORED DATA TRANSMISSION FUNCTIONAL UNIT */
	READ_DIAGNOSTIC_TROUBLE_CODES           = 0x13
	READ_DIAGNOSTIC_TROUBLE_CODES_BY_STATUS = 0x18 // present in Trionic 7
	READ_STATUS_OF_DIAGNOSTIC_TROUBLE_CODES = 0x17
	CLEAR_DIAGNOSTIC_INFORMATION            = 0x14 // present in Trionic 7

	/* INPUTOUTPUT CONTROL FUNCTIONAL UNIT */
	INPUT_OUTPUT_CONTROL_BY_COMMON_IDENTIFIER = 0x2F
	INPUT_OUTPUT_CONTROL_BY_LOCAL_IDENTIFIER  = 0x30 // present in Trionic 7

	/* REMOTE ACTIVATION OF ROUTINE FUNCTIONAL UNIT */
	REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER = 0x33 // present in Trionic 7
	REQUEST_ROUTINE_RESULTS_BY_ADDRESS          = 0x3A

	STOP_REPEATED_DATA_TRANSMISSION = 0x25
	START_COMMUNICATION             = 0x81
	STOP_COMMUNICATION              = 0x82
	ACCESS_TIMING_PARAMETERS        = 0x83

	SYMBOL_IDENTIFICATION = 0x80

	/* Dynamically defined local identifier constants */
	DM_DBLI  = 0x01
	DM_DBMA  = 0x03
	DM_CDDLI = 0x04

	/* Start Routine by local ID definitions */
	// Define Debug frame contents
	RLI_DD = 0x40
	// Set operational mode
	RLI_DMC = 0x41
	// Read Mode status
	RLI_RM = 0x42
	// Read Debug frame contents
	RLI_RC = 0x43
	// Setup for symbol table reading
	RLI_SYM = 0x50
	// Setup for symbol table checksum
	RLI_CHSUM = 0x51
	// Start of EOL programming
	RLI_EOL_START = 0x52
	// Start erasing
	RLI_ERASE   = 0x53
	RLI_END_EOL = 0x54

	// Security access level
	NO_PRIORITY          = 0x00
	LOW_PRIORITY         = 0x01
	HIGH_PRIORITY        = 0x03
	DEVELOPMENT_PRIORITY = 0x05
)
