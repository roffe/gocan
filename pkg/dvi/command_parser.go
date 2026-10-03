package dvi

type CommandParser struct {
	buffer []byte // Buffer for accumulating bytes
	//mu      sync.Mutex
	handler func(cmd *Command)
	cmd     Command // reused for every parsed command
}

// NewCommandParser returns a parser calling handler for each complete
// command. The *Command and its Data are reused: valid only during the call.
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
	buf := cp.buffer
	for {
		if len(buf) < 3 { // Not enough data to even determine command length
			break
		}

		// Peek at length of the current command
		length := int(buf[1])
		totalCommandLength := 3 + length // command byte + length byte + data + checksum

		if len(buf) < totalCommandLength { // Not enough data for a complete command
			break
		}

		commandBytes := buf[:totalCommandLength]
		if err := cp.cmd.decode(commandBytes); err != nil {
			// Handle parse error: log, discard bytes, etc.
			//log.Printf("Error parsing command: %v: %s\n", err, commandBytes)
			// Assuming we discard the faulty command and try next; adjust logic as needed.
			//fmt.Print(".")
			//log.Printf("Error parsing command: %v: %X", err, commandBytes)
			buf = buf[1:]
			continue
		}

		// Successfully parsed a command, handle it
		// For example, you might send it to another function or channel
		cp.handler(&cp.cmd)

		// Remove parsed command bytes from buffer
		buf = buf[totalCommandLength:]
	}
	// Move the unparsed tail to the front: re-slicing past parsed commands
	// shrinks the capacity until nearly every AddData has to regrow it.
	cp.buffer = cp.buffer[:copy(cp.buffer, buf)]
}
