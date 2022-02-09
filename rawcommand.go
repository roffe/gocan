package gocan

type RawCommand struct {
	Data string
}

func (r *RawCommand) Byte() []byte {
	return []byte(r.Data + "\r")
}
