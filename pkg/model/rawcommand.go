package model

type RawCommand struct {
	Data string
}

func (r *RawCommand) Byte() []byte {
	return []byte(r.Data + "\r")
}

func (r *RawCommand) GetIdentifier() uint32 {
	return 0
}

func (r *RawCommand) GetData() []byte {
	return []byte(r.Data)
}

func (r *RawCommand) String() string {
	return r.Data
}
