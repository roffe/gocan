package dvi

type CommandParser struct {
	buffer []byte // Buffer for accumulating bytes
	//mu      sync.Mutex
	handler func(cmd *Command)
}

func NewCommandParser(handler func(cmd *Command)) *CommandParser {
	if handler == nil {
		panic("handler is nil")
	}
	return &CommandParser{
		handler: handler,
	}
}

func (cp *CommandParser) AddData(data []byte) {
	//cp.mu.Lock()
	//defer cp.mu.Unlock()
	cp.buffer = append(cp.buffer, data...)
	cp.tryParseCommands()
}

func (cp *CommandParser) tryParseCommands() {
	for {
		if len(cp.buffer) < 3 { // Not enough data to even determine command length
			return
		}

		// Peek at length of the current command
		length := int(cp.buffer[1])
		totalCommandLength := 3 + length // command byte + length byte + data + checksum

		if len(cp.buffer) < totalCommandLength { // Not enough data for a complete command
			return
		}

		commandBytes := cp.buffer[:totalCommandLength]
		cmd, err := Parse(commandBytes)
		if err != nil {
			// Handle parse error: log, discard bytes, etc.
			//log.Printf("Error parsing command: %v: %s\n", err, commandBytes)
			// Assuming we discard the faulty command and try next; adjust logic as needed.
			//fmt.Print(".")
			//log.Printf("Error parsing command: %v: %X", err, commandBytes)
			cp.buffer = cp.buffer[1:]
			continue
		}

		// Successfully parsed a command, handle it
		// For example, you might send it to another function or channel
		cp.handler(cmd)

		// Remove parsed command bytes from buffer
		cp.buffer = cp.buffer[totalCommandLength:]
	}
}
