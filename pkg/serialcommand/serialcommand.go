package serialcommand

import "fmt"

type SerialCommand struct {
	Command byte
	Data    []byte
}

func NewSerialCommand(command byte, data []byte) *SerialCommand {
	return &SerialCommand{
		Command: command,
		Data:    data,
	}
}

func (sc *SerialCommand) MarshalBinary() ([]byte, error) {
	if len(sc.Data) > 255 {
		return nil, fmt.Errorf("data size is too big")
	}
	checksum := sc.Checksum()

	data := make([]byte, len(sc.Data)+3)
	data[0] = sc.Command
	data[1] = byte(len(sc.Data))
	copy(data[2:], sc.Data)
	data[len(data)-1] = checksum

	// buf := make([]byte, 0, 3+len(sc.Data))
	// buf = append(buf, sc.Command, byte(len(sc.Data)))
	// buf = append(buf, sc.Data...)
	// buf = append(buf, checksum)

	return data, nil
}

func (sc *SerialCommand) UnmarshalBinary(data []byte) error {
	if len(data) < 3 {
		return nil
	}
	sc.Command = data[0]

	commandSize := data[1]
	if len(data) != int(commandSize)+3 {
		return fmt.Errorf("invalid command size")
	}

	sc.Data = data[2 : 2+commandSize]

	checksum := data[len(data)-1]
	if checksum != sc.Checksum() {
		return fmt.Errorf("checksum validation failed")
	}
	return nil
}

func (sc *SerialCommand) Checksum() byte {
	var checksum byte
	for _, b := range sc.Data {
		checksum += b
	}
	return checksum
}

func (sc *SerialCommand) String() string {
	return fmt.Sprintf("command: %02X, data: %02X", sc.Command, sc.Data)
}
