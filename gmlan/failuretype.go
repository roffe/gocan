package gmlan

// FailureTypeString returns the GMW3110 Appendix E description of a
// DTCFailureTypeByte ("symptom"), e.g. 0x02 = "short to ground". The high
// nibble is the failure category (Table E3), the low nibble the sub type
// (Tables E4..E11); values the spec does not enumerate fall back to their
// category.
func FailureTypeString(b byte) string {
	if s, ok := failureTypes[b]; ok {
		return s
	}
	if b>>4 == 0xF {
		return "system specific"
	}
	return "reserved"
}

var failureTypes = map[byte]string{
	// category 0: general electrical failures
	0x00: "no additional information",
	0x01: "short to battery",
	0x02: "short to ground",
	0x03: "voltage below threshold",
	0x04: "open circuit",
	0x05: "short to battery or open",
	0x06: "short to ground or open",
	0x07: "voltage above threshold",
	0x08: "signal invalid",
	0x09: "rate of change above threshold",
	0x0A: "rate of change below threshold",
	0x0B: "current above threshold",
	0x0C: "current below threshold",
	0x0D: "resistance above threshold",
	0x0E: "resistance below threshold",
	0x0F: "erratic",
	// category 1: additional general electrical failures
	0x11: "above maximum threshold",
	0x12: "below minimum threshold",
	0x13: "voltage low/high temperature",
	0x14: "voltage high/low temperature",
	0x15: "signal rising time failure",
	0x16: "signal falling time failure",
	0x17: "signal shape/waveform failure",
	0x18: "signal amplitude < minimum",
	0x19: "signal amplitude > maximum",
	0x1A: "bias level out of range",
	0x1B: "signal cross coupled",
	0x1F: "intermittent",
	// category 2: FM/PWM failures
	0x21: "incorrect period",
	0x22: "low time < minimum",
	0x23: "low time > maximum",
	0x24: "high time < minimum",
	0x25: "high time > maximum",
	0x26: "frequency too low",
	0x27: "frequency too high",
	0x28: "incorrect frequency",
	0x29: "too few pulses",
	0x2A: "too many pulses",
	0x2B: "missing reference",
	// category 3: ECU internal failures
	0x31: "general checksum failure",
	0x32: "general memory failure",
	0x33: "special memory failure",
	0x34: "RAM failure",
	0x35: "ROM failure",
	0x36: "EEPROM failure",
	0x37: "watchdog/safety µC failure",
	0x38: "supervision software failure",
	0x39: "internal electronic failure",
	0x3A: "incorrect component installed",
	0x3B: "internal self test failed",
	0x3C: "internal communications failure",
	// category 4: ECU programming failures
	0x41: "operational software/calibration set not programmed",
	0x42: "calibration data set not programmed",
	0x43: "EEPROM error",
	0x44: "security access not activated",
	0x45: "variant not programmed",
	0x46: "vehicle configuration not programmed",
	0x47: "VIN not programmed",
	0x48: "theft/security data not programmed",
	0x49: "RAM error",
	0x4A: "checksum error",
	0x4B: "calibration not learned",
	0x4C: "DTC memory full",
	0x4D: "stack overflow",
	// category 5: algorithm based failures
	0x53: "temperature low",
	0x54: "temperature high",
	0x55: "expected number of transitions/events not reached",
	0x56: "allowable number of transitions/events exceeded",
	0x58: "incorrect reaction after event",
	0x59: "circuit/component time-out",
	0x5A: "plausibility failure",
	// category 6: mechanical failures
	0x61: "actuator stuck",
	0x62: "actuator stuck open",
	0x63: "actuator stuck closed",
	0x64: "actuator slipping",
	0x65: "emergency position not reachable",
	0x66: "wrong mounting position",
	0x67: "incorrect assembly",
	// category 7: bus signal/message failures
	0x71: "invalid serial data received",
	0x72: "alive counter incorrect/not updated",
	0x73: "parity error",
	0x74: "value of signal calculation incorrect",
	0x75: "signal above allowable range",
	0x76: "signal below allowable range",
	0x7F: "erratic",
}
