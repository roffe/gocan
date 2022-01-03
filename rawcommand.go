package canusb

type rawCommand struct {
	data string
}

func (r *rawCommand) Byte() []byte {
	return []byte(r.data + "\r")
}
